package cliruntime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

func TestPrivateRuntimeIdentityAndAliasResume(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	workflow := filepath.Join(workspace, "Workflow", "testing")
	if err := os.MkdirAll(workflow, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "agentworks")
	first, err := Prepare(state, workspace, "owner", workflow, "chat", "codex-cli", "run")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Prepare(state, workspace, "owner", workflow, "chat", "codex-cli", "run")
	if err != nil || first != again || !CanResume(first, again) {
		t.Fatal("restart changed runtime identity")
	}
	// Existing v1 hashes must not change as a side effect of alias normalization.
	legacyWorkflow, err := filepath.EvalSymlinks(workflow)
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := json.Marshal([]string{"owner", legacyWorkflow, "chat", "codex-cli", "run"})
	if filepath.Base(first) != fmt.Sprintf("%x", sha256.Sum256(identity)) {
		t.Fatal("v1 digest compatibility lost")
	}
	alias := filepath.Join(root, "AgentWorks")
	if _, err := os.Stat(alias); err != nil {
		if err := os.Symlink(state, alias); err != nil {
			t.Fatal(err)
		}
	}
	rel, err := filepath.Rel(state, first)
	if err != nil {
		t.Fatal(err)
	}
	if !CanResume(first, filepath.Join(alias, rel)) {
		t.Fatal("same runtime cannot resume through alias")
	}
	for _, values := range [][4]string{
		{"other-owner", "chat", "codex-cli", "run"},
		{"owner", "other-chat", "codex-cli", "run"},
		{"owner", "chat", "claude-code", "run"},
		{"owner", "chat", "codex-cli", "workshop"},
	} {
		other, err := Prepare(state, workspace, values[0], workflow, values[1], values[2], values[3])
		if err != nil {
			t.Fatal(err)
		}
		if CanResume(first, other) {
			t.Fatalf("runtime identity crossed boundary: %v", values)
		}
	}
}

func TestPrepareContainmentUsesFilesystemSpelling(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	workflow := filepath.Join(workspace, "Workflow", "testing")
	if err := os.MkdirAll(workflow, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "WORKSPACE")
	if _, err := os.Stat(alias); err != nil {
		t.Skip("requires case-insensitive filesystem")
	}
	if _, err := Prepare(filepath.Join(root, "state"), alias, "owner", workflow, "chat", "codex-cli", "run"); err != nil {
		t.Fatalf("valid workflow rejected through root alias: %v", err)
	}
	if _, err := Prepare(filepath.Join(alias, "state"), workspace, "owner", workflow, "chat", "codex-cli", "run"); err == nil {
		t.Fatal("case alias bypassed private-state containment")
	}
}
