package browser

import (
	"strings"
	"testing"
)

func TestMissingCDPPageActionTabErrorKeepsPlaceholderWhenNoTabSupplied(t *testing.T) {
	err := missingCDPPageActionTabError(9222, "snapshot", nil, "No selected CDP tab for this workflow yet.")
	msg := err.Error()

	if strings.Contains(msg, "bare positional") {
		t.Errorf("must not claim a tab was supplied when none was, got:\n%s", msg)
	}
	if !strings.Contains(msg, "<tab-id-or-label>") {
		t.Errorf("expected the placeholder retry hint, got:\n%s", msg)
	}
	for _, want := range []string{
		`agent_browser(command="status", args=[], session="<same-session>")`,
		"status needs no tab and no --cdp argument",
		`do not use "snapshot": it is a page action`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected connectivity guidance %q, got:\n%s", want, msg)
		}
	}
}

// Recovery must stay narrow: only the unambiguous tN form, and never a value
// sitting in a flag's slot.
func TestUnmarkedCDPTabArgIgnoresFlagValues(t *testing.T) {
	// "t7" here is the value of --text, not a tab selection.
	tab, rest := unmarkedCDPTabArg([]string{"--ref", "e5", "--text", "t7"})
	if tab != "" {
		t.Errorf("flag value must not be treated as a tab, got %q", tab)
	}
	if len(rest) != 4 {
		t.Errorf("args must be left intact, got %v", rest)
	}

	tab, rest = unmarkedCDPTabArg([]string{"t7", "--ref", "e5"})
	if tab != "t7" {
		t.Errorf("expected t7, got %q", tab)
	}
	if strings.Join(rest, " ") != "--ref e5" {
		t.Errorf("expected the tab stripped from the remaining args, got %v", rest)
	}
}
