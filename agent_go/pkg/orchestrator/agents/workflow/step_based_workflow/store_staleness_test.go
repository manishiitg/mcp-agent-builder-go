package step_based_workflow

import (
	"strings"
	"testing"
	"time"
)

func ledgerWith(items map[string]ItemFreshness) *FreshnessLedger {
	return &FreshnessLedger{Store: "learnings", Items: items}
}

// The live failure this exists for: soul.md changed on 2026-07-22 and a learnings
// reference last confirmed 2026-07-19 kept being served for a week. Items that
// predate the edit must be marked; items confirmed after it must not.
func TestMarkStaleItemsFlagsOnlyItemsPredatingTheEdit(t *testing.T) {
	edit := authorityEdit{
		At:          time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
		Description: "soul.md edit (objective, success criteria, or constraints)",
	}
	ledger := ledgerWith(map[string]ItemFreshness{
		"references/test-coverage-audit.md": {LastConfirmedAt: "2026-07-19T04:01:33Z"},
		"references/auth-flow.md":           {LastConfirmedAt: "2026-07-23T13:28:32Z"},
	})

	stale, changed := markStaleItems(ledger, edit)
	if !changed {
		t.Fatal("expected the ledger to change")
	}
	if len(stale) != 1 || stale[0] != "references/test-coverage-audit.md" {
		t.Fatalf("stale set = %#v", stale)
	}
	if got := ledger.Items["references/test-coverage-audit.md"]; got.StaleSince == "" || !strings.Contains(got.StaleReason, "soul.md") {
		t.Fatalf("stale item not stamped with a reason: %#v", got)
	}
	if got := ledger.Items["references/auth-flow.md"]; got.StaleSince != "" {
		t.Fatalf("item confirmed after the edit must not be marked: %#v", got)
	}
}

// Re-confirming an item after the edit means a run has now seen it in the new
// world. Leaving a permanent stale mark would make the signal useless — every
// item would eventually be flagged forever.
func TestMarkStaleItemsClearsMarkOnceReconfirmed(t *testing.T) {
	edit := authorityEdit{At: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), Description: "plan edit — x"}
	ledger := ledgerWith(map[string]ItemFreshness{
		"references/a.md": {LastConfirmedAt: "2026-07-25T00:00:00Z", StaleSince: "2026-07-22T00:00:00Z", StaleReason: "plan edit — x"},
	})
	stale, changed := markStaleItems(ledger, edit)
	if len(stale) != 0 {
		t.Fatalf("expected nothing stale, got %#v", stale)
	}
	if !changed {
		t.Fatal("clearing a stale mark is a change")
	}
	if got := ledger.Items["references/a.md"]; got.StaleSince != "" || got.StaleReason != "" {
		t.Fatalf("stale mark should be cleared: %#v", got)
	}
}

// Re-running against the same edit must not rewrite the ledger. Otherwise every
// run churns the file and its own git history stops being readable.
func TestMarkStaleItemsIsIdempotentForTheSameEdit(t *testing.T) {
	edit := authorityEdit{At: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), Description: "plan edit — x"}
	ledger := ledgerWith(map[string]ItemFreshness{
		"references/a.md": {LastConfirmedAt: "2026-07-19T00:00:00Z"},
	})
	if _, changed := markStaleItems(ledger, edit); !changed {
		t.Fatal("first pass should stamp")
	}
	stale, changed := markStaleItems(ledger, edit)
	if len(stale) != 1 {
		t.Fatalf("item is still stale, got %#v", stale)
	}
	if changed {
		t.Fatal("second pass against the same edit must not rewrite the ledger")
	}
}

// An item with no confirmation date has never been reviewed by any run. That is a
// coverage gap, not evidence that this particular edit invalidated it — asserting
// the link would fabricate a cause.
func TestMarkStaleItemsIgnoresNeverConfirmedItems(t *testing.T) {
	edit := authorityEdit{At: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), Description: "plan edit — x"}
	ledger := ledgerWith(map[string]ItemFreshness{
		"references/never.md": {},
		"references/bad.md":   {LastConfirmedAt: "not-a-date"},
	})
	stale, changed := markStaleItems(ledger, edit)
	if len(stale) != 0 || changed {
		t.Fatalf("unconfirmed items must not be attributed to an edit: stale=%#v changed=%v", stale, changed)
	}
}

func TestTruncateItemListKeepsConcernReadable(t *testing.T) {
	got := truncateItemList([]string{"a", "b", "c"}, 8)
	if len(got) != 3 {
		t.Fatalf("short list should pass through, got %#v", got)
	}
	long := make([]string, 12)
	for i := range long {
		long[i] = "item"
	}
	got = truncateItemList(long, 8)
	if len(got) != 9 || !strings.Contains(got[8], "and 4 more") {
		t.Fatalf("long list should be summarized, got %#v", got)
	}
}
