package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/workspace/models"
)

func TestDiffPatchExternalFileAppliesInsideApprovedRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	target := filepath.Join(root, "notes.md")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(externalDiffPatchRequest{
		Filepath: target,
		Diff:     "--- a/notes.md\n+++ b/notes.md\n@@ -1 +1 @@\n-old\n+new\n",
		FolderGuard: &models.FolderGuardConfig{
			Enabled: true, WritePaths: []string{root},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/external/diff", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	DiffPatchExternalFile(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	updated, err := os.ReadFile(target)
	if err != nil || string(updated) != "new\n" {
		t.Fatalf("external patch result = %q err %v", updated, err)
	}
}

func TestAuthorizeExternalWriteTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes.md")
	guard := &models.FolderGuardConfig{Enabled: true, WritePaths: []string{root}}

	got, err := authorizeExternalWriteTarget(target, guard)
	want, canonicalErr := canonicalTargetForWrite(target)
	if err != nil || canonicalErr != nil || got != want {
		t.Fatalf("approved target rejected: got %q want %q err %v canonicalErr %v", got, want, err, canonicalErr)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if _, err := authorizeExternalWriteTarget(outside, guard); err == nil {
		t.Fatal("outside target should be rejected")
	}

	guard.BlockedWritePaths = []string{root}
	if _, err := authorizeExternalWriteTarget(target, guard); err == nil {
		t.Fatal("blocked write target should be rejected")
	}
}

func TestAuthorizeExternalWriteTargetRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	guard := &models.FolderGuardConfig{Enabled: true, WritePaths: []string{root}}
	if _, err := authorizeExternalWriteTarget(filepath.Join(link, "file.md"), guard); err == nil {
		t.Fatal("symlink escape should be rejected")
	}
}
