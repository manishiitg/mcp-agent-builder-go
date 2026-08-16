package commands

import (
	"strings"
	"testing"
)

// This package reads through skills.NewWorkspaceAPIClient, which now rejects
// the API's successful not-found, so an absent COMMAND.md surfaces as missing.
// Its own parser still had to stop asserting a frontmatter defect it never
// verified.
func TestParseCommandFileDoesNotBlameFrontmatterForEmptyContent(t *testing.T) {
	_, _, err := ParseCommandFile("")
	if err == nil {
		t.Fatal("empty content must be an error")
	}
	if strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("empty content must not be reported as a frontmatter defect: %v", err)
	}
}
