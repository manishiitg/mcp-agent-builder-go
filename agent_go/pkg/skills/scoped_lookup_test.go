package skills

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The workspace API answers a missing document with HTTP 200 AND success:true,
// putting the failure only in error/ with an empty filepath. ReadFile decoded
// neither field, returned the empty Content, and ParseSkillFile blamed the
// frontmatter — so a skill that was merely in a different folder was reported
// as malformed.
func TestReadFileTreatsTheAPIsSuccessfulNotFoundAsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "File does not exist",
			"error":   "File not found: skills/missing/SKILL.md",
			"data":    map[string]interface{}{"filepath": "", "content": ""},
		})
	}))
	defer server.Close()

	_, err := NewWorkspaceAPIClient(server.URL).ReadFile("skills/missing/SKILL.md")
	if err == nil {
		t.Fatal("a not-found answered with success:true must not read back as empty content")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should say the file is missing, got: %v", err)
	}
}

func TestParseSkillFileDoesNotBlameFrontmatterForEmptyContent(t *testing.T) {
	_, _, err := ParseSkillFile("")
	if err == nil {
		t.Fatal("empty content must be an error")
	}
	if strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("empty content must not be reported as a frontmatter defect: %v", err)
	}
}

// A product may install its skills into its own project folder rather than the
// user-level skills/ folder. The unscoped lookup only ever read the latter, so
// those skills could never attach.
func TestGetSkillInPrefersTheWorkspaceThenFallsBack(t *testing.T) {
	const body = "---\nname: hyperframes\ndescription: router\n---\n\nBody text.\n"
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/documents/")
		requested = append(requested, path)
		if strings.Contains(path, "projects%2Fdemo") || strings.Contains(path, "projects/demo") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    map[string]interface{}{"filepath": path, "content": body},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "message": "File does not exist",
			"error": "File not found", "data": map[string]interface{}{"filepath": "", "content": ""},
		})
	}))
	defer server.Close()

	skill, err := GetSkillIn(server.URL, "projects/demo", "hyperframes")
	if err != nil {
		t.Fatalf("workspace-scoped skill was not found: %v (tried %v)", err, requested)
	}
	// The path handed back drives the lazy-excerpt pointer, and the agent reads
	// it relative to its own workspace root — an absolute-ish path would not
	// resolve under its folder guard.
	if skill.FilePath != "skills/hyperframes/SKILL.md" {
		t.Fatalf("FilePath = %q, want the workspace-relative path", skill.FilePath)
	}
}
