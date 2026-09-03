package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	orchestratorevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// workflowWorkspaceViews mirrors the toolbar's view registry in
// frontend/src/components/workflow/workspaceViews.ts (a frontend test keeps
// the two lists identical). The agent opens one of these for the user with
// open_workspace_view; the workflow page switches the right-hand pane.
var workflowWorkspaceViews = []struct{ ID, Label, About string }{
	{"report", "Report", "the workflow's HTML report preview"},
	{"flow", "Plan", "the step plan on the canvas"},
	{"costs", "Costs", "token and cost usage per run"},
	{"execution-logs", "Execution logs", "the run log"},
	{"learnings", "Learnings", "what the workflow has learned across runs"},
	{"knowledgebase", "Knowledgebase", "the workflow's knowledge base"},
	{"database", "Database", "the workflow database tables"},
	{"files", "Files", "the workspace file browser"},
	{"pulse", "Pulse", "pulse status and findings"},
	{"evaluation", "Evaluation", "evaluation results"},
	{"schedules", "Schedules", "scheduled runs"},
	{"backup", "Backup", "backup status"},
	{"publish", "Publish", "the published page"},
	{"notify", "Notify", "notification settings"},
	{"skills", "Workflow skills", "the workflow's skills"},
	{"secrets", "Workflow secrets", "the workflow's secrets (names only)"},
	{"mcp", "Workflow MCP servers", "the workflow's MCP servers"},
	{"browser", "Browser automation", "browser automation settings"},
	{"llm", "Workflow LLM configuration", "the LLM configuration"},
	{"bots", "Workflow bots", "connected bots"},
	{"folders", "Attached folders", "folders attached to the workflow"},
}

// WorkflowViewPresentationKind is the presentation kind the workflow page
// reacts to by opening a workspace view.
const WorkflowViewPresentationKind = "workflow.view"

func workflowWorkspaceViewIDs() []string {
	ids := make([]string, 0, len(workflowWorkspaceViews))
	for _, v := range workflowWorkspaceViews {
		ids = append(ids, v.ID)
	}
	return ids
}

// workspaceViewPresentation is the event that opens a view for the user.
func workspaceViewPresentation(view, workspacePath string) (*orchestratorevents.PresentationUpdatedEvent, error) {
	view = strings.TrimSpace(strings.ToLower(view))
	for _, v := range workflowWorkspaceViews {
		if v.ID != view {
			continue
		}
		return &orchestratorevents.PresentationUpdatedEvent{
			PresentationID: "workspace-view:" + v.ID,
			Kind:           WorkflowViewPresentationKind,
			Title:          v.Label,
			WorkspacePath:  workspacePath,
			Payload:        map[string]interface{}{"view": v.ID},
			Activity:       &orchestratorevents.PresentationActivity{Label: "Opened", Destination: "the workspace pane", Detail: v.Label},
		}, nil
	}
	return nil, fmt.Errorf("unknown view %q; one of: %s", view, strings.Join(workflowWorkspaceViewIDs(), ", "))
}

// registerOpenWorkspaceViewTool gives the workflow agent the toolbar: it can
// open any workspace view on the right for the user (the report after
// building it, the costs when asked about spend, the schedules after adding
// one) instead of describing where to click.
func (api *StreamingAPI) registerOpenWorkspaceViewTool(registrar definitionToolRegistrar, sessionID, workspacePath string) error {
	var lines []string
	for _, v := range workflowWorkspaceViews {
		lines = append(lines, fmt.Sprintf("%s — %s (%s)", v.ID, v.Label, v.About))
	}
	description := "Open one of the workspace views on the right side of the user's screen, the same views as the toolbar above the chat. " +
		"Use it when what you are talking about is on one of them: after you build or update the report, open `report`; when the user asks about spend, open `costs`; " +
		"after adding a schedule, open `schedules`. One call per view; the view stays open until the user changes it. Views:\n" + strings.Join(lines, "\n")
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"view": map[string]interface{}{"type": "string", "enum": workflowWorkspaceViewIDs(), "description": "which view to open"},
		},
		"required": []string{"view"},
	}
	return registrar.RegisterCustomTool("open_workspace_view", description, params, func(_ context.Context, args map[string]interface{}) (string, error) {
		view, _ := args["view"].(string)
		event, err := workspaceViewPresentation(view, workspacePath)
		if err != nil {
			return "", err
		}
		api.emitAgentProfileEvent(sessionID, event)
		out, _ := json.Marshal(map[string]interface{}{"status": "ok", "opened": event.Payload["view"], "label": event.Title})
		return string(out), nil
	}, "workflow_ui")
}
