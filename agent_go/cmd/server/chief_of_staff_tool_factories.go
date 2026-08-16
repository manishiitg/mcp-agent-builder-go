package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

// chiefOfStaffToolFactoryIDs are the ToolFactory registry keys the
// chief-of-staff product.yaml's profile.tools[] binds against. The tool
// NAME the agent actually calls (create_workflow, get_activity_status,
// update_chief_of_staff_notifications) is unchanged -- only how it gets
// registered moved, from a manual block gated on isChiefOfStaffChat to a
// declared profile.tools[] binding, same as video.show-video.
const (
	ChiefOfStaffToolFactoryCreateWorkflow      = "chief-of-staff.create-workflow"
	ChiefOfStaffToolFactoryActivityStatus      = "chief-of-staff.activity-status"
	ChiefOfStaffToolFactoryUpdateNotifications = "chief-of-staff.update-notifications"
)

// registerChiefOfStaffToolFactories registers the three Chief-of-Staff-only
// tool factories with the shared agent profile registry. Called once at
// server startup, alongside videoproduct's own factory registration -- the
// factories themselves stay in cmd/server (not a separate internal package)
// because their handlers are deeply coupled to *StreamingAPI internals
// (api.scheduler, api.chatStore, api.persistChiefNotificationConfig), the
// same reason Video Studio's factories instead use a standalone workspace
// API client: there is no such client-based equivalent for these three.
func (api *StreamingAPI) registerChiefOfStaffToolFactories(registry *agentprofiles.Registry) error {
	if err := registry.RegisterToolFactory(ChiefOfStaffToolFactoryCreateWorkflow, api.workflowCreatorToolFactory()); err != nil {
		return err
	}
	if err := registry.RegisterToolFactory(ChiefOfStaffToolFactoryActivityStatus, api.activityStatusToolFactory()); err != nil {
		return err
	}
	return registry.RegisterToolFactory(ChiefOfStaffToolFactoryUpdateNotifications, api.notificationToolFactory())
}

// workflowCreatorToolFactory wraps handleWorkflowCreatorTool -- the handler
// itself is untouched; only the registration shape changed. Params/description
// are copied verbatim from the former registerWorkflowCreatorTool.
func (api *StreamingAPI) workflowCreatorToolFactory() agentprofiles.ToolFactory {
	return func(_ agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		return agentprofiles.ToolSpec{
			Name:        "create_workflow",
			Category:    "workflow_creator",
			Description: "Create a new workflow at Workflow/<folder_name>/ with the given workflow.json and planning/plan.json. Use one large message_sequence per shared-context span, with proof/provenance, evidence-based double-checking, repair, and final validation inside it; create another large sequence only when its context should be isolated. Put deterministic API/SDK/CLI/data-fetch/parse/persist work in coherent scripted-fetcher candidates with authoritative validated outputs. Never use one regular step per endpoint, routine action, or proof check. This tool writes structure only; after creation tell the user to open Workshop so deterministic steps are declared scripted and main.py is authored/tested before production. The tool validates the complete plan graph before writing anything and refuses dangling targets or overwrite.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder_name": map[string]interface{}{
						"type":        "string",
						"description": "Shell-safe folder name under Workflow/ — kebab-case (lowercase letters, digits, hyphens only). No spaces, no underscores, no uppercase, no special characters. Examples: 'customer-onboarding', 'sales-report', 'api-health-check'. This is ONLY the on-disk folder name so shell commands like `ls Workflow/<folder_name>/` work without quoting. The human-readable display name goes in workflow_json.label and can be any string.",
					},
					"workflow_json": map[string]interface{}{
						"type":                 "object",
						"description":          "The full workflow.json manifest object. Required fields: schema_version (int, 1), id (string, e.g. 'wf_<folder_name>'), label (string, free-form human-readable name — can contain spaces, capitalization, anything). Should include objective, success_criteria, and a capabilities object with selected_servers/skills/etc picked smartly from the current chat context. Set capabilities.selected_global_secret_names to [] unless specific global secrets are required.",
						"additionalProperties": true,
					},
					"plan_json": map[string]interface{}{
						"type":                 "object",
						"description":          "The full plan.json object. Required field: steps (array, at least 1 step). Start with one large message_sequence per coherent shared-context span; put run-specific proof/provenance, evidence-based double-check and repair turns, and the final validation_schema inside that step. Use multiple large sequences only when contexts should not be shared because of security/credentials, independent outputs/retries, clean-room independence, human/routing boundaries, or context contamination. Fixed API/SDK/CLI calls, deterministic fetching/pagination/parsing/normalization, and mechanical persistence belong in coherent regular fetcher steps batched by source/auth/retry/output contract. Do not create one step per endpoint/tool/checklist/proof item. Every output-producing step needs validation_schema. Each step needs type, id (kebab-case, unique), and title. Every route and explicit next_step_id must target a declared step or end; the complete graph is validated atomically before creation. plan.json cannot store execution mode, so after creation explicitly hand off deterministic steps to Workshop for declared_execution_mode=scripted plus authored/tested main.py before the first production run.",
						"additionalProperties": true,
					},
				},
				"required": []string{"folder_name", "workflow_json", "plan_json"},
			},
			Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
				return api.handleWorkflowCreatorTool(ctx, args)
			},
		}, nil
	}
}

// activityStatusToolFactory wraps handleActivityStatusTool. Uses
// runtime.UserID (the live requesting user for this turn) rather than a
// captured closure value -- more correct than the manual registration it
// replaces, which captured currentUserID once per query setup; a ToolFactory
// is invoked fresh per profile-bound turn, so runtime.UserID is always
// current.
func (api *StreamingAPI) activityStatusToolFactory() agentprofiles.ToolFactory {
	return func(runtime agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		userID := runtime.UserID
		return agentprofiles.ToolSpec{
			Name:        "get_activity_status",
			Category:    "activity_status",
			Description: "Return a JSON snapshot of currently running workflow executions and currently running schedules. Use this when the user asks what workflows, background runs, cron jobs, or multi-agent schedules are running right now.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Execute: func(ctx context.Context, _ map[string]interface{}) (string, error) {
				return api.handleActivityStatusTool(ctx, userID)
			},
		}, nil
	}
}

// notificationToolFactory wraps persistChiefNotificationConfig, preserving
// the same context-value refresh (BotNotificationDestinationKey) the manual
// registration relied on so a notify_user test later in the same turn still
// sees a freshly-saved destination immediately.
func (api *StreamingAPI) notificationToolFactory() agentprofiles.ToolFactory {
	return func(runtime agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		userID := strings.TrimSpace(runtime.UserID)
		if userID == "" {
			userID = "default"
		}
		return agentprofiles.ToolSpec{
			Name:        "update_chief_of_staff_notifications",
			Category:    "notification_tools",
			Description: "Configure or disable the Chief of Staff Slack Incoming Webhook destination. Pass the name of an existing encrypted user secret containing an official Slack Incoming Webhook URL; never pass or expose the URL itself. This updates both interactive Chief of Staff chat and scheduled Chief/Org Pulse runs. Pass an empty secret name to disable the dedicated webhook.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"slack_webhook_secret_name": map[string]interface{}{
						"type":        "string",
						"description": "Existing encrypted secret name, or an empty string to disable the Chief of Staff Slack webhook.",
					},
				},
				"required": []string{"slack_webhook_secret_name"},
			},
			Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
				secretName, _ := args["slack_webhook_secret_name"].(string)
				_, secretValue, err := api.persistChiefNotificationConfig(ctx, userID, secretName)
				if err != nil {
					return "", err
				}
				if destination, ok := ctx.Value(virtualtools.BotNotificationDestinationKey).(*services.NotificationDestination); ok && destination != nil {
					if strings.TrimSpace(secretName) == "" {
						destination.SlackWebhook = nil
					} else {
						destination.SlackWebhook = &services.SlackWebhookDest{
							SecretName: strings.TrimSpace(secretName),
							URL:        secretValue,
						}
					}
				}
				if strings.TrimSpace(secretName) == "" {
					return "Chief of Staff Slack webhook notifications are disabled for interactive and scheduled runs.", nil
				}
				return "Chief of Staff Slack webhook notifications are ready for interactive and scheduled runs using encrypted secret \"" + strings.TrimSpace(secretName) + "\". The webhook value was not exposed.", nil
			},
		}, nil
	}
}
