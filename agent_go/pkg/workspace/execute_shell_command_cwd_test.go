package workspace

import (
	"strings"
	"testing"
)

// A failed command must say where it ran. The cwd is per-session but the agent
// only ever sees workspace-rooted paths, so `cd Workflow/x` issued from inside
// Workflow/x fails identically to a genuinely missing folder.
func TestFailedShellCommandReportsItsWorkingDirectory(t *testing.T) {
	result := ShellCommandResult{
		Stderr:   "sh: line 0: cd: Workflow/build-in-public: No such file or directory\n",
		ExitCode: 1,
	}
	hint := shellWorkingDirectoryHint(result, "Workflow/build-in-public")
	if !strings.Contains(hint, "Workflow/build-in-public") {
		t.Fatalf("hint does not name the working directory: %q", hint)
	}
	if !strings.Contains(hint, "not the workspace docs root") {
		t.Fatalf("hint does not correct the docs-root assumption: %q", hint)
	}
	// stderr already ends in a newline; the hint must not add a blank line.
	if strings.HasPrefix(hint, "\n") {
		t.Fatalf("hint double-spaces newline-terminated stderr: %q", hint)
	}
}

func TestShellWorkingDirectoryHintSeparatesUnterminatedStderr(t *testing.T) {
	hint := shellWorkingDirectoryHint(ShellCommandResult{Stderr: "boom", ExitCode: 2}, "Workflow/demo")
	if !strings.HasPrefix(hint, "\n") {
		t.Fatalf("hint runs into unterminated stderr: %q", hint)
	}
}

// A successful command pays nothing for the hint.
func TestSuccessfulShellCommandCarriesNoHint(t *testing.T) {
	if hint := shellWorkingDirectoryHint(ShellCommandResult{ExitCode: 0}, "Workflow/demo"); hint != "" {
		t.Fatalf("success carries a hint: %q", hint)
	}
	if hint := shellWorkingDirectoryHint(ShellCommandResult{ExitCode: 1}, ""); hint != "" {
		t.Fatalf("unknown cwd invents one: %q", hint)
	}
}

// The one host path an agent is instructed to read must not be answered with a
// suggestion that cannot work. The CLI spills an oversized tool result outside
// the workspace and tells the agent to re-read it with offset/limit; "use
// workspace-relative paths" points at a file that does not exist there.
func TestToolResultSpillDenialGivesTheOnlyRecoveryThatWorks(t *testing.T) {
	spill := "/Users/mipl/.claude/projects/-Users-mipl-ai-work-mcp-agent-builder-go-workspace-docs-Workflow-build-in-public/720e9633-5f5c-45b5-9138-8df5e91e3011/tool-results/mcp-api-bridge-execute_shell_command-1785814553588.txt"
	msg := codingCLIToolResultSpillDenial(spill)
	if msg == "" {
		t.Fatal("spill path not recognized")
	}
	if strings.Contains(msg, "workspace-relative") && !strings.Contains(msg, "no workspace-relative path points at it") {
		t.Errorf("denial still suggests a workspace path: %s", msg)
	}
	for _, want := range []string{"grep", "head/tail", "sed -n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("denial does not name %q as a way to produce less output: %s", want, msg)
		}
	}
}

// Ordinary host paths keep the generic denial, which correctly points at the
// workspace roots.
func TestNonSpillHostPathsKeepTheGenericDenial(t *testing.T) {
	for _, path := range []string{
		"/Users/mipl/ai-work/mcp-agent-builder-go",
		"/Users/mipl/.claude/projects/some-slug/session/transcript.jsonl",
		"/home/agent/tool-results/output.txt",
	} {
		if msg := codingCLIToolResultSpillDenial(path); msg != "" {
			t.Errorf("%s wrongly treated as a CLI spill: %s", path, msg)
		}
	}
}
