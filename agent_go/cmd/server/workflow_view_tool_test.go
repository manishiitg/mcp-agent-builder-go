package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

type recordedTool struct {
	desc string
	exec func(context.Context, map[string]interface{}) (string, error)
}

type recordingRegistrar struct {
	tools map[string]recordedTool
}

func (r *recordingRegistrar) RegisterCustomTool(name, desc string, _ map[string]interface{}, exec func(context.Context, map[string]interface{}) (string, error), _ string) error {
	if r.tools == nil {
		r.tools = map[string]recordedTool{}
	}
	r.tools[name] = recordedTool{desc: desc, exec: exec}
	return nil
}

func (r *recordingRegistrar) RegisterCustomToolWithTimeout(name, desc string, params map[string]interface{}, exec func(context.Context, map[string]interface{}) (string, error), _ time.Duration, category string) error {
	return r.RegisterCustomTool(name, desc, params, exec, category)
}

func TestOpenWorkspaceViewToolOpensAKnownViewAndRefusesOthers(t *testing.T) {
	api := &StreamingAPI{}
	reg := &recordingRegistrar{}
	if err := api.registerOpenWorkspaceViewTool(reg, "s1", "Workflow/x"); err != nil {
		t.Fatal(err)
	}
	open, ok := reg.tools["open_workspace_view"]
	if !ok || !strings.Contains(open.desc, "report — Report") || !strings.Contains(open.desc, "schedules — Schedules") || !strings.Contains(open.desc, "refresh_workspace_view") {
		t.Fatalf("open tool = %+v", open)
	}
	out, err := open.exec(context.Background(), map[string]interface{}{"view": "Report"})
	if err != nil || !strings.Contains(out, `"opened":"report"`) || !strings.Contains(out, `"label":"Report"`) {
		t.Fatalf("out=%s err=%v", out, err)
	}
	if _, err := open.exec(context.Background(), map[string]interface{}{"view": "dashboard"}); err == nil || !strings.Contains(err.Error(), "one of:") {
		t.Fatalf("unknown view must list the real ones: %v", err)
	}
	refresh, ok := reg.tools["refresh_workspace_view"]
	if !ok {
		t.Fatal("refresh_workspace_view must be registered alongside open_workspace_view")
	}
	out, err = refresh.exec(context.Background(), map[string]interface{}{"view": "database"})
	if err != nil || !strings.Contains(out, `"refreshed":"database"`) {
		t.Fatalf("refresh out=%s err=%v", out, err)
	}
	ev, err := workspaceViewPresentation("costs", "Workflow/x")
	if err != nil || ev.Kind != WorkflowViewPresentationKind || ev.Payload["view"] != "costs" || ev.Payload["action"] != "open" || ev.Activity == nil || ev.Activity.Detail != "Costs" || ev.WorkspacePath != "Workflow/x" {
		t.Fatalf("event = %+v err=%v", ev, err)
	}
	rv, err := workspaceViewAction("report", "Workflow/x", "refresh")
	if err != nil || rv.Payload["action"] != "refresh" || rv.Activity.Label != "Refreshed" || rv.PresentationID == ev.PresentationID {
		t.Fatalf("refresh event = %+v err=%v", rv, err)
	}
}
