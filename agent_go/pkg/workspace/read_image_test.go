package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeReadImageAbsolutePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	absoluteInput := filepath.Join(root, "_users", "default", "Chats", "sample.png")
	absolutePath, guardPath, err := normalizeReadImageAbsolutePath(absoluteInput)
	if err != nil {
		t.Fatalf("normalizeReadImageAbsolutePath returned error: %v", err)
	}
	if absolutePath != absoluteInput {
		t.Fatalf("absolutePath = %q, want %q", absolutePath, absoluteInput)
	}
	wantGuardPath := filepath.Join("_users", "default", "Chats", "sample.png")
	if guardPath != wantGuardPath {
		t.Fatalf("guardPath = %q, want %q", guardPath, wantGuardPath)
	}
}

func TestNormalizeReadImageAbsolutePathRejectsRelativePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	_, _, err := normalizeReadImageAbsolutePath("Downloads/sample.png")
	if err == nil {
		t.Fatal("expected relative read_image path to be rejected")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %q, want mention of absolute path", err.Error())
	}
}

func TestNormalizeReadImageAbsolutePathRejectsOutsideWorkspaceDocs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace-docs")
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	_, _, err := normalizeReadImageAbsolutePath(filepath.Join(t.TempDir(), "sample.png"))
	if err == nil {
		t.Fatal("expected path outside workspace-docs to be rejected")
	}
	if !strings.Contains(err.Error(), "workspace docs root") {
		t.Fatalf("error = %q, want mention of workspace docs root", err.Error())
	}
}

func TestReadImageRejectsRelativePathBeforeWorkspaceAPI(t *testing.T) {
	client := NewClient("http://127.0.0.1:1")
	_, err := client.ReadImage(context.Background(), ReadImageParams{
		Filepath: "Downloads/sample.png",
		Query:    "What is in this image?",
	})
	if err == nil {
		t.Fatal("expected relative read_image path to be rejected before API call")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %q, want mention of absolute path", err.Error())
	}
}
