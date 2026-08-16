package subagents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The workspace API reports a missing document as HTTP 200 with success:true,
// carrying the reason only in error/ with an empty filepath — a contract five
// other call sites implement by hand (cmd/server/workflow.go,
// services/workspace_config.go, pkg/chathistory, pkg/workspace, pkg/costledger).
// This client did not, so a missing SUBAGENT.md read back as "" and the parser
// reported it as malformed frontmatter instead of absent.
func TestReadFileDetectsTheAPIsSuccessfulNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "File does not exist",
			"error":   "File not found: subagents/missing/SUBAGENT.md",
			"data":    map[string]interface{}{"filepath": "", "content": ""},
		})
	}))
	defer server.Close()

	_, err := NewWorkspaceAPIClient(server.URL).ReadFile("subagents/missing/SUBAGENT.md")
	if err == nil {
		t.Fatal("a not-found answered with success:true must not read back as empty content")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want a not-found error, got: %v", err)
	}
}

func TestParseSubAgentFileDoesNotBlameFrontmatterForEmptyContent(t *testing.T) {
	_, _, err := ParseSubAgentFile("")
	if err == nil {
		t.Fatal("empty content must be an error")
	}
	if strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("empty content must not be reported as a frontmatter defect: %v", err)
	}
}
