package step_based_workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workflowtypes"
)

func TestWorkflowFolderAccessBuilderPromptAdvertisesApprovalFlowWithoutGrants(t *testing.T) {
	docsRoot := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsRoot)
	workspacePath := "Workflow/no-attached-folders"
	if err := os.MkdirAll(filepath.Join(docsRoot, filepath.FromSlash(workspacePath)), 0o755); err != nil {
		t.Fatal(err)
	}

	prompt := workflowFolderAccessBuilderPrompt(workspacePath)
	for _, required := range []string{
		"No external folders are currently attached",
		"request_workflow_folder_access",
		"create a pending request",
		"Workflow toolbar → Attached folders",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("Builder prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func TestUpsertWorkflowFolderAccessRequestPersistsAndDeduplicates(t *testing.T) {
	raw := []byte(`{"schema_version":1,"id":"wf_test","label":"test","folder_access":[]}`)
	request := workflowtypes.WorkflowFolderAccessRequest{
		ID: "folder-request-1", Alias: "public-website", Access: workflowtypes.FolderAccessReadWrite,
		Reason: "Publish the site", RequestedAt: "2026-08-29T16:45:00Z",
	}
	updated, existing, err := upsertWorkflowFolderAccessRequest(raw, request)
	if err != nil || existing {
		t.Fatalf("first upsert: existing=%v err=%v", existing, err)
	}
	var manifest workflowFolderAccessManifest
	if err := json.Unmarshal(updated, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.FolderAccessRequests) != 1 || manifest.FolderAccessRequests[0].Alias != "public-website" {
		t.Fatalf("pending request not persisted: %#v", manifest.FolderAccessRequests)
	}
	_, existing, err = upsertWorkflowFolderAccessRequest(updated, request)
	if err != nil || !existing {
		t.Fatalf("duplicate upsert: existing=%v err=%v", existing, err)
	}

	request.RequestedPath = "/tmp/public-website"
	updated, existing, err = upsertWorkflowFolderAccessRequest(updated, request)
	if err != nil || existing {
		t.Fatalf("path enrichment: existing=%v err=%v", existing, err)
	}
	if err := json.Unmarshal(updated, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.FolderAccessRequests) != 1 || manifest.FolderAccessRequests[0].RequestedPath != request.RequestedPath {
		t.Fatalf("pending request path was not enriched in place: %#v", manifest.FolderAccessRequests)
	}
}

func TestAppendWorkflowFolderAccessPreservesModesAndAliases(t *testing.T) {
	docsRoot := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsRoot)
	workspacePath := "Workflow/attached-folders"
	workflowDir := filepath.Join(docsRoot, filepath.FromSlash(workspacePath))
	readOnly := t.TempDir()
	readWrite := t.TempDir()
	readOnly, _ = filepath.EvalSymlinks(readOnly)
	readWrite, _ = filepath.EvalSymlinks(readWrite)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := workflowFolderAccessManifest{FolderAccess: []workflowtypes.WorkflowFolderGrant{
		{ID: "read", Alias: "reference-data", Path: readOnly, Access: workflowtypes.FolderAccessReadOnly},
		{ID: "write", Alias: "rts-source", Path: readWrite, Access: workflowtypes.FolderAccessReadWrite},
		{ID: "missing", Alias: "missing", Path: filepath.Join(t.TempDir(), "gone"), Access: workflowtypes.FolderAccessReadWrite},
	}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "workflow.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	reads, writes, readOnlyPaths, env := appendWorkflowFolderAccess(workspacePath, []string{"Workflow/attached-folders"}, nil)
	if !containsString(reads, readOnly) || !containsString(reads, readWrite) {
		t.Fatalf("read grants missing: %v", reads)
	}
	if containsString(writes, readOnly) || !containsString(writes, readWrite) {
		t.Fatalf("write modes not preserved: %v", writes)
	}
	if !containsString(readOnlyPaths, readOnly) || containsString(readOnlyPaths, readWrite) {
		t.Fatalf("read-only write-deny roots not preserved: %v", readOnlyPaths)
	}
	if env["WORKFLOW_FOLDER_REFERENCE_DATA"] != readOnly || env["WORKFLOW_FOLDER_RTS_SOURCE"] != readWrite {
		t.Fatalf("alias environment incorrect: %#v", env)
	}
	if _, exists := env["WORKFLOW_FOLDER_MISSING"]; exists {
		t.Fatal("missing host folder should not become a runtime capability")
	}

	sessionID := "attached-folder-read-only-test"
	t.Cleanup(func() { common.ClearSessionShellConfig(sessionID) })
	common.SetSessionFolderGuard(sessionID, reads, writes)
	common.SetSessionFolderGuardBlockedWritePaths(sessionID, []string{"Workflow/attached-folders/planning"})
	configureWorkflowFolderAccessSession(sessionID, workspacePath, readOnlyPaths, env)
	session := common.GetSessionShellConfig(sessionID)
	if session == nil || !containsString(session.BlockedWritePaths, readOnly) || !containsString(session.BlockedWritePaths, "Workflow/attached-folders/planning") {
		t.Fatalf("read-only attachment did not merge into session write denies: %#v", session)
	}
}

func TestRefreshWorkflowFolderAccessSessionRestoresGrantAfterBaseGuardReset(t *testing.T) {
	docsRoot := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsRoot)
	workspacePath := "Workflow/restored-builder"
	workflowDir := filepath.Join(docsRoot, filepath.FromSlash(workspacePath))
	attached := t.TempDir()
	attached, _ = filepath.EvalSymlinks(attached)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := workflowFolderAccessManifest{FolderAccess: []workflowtypes.WorkflowFolderGrant{{
		ID: "source", Alias: "public-website", Path: attached, Access: workflowtypes.FolderAccessReadWrite,
	}}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "workflow.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	sessionID := "restored-builder-folder-access"
	t.Cleanup(func() { common.ClearSessionShellConfig(sessionID) })
	common.SetSessionWorkingDir(sessionID, workspacePath)
	common.SetSessionFolderGuard(sessionID, []string{workspacePath, "Downloads"}, []string{workspacePath, "Downloads"})

	RefreshWorkflowFolderAccessSession(sessionID, workspacePath)
	cfg := common.GetSessionShellConfig(sessionID)
	if cfg == nil || !containsString(cfg.ReadPaths, attached) || !containsString(cfg.WritePaths, attached) {
		t.Fatalf("restored session did not receive attached folder: %#v", cfg)
	}
	if cfg.Env["WORKFLOW_FOLDER_PUBLIC_WEBSITE"] != attached {
		t.Fatalf("restored session did not receive folder alias env: %#v", cfg.Env)
	}
}
