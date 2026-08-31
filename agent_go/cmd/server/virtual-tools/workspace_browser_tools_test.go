package virtualtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

// PLAT-200: spillOversizedBrowserOutput must never invent a write location --
// it only writes when the calling session's OWN already-granted ReadPaths
// prove tool_output_folder is readable back by that session (the PLAT-078
// grant), and it must decline cleanly (no panic, a plain error) in every
// other case rather than guess.

func TestSpillOversizedBrowserOutputDeclinesWithNoSessionOnContext(t *testing.T) {
	_, err := spillOversizedBrowserOutput(context.Background(), "some large tree")
	if err == nil {
		t.Fatal("want an error when ctx carries no session id, got nil")
	}
}

func TestSpillOversizedBrowserOutputDeclinesWhenSessionHasNoFolderGuard(t *testing.T) {
	sessionID := "spill-test-no-guard"
	ctx := context.WithValue(context.Background(), common.ChatSessionIDKey, sessionID)
	_, err := spillOversizedBrowserOutput(ctx, "some large tree")
	if err == nil {
		t.Fatal("want an error when the session has no folder guard configured, got nil")
	}
}

func TestSpillOversizedBrowserOutputDeclinesWhenSessionHasNoToolOutputFolderGrant(t *testing.T) {
	sessionID := "spill-test-no-tool-output-grant"
	// A real guard, but one that never granted tool_output_folder -- e.g. a
	// step whose folder guard predates PLAT-078, or a narrowly scoped session.
	common.SetSessionFolderGuard(sessionID, []string{"Workflow/testing/execution"}, []string{"Workflow/testing/execution/step-one"})

	ctx := context.WithValue(context.Background(), common.ChatSessionIDKey, sessionID)
	_, err := spillOversizedBrowserOutput(ctx, "some large tree")
	if err == nil {
		t.Fatal("want an error when the session's granted read paths do not include tool_output_folder, got nil")
	}
}

func TestSpillOversizedBrowserOutputWritesUnderTheSessionsGrantedToolOutputFolder(t *testing.T) {
	var gotMethod, gotPath, gotContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		var body struct {
			Content string `json:"content"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotContent = body.Content
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}))
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	sessionID := "spill-test-has-grant"
	common.SetSessionFolderGuard(sessionID,
		[]string{"Workflow/testing/execution", "Workflow/testing/tool_output_folder"},
		[]string{"Workflow/testing/execution/step-one"},
	)

	ctx := context.WithValue(context.Background(), common.ChatSessionIDKey, sessionID)
	content := "a very large accessibility tree that would not fit inline"
	relPath, err := spillOversizedBrowserOutput(ctx, content)
	if err != nil {
		t.Fatalf("want a successful spill, got error: %v", err)
	}
	if !strings.HasPrefix(relPath, "Workflow/testing/tool_output_folder/") {
		t.Fatalf("spilled path %q is not under the session's granted tool_output_folder", relPath)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("want PUT to /api/documents/..., got method %s", gotMethod)
	}
	if !strings.Contains(gotPath, "tool_output_folder") {
		t.Fatalf("write request path %q did not target tool_output_folder", gotPath)
	}
	if gotContent != content {
		t.Fatalf("write request content = %q, want the full untruncated content %q", gotContent, content)
	}
}
