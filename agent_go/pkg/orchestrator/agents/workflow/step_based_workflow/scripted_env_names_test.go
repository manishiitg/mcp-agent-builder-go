package step_based_workflow

import (
	"strings"
	"testing"
)

// The declared list is the only place a script author learns which env vars
// exist. When it omitted DB_PATH, authors concluded the database was unreachable
// and routed around it — one workflow derived the path from STEP_EXECUTION_DIR,
// another shelled out to the sqlite3 CLI and wrote that into permanent learnings.
// Both are forbidden by the stores contract, which requires $DB_PATH and says an
// open failure must be reported rather than worked around. Every name the runtime
// injects has to be listed here or the framework contradicts itself.
func TestScriptedEnvVarNamesDeclareEverythingTheRuntimeInjects(t *testing.T) {
	got := buildScriptedEnvVarNamesForPrompt(true, map[string]string{
		"VAR_SITE_URL":           "https://example.test",
		"SECRET_MEMBER_PASSWORD": "x",
		"MCP_API_URL":            "http://localhost:1",
	})
	lines := strings.Split(got, "\n")

	// execScriptedScript sets these four plus the workspace env; all must appear.
	for _, want := range []string{"STEP_OUTPUT_DIR", "STEP_EXECUTION_DIR", "DB_PATH", "RUN_FOLDER", "MCP_API_URL", "VAR_SITE_URL", "SECRET_MEMBER_PASSWORD"} {
		found := false
		for _, l := range lines {
			if l == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("declared env list missing %q:\n%s", want, got)
		}
	}

	// A duplicate would read as two different variables to the author.
	seen := map[string]int{}
	for _, l := range lines {
		seen[l]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Fatalf("%q declared %d times:\n%s", name, n, got)
		}
	}
}

// Workspace env supplying a name already declared must not duplicate it.
func TestScriptedEnvVarNamesDedupesAgainstWorkspaceEnv(t *testing.T) {
	got := buildScriptedEnvVarNamesForPrompt(true, map[string]string{
		"DB_PATH":     "/somewhere/db.sqlite",
		"MCP_API_URL": "http://localhost:1",
	})
	if n := strings.Count(got, "DB_PATH"); n != 1 {
		t.Fatalf("DB_PATH appears %d times:\n%s", n, got)
	}
}

func TestScriptedEnvVarNamesEmptyWhenNotScripted(t *testing.T) {
	if got := buildScriptedEnvVarNamesForPrompt(false, map[string]string{"VAR_X": "1"}); got != "" {
		t.Fatalf("non-scripted mode should declare nothing, got %q", got)
	}
}
