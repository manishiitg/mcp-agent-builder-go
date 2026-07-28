package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
)

// Detects knowledge that a newer owner decision has contradicted.
//
// The failure this addresses, observed in a live workflow: soul.md was edited on
// 2026-07-22 to retire a rule; a learnings reference asserting that rule had been
// last confirmed on 2026-07-19 and was never revisited. It kept being advertised
// in the skill index and served to every run for a week. Nothing was broken —
// there was simply no mechanism that ever compared the two dates.
//
// The freshness ledger already stores each item's last_confirmed_at, and the plan
// changelog already stores every plan edit with a timestamp and a mandatory
// reason. Neither was ever compared to the other. That comparison is all this is.
//
// It deliberately does not try to judge whether an item is genuinely wrong — only
// that it predates a decision and therefore needs review. The judgement belongs to
// learning_health, which now has an evidence-backed trigger instead of having to
// notice on its own.

// authorityEdit is the most recent owner-side change to what the workflow is
// supposed to do: a plan-mod tool call, or a soul.md edit.
type authorityEdit struct {
	At          time.Time
	Description string
}

// newestPlanChangelogEdit scans planning/changelog/ for the most recent entry.
//
// Entries carry a mandatory `reason` supplied at plan-mod time, which is far more
// useful in a staleness report than the tool name alone — it is the human
// rationale for the change that invalidated the knowledge.
func newestPlanChangelogEdit(workspacePath string) (authorityEdit, bool) {
	dir := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath), PlanningFolderName, "changelog")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return authorityEdit{}, false
	}
	var newest authorityEdit
	found := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var file PlanChangelog
		if json.Unmarshal(raw, &file) != nil {
			continue
		}
		for _, entry := range file.Entries {
			ts, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.Timestamp))
			if err != nil {
				continue
			}
			if found && !ts.After(newest.At) {
				continue
			}
			desc := strings.TrimSpace(entry.Reason)
			if desc == "" {
				desc = strings.TrimSpace(entry.Tool)
			}
			if desc == "" {
				desc = "plan edit"
			}
			if len(entry.StepIDs) > 0 {
				desc = fmt.Sprintf("%s (steps: %s)", desc, strings.Join(entry.StepIDs, ", "))
			}
			newest = authorityEdit{At: ts.UTC(), Description: "plan edit — " + desc}
			found = true
		}
	}
	return newest, found
}

// newestSoulEdit uses soul.md's mtime. soul.md is written by shell rather than
// through a typed tool, so there is no changelog entry to read — the file's own
// timestamp is the only record that the objective or a constraint changed.
func newestSoulEdit(workspacePath string) (authorityEdit, bool) {
	path := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath), SoulFolderName, SoulFileName)
	info, err := os.Stat(path)
	if err != nil {
		return authorityEdit{}, false
	}
	return authorityEdit{At: info.ModTime().UTC(), Description: "soul.md edit (objective, success criteria, or constraints)"}, true
}

// newestAuthorityEdit returns whichever owner-side change landed most recently.
func newestAuthorityEdit(workspacePath string) (authorityEdit, bool) {
	planEdit, hasPlan := newestPlanChangelogEdit(workspacePath)
	soulEdit, hasSoul := newestSoulEdit(workspacePath)
	switch {
	case hasPlan && hasSoul:
		if soulEdit.At.After(planEdit.At) {
			return soulEdit, true
		}
		return planEdit, true
	case hasPlan:
		return planEdit, true
	case hasSoul:
		return soulEdit, true
	}
	return authorityEdit{}, false
}

// staleStoreReport is what one store looked like against the newest edit.
type staleStoreReport struct {
	Store      string
	StaleItems []string
	Edit       authorityEdit
}

// markStaleItems stamps items confirmed before the edit and returns their names.
// Items already carrying the same StaleSince are still counted — they remain
// stale — but re-stamping them is a no-op, so the ledger only changes when the
// set actually changes.
func markStaleItems(ledger *FreshnessLedger, edit authorityEdit) ([]string, bool) {
	var stale []string
	changed := false
	stamp := edit.At.Format(time.RFC3339)
	for name, item := range ledger.Items {
		confirmedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.LastConfirmedAt))
		// An item with no confirmation date has never been reviewed by a run at
		// all. That is a gap for learning_health to chase, not evidence that this
		// particular edit invalidated it — claiming so would be a fabricated link.
		if err != nil {
			continue
		}
		if !confirmedAt.Before(edit.At) {
			// Confirmed after the edit: a run has already seen it in the new world.
			// Clear any earlier stale mark rather than leaving a permanent scar.
			if item.StaleSince != "" {
				item.StaleSince = ""
				item.StaleReason = ""
				ledger.Items[name] = item
				changed = true
			}
			continue
		}
		stale = append(stale, name)
		if item.StaleSince != stamp || item.StaleReason != edit.Description {
			item.StaleSince = stamp
			item.StaleReason = edit.Description
			ledger.Items[name] = item
			changed = true
		}
	}
	sort.Strings(stale)
	return stale, changed
}

// markStoreStaleness evaluates one store's ledger against the newest owner edit.
func (hcpo *StepBasedWorkflowOrchestrator) markStoreStaleness(ctx context.Context, store, ledgerPath string, edit authorityEdit) (staleStoreReport, error) {
	report := staleStoreReport{Store: store, Edit: edit}

	freshnessLedgerMu.Lock()
	defer freshnessLedgerMu.Unlock()

	existing, err := hcpo.BaseOrchestrator.ReadWorkspaceFile(ctx, ledgerPath)
	if err != nil || strings.TrimSpace(existing) == "" {
		// No ledger means the store has never been confirmed by a run; there is
		// nothing to call stale.
		return report, nil
	}
	ledger := parseFreshnessLedger(existing, store)
	if len(ledger.Items) == 0 {
		return report, nil
	}

	stale, changed := markStaleItems(&ledger, edit)
	report.StaleItems = stale
	if !changed {
		return report, nil
	}
	updated, marshalErr := marshalFreshnessLedger(ledger)
	if marshalErr != nil {
		return report, marshalErr
	}
	if writeErr := hcpo.BaseOrchestrator.WriteWorkspaceFile(ctx, ledgerPath, updated); writeErr != nil {
		return report, fmt.Errorf("write freshness ledger %s: %w", ledgerPath, writeErr)
	}
	return report, nil
}

// MarkStaleStoresAfterRun compares both knowledge stores against the newest
// owner-side edit, stamps the items that predate it, and files one concern per
// affected store.
//
// One concern per store, not per item: twelve items invalidated by a single plan
// edit are one problem with one fix, and twelve rows would drown the concern list
// that Pulse ranks by recurrence.
//
// Best-effort — a completed run is never failed by this bookkeeping.
func (hcpo *StepBasedWorkflowOrchestrator) MarkStaleStoresAfterRun(ctx context.Context, runFolder string) {
	workspacePath := strings.Trim(strings.TrimSpace(hcpo.GetWorkspacePath()), "/")
	if workspacePath == "" {
		return
	}
	edit, ok := newestAuthorityEdit(workspacePath)
	if !ok {
		return
	}

	for _, target := range []struct{ store, ledgerPath string }{
		{"learnings", learningsFreshnessLedgerPath()},
		{KnowledgebaseFolderName, knowledgebaseFreshnessLedgerPath()},
	} {
		report, err := hcpo.markStoreStaleness(ctx, target.store, target.ledgerPath, edit)
		if err != nil {
			hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to evaluate %s staleness: %v", target.store, err))
			continue
		}
		if len(report.StaleItems) == 0 {
			continue
		}
		hcpo.GetLogger().Info(fmt.Sprintf("🕰️ %d %s item(s) predate the newest owner edit (%s)", len(report.StaleItems), target.store, edit.Description))
		concern := fmt.Sprintf("CONCERNS: %d %s item(s) were last confirmed before %s — %s. Affected: %s. Review them against the current plan/soul and update or retract; a run has not seen them since the change.",
			len(report.StaleItems), target.store, edit.At.Format(time.RFC3339), edit.Description,
			strings.Join(truncateItemList(report.StaleItems, 8), ", "))
		if _, err := RecordRunConcerns(ctx, workspacePath, runFolder, hcpo.currentGroupName, "store-freshness:"+target.store, ConcernPhaseExecution, concern); err != nil {
			hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to record %s staleness concern: %v", target.store, err))
		}
	}
}

// truncateItemList keeps the concern text readable when a single edit invalidates
// a large store. The full set stays in the ledger's stale_since stamps.
func truncateItemList(items []string, max int) []string {
	if len(items) <= max {
		return items
	}
	out := append([]string{}, items[:max]...)
	return append(out, fmt.Sprintf("and %d more", len(items)-max))
}
