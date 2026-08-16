package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAddAgentProfileWorkflowGroupCreatesAndReusesGroup(t *testing.T) {
	input := `{"variables":[],"groups":[],"extraction_date":"old"}`
	now := time.Date(2026, 8, 7, 10, 30, 0, 0, time.UTC)
	updated, changed, err := addAgentProfileWorkflowGroup(input, "launch-film", now)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !strings.HasSuffix(updated, "\n") {
		t.Fatalf("changed=%v updated=%q", changed, updated)
	}
	var manifest agentProfileVariablesManifest
	if err := json.Unmarshal([]byte(updated), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Groups) != 1 || manifest.Groups[0].Name != "launch-film" || !manifest.Groups[0].Enabled {
		t.Fatalf("unexpected groups: %+v", manifest.Groups)
	}
	unchanged, changed, err := addAgentProfileWorkflowGroup(updated, "launch-film", now.Add(time.Minute))
	if err != nil || changed || unchanged != updated {
		t.Fatalf("second add changed=%v err=%v", changed, err)
	}
}

func TestAddAgentProfileWorkflowGroupRejectsUnsafeName(t *testing.T) {
	input := `{"variables":[],"groups":[]}`
	for _, name := range []string{"", "../outside", "has spaces", strings.Repeat("a", 81)} {
		if _, _, err := addAgentProfileWorkflowGroup(input, name, time.Now()); err == nil {
			t.Fatalf("accepted unsafe group %q", name)
		}
	}
}

func TestProfileRequiresWorkflowRuntime(t *testing.T) {
	if profileRequiresWorkflowRuntime(nil) {
		t.Fatal("nil profile requires workflow runtime")
	}
	resolved := &resolvedAgentProfile{}
	if profileRequiresWorkflowRuntime(resolved) {
		t.Fatal("empty capability requires workflow runtime")
	}
	resolved.Definition.Runtime.Capabilities.WorkflowExecution = "required"
	if !profileRequiresWorkflowRuntime(resolved) {
		t.Fatal("required capability did not enable workflow runtime")
	}
}
