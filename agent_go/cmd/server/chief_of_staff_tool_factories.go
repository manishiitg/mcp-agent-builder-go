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
// NAME the agent actually calls (get_activity_status,
// update_chief_of_staff_notifications) is unchanged -- only how it gets
// registered moved, from a manual block gated on isChiefOfStaffChat to a
// declared profile.tools[] binding, same as video.show-video.
//
// create_workflow deliberately has no factory here: the new chief-of-staff
// profile is read-only over Workflow/ (see "Core purpose" in
// docs/design/chief_of_staff_as_product.md), and creating a workflow is a
// write. The legacy no-profile chat still registers it manually, unaffected.
const (
	ChiefOfStaffToolFactoryActivityStatus      = "chief-of-staff.activity-status"
	ChiefOfStaffToolFactoryUpdateNotifications = "chief-of-staff.update-notifications"
)

// registerChiefOfStaffToolFactories registers the Chief-of-Staff-only tool
// factories with the shared agent profile registry. Called once at server
// startup, alongside videoproduct's own factory registration -- the
// factories themselves stay in cmd/server (not a separate internal package)
// because their handlers are deeply coupled to *StreamingAPI internals
// (api.scheduler, api.chatStore, api.persistChiefNotificationConfig), the
// same reason Video Studio's factories instead use a standalone workspace
// API client: there is no such client-based equivalent for these.
func (api *StreamingAPI) registerChiefOfStaffToolFactories(registry *agentprofiles.Registry) error {
	if err := registry.RegisterToolFactory(ChiefOfStaffToolFactoryActivityStatus, api.activityStatusToolFactory()); err != nil {
		return err
	}
	return registry.RegisterToolFactory(ChiefOfStaffToolFactoryUpdateNotifications, api.notificationToolFactory())
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
