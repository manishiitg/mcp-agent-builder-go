package step_based_workflow

import (
	"strings"
	"testing"
)

func TestBuildStepKBGuidanceWithTargetUsesShellForPermittedWrites(t *testing.T) {
	target := "/app/workspace-docs/Workflow/social-media/knowledgebase/notes"
	prompt := BuildStepKBGuidanceWithTarget(KBAccessReadWrite, "Capture durable audience facts.", target)

	required := []string{
		"Knowledgebase contribution",
		"**Target:** `" + target + "/`",
		"Use these exact paths; do not rely on your shell working directory",
		"execute_shell_command",
		"including new topic files",
		"preserve unrelated content",
	}
	for _, snippet := range required {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected prompt to contain %q\n\nPrompt:\n%s", snippet, prompt)
		}
	}

	forbidden := []string{
		"Write with shell heredoc",
		"cat > file <<EOF",
		"Use heredoc",
	}
	for _, snippet := range forbidden {
		if strings.Contains(prompt, snippet) {
			t.Fatalf("prompt still contains forbidden write guidance %q\n\nPrompt:\n%s", snippet, prompt)
		}
	}
}

func TestBuildKBContributionReviewMessageWithTargetUsesShell(t *testing.T) {
	target := "/app/workspace-docs/Workflow/social-media/knowledgebase/notes"
	prompt := BuildKBContributionReviewMessageWithTarget(KBAccessReadWrite, "Capture durable audience facts.", target)

	required := []string{
		"**Target:** `" + target + "/`",
		"Use these exact paths; do not rely on cwd",
		"execute_shell_command",
		"Preserve unrelated content",
	}
	for _, snippet := range required {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected prompt to contain %q\n\nPrompt:\n%s", snippet, prompt)
		}
	}
}
