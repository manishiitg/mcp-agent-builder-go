package cliruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareRefusesUnsafeStorage(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace-docs")
	workflow := filepath.Join(workspace, "Workflow", "testing")
	if err := os.MkdirAll(workflow, 0700); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"", "relative", workspace, filepath.Join(workspace, "nested")} {
		if dir, err := Prepare(state, workspace, "owner", workflow, "chat", "codex-cli", "workshop"); err == nil || dir != "" {
			t.Fatalf("unsafe state accepted: %q", state)
		}
	}
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(workflow, filepath.Join(state, "cli-runtimes")); err != nil {
		t.Fatal(err)
	}
	if dir, err := Prepare(state, workspace, "owner", workflow, "chat", "codex-cli", "workshop"); err == nil || dir != "" {
		t.Fatal("symlinked runtime accepted")
	}
	if err := os.Remove(filepath.Join(state, "cli-runtimes")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "cli-runtimes"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if dir, err := Prepare(state, workspace, "owner", workflow, "chat", "codex-cli", "workshop"); err == nil || dir != "" {
		t.Fatal("file obstruction fell back")
	}
}
