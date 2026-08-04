package videoproduct

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinVideoSkills(t *testing.T) {
	want := map[string]string{
		"video-creation":        "work/production.json",
		"video-shot-generation": "reference",
		"video-editing":         "hard cuts",
		"video-quality":         "work/qa/",
		"html-composition":      "headless Chrome",
	}

	skills := builtinSkills()
	if len(skills) != len(want) {
		t.Fatalf("builtin skill count = %d, want %d", len(skills), len(want))
	}
	for _, skill := range skills {
		required, ok := want[skill.Name]
		if !ok {
			t.Fatalf("unexpected builtin skill %q", skill.Name)
		}
		if strings.TrimSpace(skill.Content) == "" {
			t.Fatalf("builtin skill %q has empty content", skill.Name)
		}
		if !strings.Contains(skill.Content, required) {
			t.Fatalf("builtin skill %q does not contain %q", skill.Name, required)
		}
		if skill.Source.Origin != "builtin" {
			t.Fatalf("builtin skill %q origin = %q", skill.Name, skill.Source.Origin)
		}
	}
}

func TestVideoShellToolEmitsSafeActivity(t *testing.T) {
	workspace := t.TempDir()
	var events []AgentEvent
	tool := videoShellTool(workspace, nil, func(event AgentEvent) { events = append(events, event) })
	result, err := tool.Handler(context.Background(), map[string]interface{}{"command": "printf hello"})
	if err != nil || strings.TrimSpace(result) != "hello" {
		t.Fatalf("shell result = %q, err = %v", result, err)
	}
	if len(events) != 2 || events[0].Status != "running" || events[1].Status != "completed" {
		t.Fatalf("tool events = %#v", events)
	}
	if events[0].ToolCallID == "" || events[0].ToolCallID != events[1].ToolCallID {
		t.Fatalf("tool call IDs = %q and %q", events[0].ToolCallID, events[1].ToolCallID)
	}
	if events[0].Text != "" || events[1].Text != "" {
		t.Fatalf("tool events exposed command text: %#v", events)
	}
	if _, err := os.Stat(filepath.Join(workspace, "hello")); !os.IsNotExist(err) {
		t.Fatalf("unexpected workspace output: %v", err)
	}
}
