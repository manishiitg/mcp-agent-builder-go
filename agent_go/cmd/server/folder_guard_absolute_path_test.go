package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
)

// The folder guard compares tool path arguments against workspace-relative
// allow entries. An absolute argument never prefix-matched, so a legitimate
// write was denied against an allow list that contained its parent:
//
//	cannot write to '/Users/.../workspace-docs/Workflow/social-media/builder/improve.html'
//	(allowed write folders: [Downloads Workflow/social-media ...])
//
// Prompts actively tell agents to use absolute workspace paths, and the
// workspace client normalizes the same way — but only after this guard runs.
func TestAbsoluteWorkspacePathNormalizesIntoAllowedFolder(t *testing.T) {
	root := t.TempDir()
	docsRoot := filepath.Join(root, "workspace-docs")
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKSPACE_DOCS_PATH", docsRoot)

	absolute := filepath.Join(docsRoot, "Workflow", "social-media", "builder", "improve.html")

	relative, ok := orchestrator.NormalizeAbsoluteWorkspaceDocsPath(absolute)
	if !ok {
		t.Fatalf("NormalizeAbsoluteWorkspaceDocsPath(%q) = not normalized, want workspace-relative", absolute)
	}
	if relative != filepath.Join("Workflow", "social-media", "builder", "improve.html") {
		t.Fatalf("relative = %q, want Workflow/social-media/builder/improve.html", relative)
	}

	// The denial in production: relative form matches, absolute form does not.
	const allowed = "Workflow/social-media"
	if !isPathAllowedByFolderGuard(filepath.Clean(relative), allowed) {
		t.Fatalf("normalized path %q not allowed under %q", relative, allowed)
	}
	if isPathAllowedByFolderGuard(filepath.Clean(absolute), allowed) {
		t.Fatal("absolute path matched a relative allow entry; the normalization step would be untested")
	}
}

// Paths outside every workspace root must stay unnormalized so the guard still
// denies them.
func TestAbsolutePathOutsideWorkspaceIsNotNormalized(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	if _, ok := orchestrator.NormalizeAbsoluteWorkspaceDocsPath("/etc/passwd"); ok {
		t.Fatal("/etc/passwd normalized into the workspace; the guard would then allow it")
	}
}
