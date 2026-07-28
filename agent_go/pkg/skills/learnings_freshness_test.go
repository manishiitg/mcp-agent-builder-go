package skills

import (
	"strings"
	"testing"
)

const ledgerWithStale = `{
  "store": "learnings",
  "items": {
    "references/test-coverage-audit.md": {
      "last_confirmed_at": "2026-07-19T04:01:33Z",
      "stale_since": "2026-07-22T09:00:00Z",
      "stale_reason": "soul.md edit (objective, success criteria, or constraints)"
    },
    "references/auth-flow.md": {
      "last_confirmed_at": "2026-07-23T13:28:32Z"
    }
  }
}`

// The whole point: a retired rule must not look identical to a current one at the
// moment the agent picks a file to read.
func TestCollectStaleLearningsReturnsOnlyContradictedItems(t *testing.T) {
	got := collectStaleLearnings(ledgerWithStale)
	if len(got) != 1 {
		t.Fatalf("expected 1 stale item, got %#v", got)
	}
	if got[0].Name != "references/test-coverage-audit.md" {
		t.Fatalf("wrong item: %#v", got[0])
	}
	if !strings.Contains(got[0].StaleReason, "soul.md") {
		t.Fatalf("reason should explain what invalidated it: %#v", got[0])
	}
}

// A healthy workflow must add nothing to every session's prompt.
func TestCollectStaleLearningsQuietWhenClean(t *testing.T) {
	clean := `{"store":"learnings","items":{"references/a.md":{"last_confirmed_at":"2026-07-23T00:00:00Z"}}}`
	for name, raw := range map[string]string{
		"clean ledger": clean,
		"empty":        "",
		"no items":     `{"store":"learnings"}`,
	} {
		if got := collectStaleLearnings(raw); len(got) != 0 {
			t.Fatalf("%s should yield nothing, got %#v", name, got)
		}
	}
}

// A malformed ledger must stay silent rather than guess. Warning on bad evidence
// would train agents to ignore the flag entirely.
func TestCollectStaleLearningsSilentOnMalformedLedger(t *testing.T) {
	if got := collectStaleLearnings("{not json"); got != nil {
		t.Fatalf("malformed ledger must not produce warnings, got %#v", got)
	}
}

func TestRenderStaleLearningsWarningTellsAgentWhatToDo(t *testing.T) {
	out := renderStaleLearningsWarning(collectStaleLearnings(ledgerWithStale))
	for _, want := range []string{
		"contradicted by a newer decision",
		"references/test-coverage-audit.md",
		"stale since 2026-07-22T09:00:00Z",
		"soul.md",
		// The agent needs an action, not just a flag: prefer current sources and
		// escalate, rather than silently choosing one.
		"CONCERNS:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("warning missing %q:\n%s", want, out)
		}
	}
	if renderStaleLearningsWarning(nil) != "" {
		t.Fatal("no stale items must render nothing at all")
	}
}

// This block ships in every session that attaches the skill, so a workflow with
// a large invalidated store must not trade one context problem for another.
func TestRenderStaleLearningsWarningCapsTheList(t *testing.T) {
	var entries []staleLearningEntry
	for i := 0; i < 12; i++ {
		entries = append(entries, staleLearningEntry{Name: "references/f.md", StaleSince: "2026-07-22T00:00:00Z"})
	}
	out := renderStaleLearningsWarning(entries)
	if strings.Count(out, "- `references/f.md`") != maxListedStaleLearnings {
		t.Fatalf("expected %d listed entries:\n%s", maxListedStaleLearnings, out)
	}
	if !strings.Contains(out, "and 4 more") || !strings.Contains(out, "_freshness.json") {
		t.Fatalf("truncation must point at the full set:\n%s", out)
	}
}
