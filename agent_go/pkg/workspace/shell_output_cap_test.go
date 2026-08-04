package workspace

import (
	"strings"
	"testing"
)

// The sizes a coding CLI actually rejected on 2026-08-04 must survive the cap.
func TestAgentShellOutputStaysUnderTheObservedRejectionSizes(t *testing.T) {
	for _, size := range []int{67930, 80857, 103685, 130046} {
		capped := capShellResultForAgent(ShellCommandResult{Stdout: strings.Repeat("x", size)})
		total := len(capped.Stdout) + len(capped.Stderr)
		if total > defaultAgentShellOutputBytes {
			t.Errorf("%d-character output capped to %d, over the %d limit", size, total, defaultAgentShellOutputBytes)
		}
	}
}

// Both ends must survive: the head says what ran, the tail says how it finished.
func TestTruncationKeepsHeadAndTail(t *testing.T) {
	body := "HEAD-MARKER" + strings.Repeat("x", 200000) + "TAIL-MARKER"
	capped := capShellResultForAgent(ShellCommandResult{Stdout: body})
	if !strings.HasPrefix(capped.Stdout, "HEAD-MARKER") {
		t.Error("head dropped")
	}
	if !strings.HasSuffix(capped.Stdout, "TAIL-MARKER") {
		t.Error("tail dropped")
	}
}

// An agent told only "truncated" re-runs the identical command; three of the
// 2026-08-04 rejections were byte-identical repeats.
func TestTruncationMarkerForbidsAnIdenticalRetryAndNamesAlternatives(t *testing.T) {
	capped := capShellResultForAgent(ShellCommandResult{Stdout: strings.Repeat("x", 200000)})
	for _, want := range []string{"Do NOT re-run this command unchanged", "grep", "head/tail", "sed -n", "jq/awk"} {
		if !strings.Contains(capped.Stdout, want) {
			t.Errorf("marker omits %q", want)
		}
	}
	if !strings.Contains(capped.Stdout, "of 200000 characters") {
		t.Errorf("marker does not state the real size: %s", capped.Stdout[:400])
	}
}

// stderr carries the reason for a failure and must not be pushed out by stdout.
func TestHugeStdoutDoesNotEvictStderr(t *testing.T) {
	capped := capShellResultForAgent(ShellCommandResult{
		Stdout:   strings.Repeat("x", 300000),
		Stderr:   "sh: line 0: cd: Workflow/build-in-public: No such file or directory\n",
		ExitCode: 1,
	})
	if !strings.Contains(capped.Stderr, "No such file or directory") {
		t.Fatalf("stderr lost: %q", capped.Stderr)
	}
	if capped.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", capped.ExitCode)
	}
}

// Output that already fits is returned byte-for-byte.
func TestOutputUnderTheCapIsUntouched(t *testing.T) {
	original := ShellCommandResult{Stdout: "hello", Stderr: "warn", ExitCode: 0}
	if capped := capShellResultForAgent(original); capped != original {
		t.Errorf("small result modified: %#v", capped)
	}
}

func TestCapCanBeDisabledForADifferentConsumer(t *testing.T) {
	t.Setenv("SHELL_MAX_AGENT_OUTPUT_BYTES", "0")
	body := strings.Repeat("x", 200000)
	if capped := capShellResultForAgent(ShellCommandResult{Stdout: body}); capped.Stdout != body {
		t.Errorf("cap applied when disabled: %d characters", len(capped.Stdout))
	}
}

func TestCapHonoursAnOperatorOverride(t *testing.T) {
	t.Setenv("SHELL_MAX_AGENT_OUTPUT_BYTES", "1000")
	capped := capShellResultForAgent(ShellCommandResult{Stdout: strings.Repeat("x", 50000)})
	if len(capped.Stdout) > 1000 {
		t.Errorf("override ignored: %d characters", len(capped.Stdout))
	}
}
