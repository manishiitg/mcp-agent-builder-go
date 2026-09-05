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
var workflowWorkspaceViews = func() []struct{ ID, Label, About string } {
	views := make([]struct{ ID, Label, About string }, 0, len(uiControlContract.Views))
	for _, view := range uiControlContract.Views {
		views = append(views, struct{ ID, Label, About string }{view.ID, view.Label, view.Label})
	}
	return views
}()

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
	return workspaceViewAction(view, workspacePath, "open", "")
}

// workspaceViewTargets documents what `target` means for the views that honor
// one. A view missing here ignores a target rather than failing, so the agent
// can always pass one.
var workspaceViewTargets = map[string]string{
	"report":         "the report's top-level tab name, as the report HTML labels it",
	"flow":           "a step id, focused on the canvas",
	"files":          "a workspace-relative file path, opened in the pane",
	"database":       "a table name",
	"execution-logs": "a step id",
	"schedules":      "a schedule id or name",
}

// workspaceViewAction builds the open or refresh event for a view; the page
// reads payload.action to tell them apart, and payload.target for what to
// focus inside the view.
func workspaceViewAction(view, workspacePath, action, target string) (*orchestratorevents.PresentationUpdatedEvent, error) {
	view = strings.TrimSpace(strings.ToLower(view))
	for _, v := range workflowWorkspaceViews {
		if v.ID != view {
			continue
		}
		label := "Open requested"
		if action == "refresh" {
			label = "Refresh requested"
		}
		detail := v.Label
		payload := map[string]interface{}{"view": v.ID, "action": action}
		if target = strings.TrimSpace(target); target != "" {
			payload["target"] = target
			detail = v.Label + " · " + target
		}
		return &orchestratorevents.PresentationUpdatedEvent{
			PresentationID: "workspace-view:" + v.ID + ":" + action,
			Kind:           WorkflowViewPresentationKind,
			Title:          v.Label,
			WorkspacePath:  workspacePath,
			Payload:        payload,
			Activity:       &orchestratorevents.PresentationActivity{Label: label, Destination: "the workspace pane", Detail: detail},
		}, nil
	}
	return nil, fmt.Errorf("unknown view %q; one of: %s", view, strings.Join(workflowWorkspaceViewIDs(), ", "))
}

// registerOpenWorkspaceViewTool gives the workflow agent the toolbar: it can
// open any workspace view on the right for the user (the report after
// building it, the costs when asked about spend, the schedules after adding
// one) instead of describing where to click.
func (api *StreamingAPI) registerOpenWorkspaceViewTool(registrar definitionToolRegistrar, sessionID, workspacePath string) error {
	if err := api.registerUIControlTools(registrar, sessionID, workspacePath); err != nil {
		return err
	}
	var lines []string
	for _, v := range workflowWorkspaceViews {
		lines = append(lines, fmt.Sprintf("%s — %s (%s)", v.ID, v.Label, v.About))
	}
	description := "Legacy, unverified presentation request. Prefer perform_ui_action for browser-acknowledged actions. This tool returns requested, NOT proof of visible rendering. Open one of the workspace views on the right side of the user's screen, the same views as the toolbar above the chat. " +
		"Use it when what you are talking about is on one of them: after you build or update the report, open `report`; when the user asks about spend, open `costs`; " +
		"after adding a schedule, open `schedules`. Opening a view that is already on screen does nothing; to reload one you changed, use refresh_workspace_view. Pass `target` to focus something inside the view — a report tab, a plan step, a file, a table. What each view contains, in detail: builder-reference/references/workspace-views.md. Views:\n" + strings.Join(lines, "\n")
	var targetHelp []string
	for _, v := range workflowWorkspaceViews {
		if meaning, ok := workspaceViewTargets[v.ID]; ok {
			targetHelp = append(targetHelp, fmt.Sprintf("%s: %s", v.ID, meaning))
		}
	}
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"view":   map[string]interface{}{"type": "string", "enum": workflowWorkspaceViewIDs(), "description": "which view to open"},
			"target": map[string]interface{}{"type": "string", "description": "optional — what to focus inside the view. " + strings.Join(targetHelp, "; ") + ". Ignored by views that have nothing to focus."},
		},
		"required": []string{"view"},
	}
	if err := registrar.RegisterCustomTool("open_workspace_view", description, params, func(_ context.Context, args map[string]interface{}) (string, error) {
		if api.uiBroker().scope(sessionID) != workspacePath {
			return uiError(fmt.Errorf("inactive_scope")), nil
		}
		view, _ := args["view"].(string)
		target, _ := args["target"].(string)
		event, err := workspaceViewAction(view, workspacePath, "open", target)
		if err != nil {
			return "", err
		}
		api.emitAgentProfileEvent(sessionID, event)
		out, _ := json.Marshal(map[string]interface{}{"status": "requested", "visible": false, "receipt": "unverified_legacy_presentation", "opened": event.Payload["view"], "label": event.Title})
		return string(out), nil
	}, "workflow_ui"); err != nil {
		return err
	}
	refreshDescription := "Legacy, unverified refresh request; requested is NOT proof that fresh content loaded. Reload a workspace view after you changed what it shows: the report after editing db/reports/index.html, the database after writing rows, schedules after adding one, files after writing them. " +
		"Opens the view first if it is not on screen. Same view names and the same optional `target` as open_workspace_view."
	return registrar.RegisterCustomTool("refresh_workspace_view", refreshDescription, params, func(_ context.Context, args map[string]interface{}) (string, error) {
		if api.uiBroker().scope(sessionID) != workspacePath {
			return uiError(fmt.Errorf("inactive_scope")), nil
		}
		view, _ := args["view"].(string)
		target, _ := args["target"].(string)
		event, err := workspaceViewAction(view, workspacePath, "refresh", target)
		if err != nil {
			return "", err
		}
		api.emitAgentProfileEvent(sessionID, event)
		out, _ := json.Marshal(map[string]interface{}{"status": "requested", "visible": false, "receipt": "unverified_legacy_presentation", "refreshed": event.Payload["view"], "label": event.Title})
		return string(out), nil
	}, "workflow_ui")
}
