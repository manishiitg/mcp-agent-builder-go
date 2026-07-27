package browser

import (
	"strings"
	"testing"
)

// A bare tab id positional ("t7" instead of "--tab t7") is still rejected, but
// the error must name the token instead of claiming no tab was supplied, and
// the retry hints must mark that token rather than repeating it as a stray arg.
func TestMissingCDPPageActionTabErrorNamesUnmarkedTabPositional(t *testing.T) {
	err := missingCDPPageActionTabError(9222, "snapshot", []string{"t7"}, "No selected CDP tab for this workflow yet.")
	if err == nil {
		t.Fatal("expected an error when the tab is not marked")
	}
	msg := err.Error()

	if !strings.Contains(msg, `Tab "t7" was passed as a bare positional argument`) {
		t.Errorf("error must name the unmarked tab, got:\n%s", msg)
	}
	if strings.Contains(msg, "<tab-id-or-label>") {
		t.Errorf("retry hints must use the supplied tab id, not a placeholder, got:\n%s", msg)
	}
	if !strings.Contains(msg, `"--tab","t7"`) || !strings.Contains(msg, `"tab","t7"`) {
		t.Errorf("retry hints must mark t7 as the tab selection, got:\n%s", msg)
	}
	// The stray positional must not survive into the retry hint, or the
	// suggested command fails the same way.
	if strings.Contains(msg, `"t7","t7"`) {
		t.Errorf("retry hints must drop the stray positional, got:\n%s", msg)
	}
}

func TestMissingCDPPageActionTabErrorKeepsPlaceholderWhenNoTabSupplied(t *testing.T) {
	err := missingCDPPageActionTabError(9222, "snapshot", nil, "No selected CDP tab for this workflow yet.")
	msg := err.Error()

	if strings.Contains(msg, "bare positional") {
		t.Errorf("must not claim a tab was supplied when none was, got:\n%s", msg)
	}
	if !strings.Contains(msg, "<tab-id-or-label>") {
		t.Errorf("expected the placeholder retry hint, got:\n%s", msg)
	}
}

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
