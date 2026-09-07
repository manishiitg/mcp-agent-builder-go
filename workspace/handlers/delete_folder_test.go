package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/coding-agent-loop/workspace/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// Gin's wildcard route capture always includes the leading slash
// ("/Chats/foo" for a request to /api/folders/Chats/foo). Without stripping
// it, SanitizeInputPath sees what looks like an absolute path, IsPerUserPath's
// prefix match never fires, resolution skips the _users/<id>/ prefix
// entirely, and the handler reports "Folder not found" for a folder that
// genuinely exists — confirmed live against a real desktop deployment before
// this fix (the delete silently no-op'd while the caller saw success).
func TestDeleteFolderResolvesPerUserPathDespiteWildcardLeadingSlash(t *testing.T) {
	docsDir, cleanup := setupTestDocsDir(t)
	defer cleanup()

	target := filepath.Join(docsDir, utils.UsersDirectory, "default", "Chats", "SparkQuill", "activities", "old-one")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "activity.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write activity.json: %v", err)
	}

	gin.SetMode(gin.TestMode)
	viper.Set("docs-dir", docsDir)
	r := gin.New()
	r.DELETE("/api/folders/*folderpath", DeleteFolder)

	req, _ := http.NewRequest("DELETE", "/api/folders/Chats/SparkQuill/activities/old-one?confirm=true", nil)
	req.Header.Set("X-User-ID", "default")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("folder still exists on disk after a reported-successful delete: %v", err)
	}
}

// Refusing an unconfirmed delete must not depend on the leading-slash fix —
// this stays a 400 either way.
func TestDeleteFolderRequiresConfirmation(t *testing.T) {
	docsDir, cleanup := setupTestDocsDir(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	viper.Set("docs-dir", docsDir)
	r := gin.New()
	r.DELETE("/api/folders/*folderpath", DeleteFolder)

	req, _ := http.NewRequest("DELETE", "/api/folders/Chats/SparkQuill/activities/old-one", nil)
	req.Header.Set("X-User-ID", "default")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without confirm=true, got %d: %s", w.Code, w.Body.String())
	}
}
