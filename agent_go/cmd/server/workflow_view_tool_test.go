package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

type recordingRegistrar struct {
	name string
	desc string
	exec func(context.Context, map[string]interface{}) (string, error)
}

func (r *recordingRegistrar) RegisterCustomTool(name, desc string, _ map[string]interface{}, exec func(context.Context, map[string]interface{}) (string, error), _ string) error {
	r.name, r.desc, r.exec = name, desc, exec
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
	if reg.name != "open_workspace_view" || !strings.Contains(reg.desc, "report — Report") || !strings.Contains(reg.desc, "schedules — Schedules") {
		t.Fatalf("tool = %q\n%s", reg.name, reg.desc)
	}
	out, err := reg.exec(context.Background(), map[string]interface{}{"view": "Report"})
	if err != nil || !strings.Contains(out, `"opened":"report"`) || !strings.Contains(out, `"label":"Report"`) {
		t.Fatalf("out=%s err=%v", out, err)
	}
	if _, err := reg.exec(context.Background(), map[string]interface{}{"view": "dashboard"}); err == nil || !strings.Contains(err.Error(), "one of:") {
		t.Fatalf("unknown view must list the real ones: %v", err)
	}
	ev, err := workspaceViewPresentation("costs", "Workflow/x")
	if err != nil || ev.Kind != WorkflowViewPresentationKind || ev.Payload["view"] != "costs" || ev.Activity == nil || ev.Activity.Detail != "Costs" || ev.WorkspacePath != "Workflow/x" {
		t.Fatalf("event = %+v err=%v", ev, err)
	}
}
