package step_based_workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
)

// Evidence that knowledge may have been overtaken by a newer decision.
//
// The failure this addresses: soul.md was edited to retire a rule, a learnings
// reference asserting that rule had last been confirmed three days earlier, and
// it kept being served for a week. learning_health's first documented trigger is
// already "plan or prompt changes affected step behavior" — the module was
// correctly scoped and simply had no fact to fire on.
//
// So this computes the fact and nothing else: "learnings were last confirmed at
// T; here are the plan and soul edits since T." It deliberately does NOT decide
// which learnings are stale. Only reading the diffs against the actual files can
// tell a cosmetic description edit from one that invalidates an auth flow, and
// that judgement belongs to the improve-learnings reviewer, which already reads
// those files and already returns findings for the Pulse Fixer to apply.
//
// An earlier version marked every item older than the newest edit. That was Go
// making a judgement it cannot make: routine bookkeeping (a review_notes touch)
// looked identical to a rewritten step description, so it would have flagged
// almost everything almost always, and taught everyone to ignore the flag.
//
// Computed on read, not on a schedule: it is a few small file reads, it needs no
// state of its own, and it stays true until someone actually reviews — so the
// signal re-appears on its own rather than depending on anything being recorded.

// StoreEditEvidence is one knowledge store measured against later owner edits.
type StoreEditEvidence struct {
	Store            string   `json:"store"`
	LastConfirmedAt  string   `json:"last_confirmed_at,omitempty"`
	LastConfirmedRun string   `json:"last_confirmed_run,omitempty"`
	ItemCount        int      `json:"item_count"`
	EditsSince       []string `json:"edits_since,omitempty"`
	Note             string   `json:"note,omitempty"`
}

// maxListedEdits caps the evidence handed to the Gate. The full record is in
// planning/changelog/; this is a prompt for a decision, not an archive.
const maxListedEdits = 10

func workflowAbsPath(workspacePath string, parts ...string) string {
	all := append([]string{fsutil.WorkspaceDocsRoot(), filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/"))}, parts...)
	return filepath.Join(all...)
}

// planEditsSince returns human-readable descriptions of plan-mod calls made after
// the given time, newest first.
//
// Each entry carries the mandatory `reason` the editor supplied, plus any
// recorded field-level old/new values. That detail is the point: the reviewer
// needs to see WHAT changed to judge whether any learning is affected, and a
// tool name alone cannot support that judgement.
func planEditsSince(workspacePath string, since time.Time) []string {
	dir := workflowAbsPath(workspacePath, PlanningFolderName, "changelog")
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type dated struct {
		at   time.Time
		desc string
	}
	var found []dated
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var file PlanChangelog
		if json.Unmarshal(raw, &file) != nil {
			continue
		}
		for _, e := range file.Entries {
			ts, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Timestamp))
			if err != nil || !ts.After(since) {
				continue
			}
			found = append(found, dated{at: ts.UTC(), desc: describePlanEdit(e, ts.UTC())})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at.After(found[j].at) })
	out := make([]string, 0, len(found))
	for _, d := range found {
		out = append(out, d.desc)
	}
	return out
}

func describePlanEdit(e PlanChangelogEntry, at time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s", at.Format(time.RFC3339), strings.TrimSpace(e.Tool))
	if len(e.StepIDs) > 0 {
		fmt.Fprintf(&b, " on %s", strings.Join(e.StepIDs, ", "))
	}
	if reason := strings.TrimSpace(e.Reason); reason != "" {
		fmt.Fprintf(&b, ": %s", reason)
	}
	var fields []string
	for _, c := range e.Changes {
		if f := strings.TrimSpace(c.Field); f != "" {
			fields = append(fields, f)
		}
	}
	if len(fields) > 0 {
		fmt.Fprintf(&b, " [fields changed: %s]", strings.Join(fields, ", "))
	}
	return b.String()
}

// soulEditSince reports a soul.md change after the given time.
//
// soul.md is shell-written and has no changelog entry, so its mtime is the only
// record. mtime is blunt — it moves on any save, including a typo fix — which is
// exactly why this is reported as evidence for a reviewer to weigh rather than
// treated as proof that anything is stale.
func soulEditSince(workspacePath string, since time.Time) (string, bool) {
	info, err := os.Stat(workflowAbsPath(workspacePath, SoulFolderName, SoulFileName))
	if err != nil {
		return "", false
	}
	at := info.ModTime().UTC()
	if !at.After(since) {
		return "", false
	}
	return fmt.Sprintf("%s — soul.md modified (objective, success criteria, or constraints may have changed; mtime moves on any save, so confirm what actually changed)", at.Format(time.RFC3339)), true
}

func readFreshnessLedgerFile(workspacePath, relPath string) (FreshnessLedger, bool) {
	raw, err := os.ReadFile(workflowAbsPath(workspacePath, filepath.FromSlash(relPath)))
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return FreshnessLedger{}, false
	}
	return parseFreshnessLedger(string(raw), ""), true
}

// CollectStoreEditEvidence measures both knowledge stores against later owner
// edits. Returns only stores that have content AND unreviewed edits — a store
// nobody has changed since it was last confirmed contributes nothing.
func CollectStoreEditEvidence(workspacePath string) []StoreEditEvidence {
	workspacePath = strings.Trim(strings.TrimSpace(workspacePath), "/")
	if workspacePath == "" {
		return nil
	}
	var out []StoreEditEvidence
	for _, target := range []struct{ store, ledgerPath string }{
		{"learnings", learningsFreshnessLedgerPath()},
		{KnowledgebaseFolderName, knowledgebaseFreshnessLedgerPath()},
	} {
		ledger, ok := readFreshnessLedgerFile(workspacePath, target.ledgerPath)
		if !ok || len(ledger.Items) == 0 {
			continue
		}
		confirmedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(ledger.LastConfirmedAt))
		if err != nil {
			// Content with no confirmation baseline at all. Say so rather than
			// silently skipping: "never confirmed" is itself worth reviewing.
			out = append(out, StoreEditEvidence{
				Store:     target.store,
				ItemCount: len(ledger.Items),
				Note:      "store has content but no confirmation baseline yet — no run has recorded reviewing it",
			})
			continue
		}
		edits := planEditsSince(workspacePath, confirmedAt)
		if soulEdit, has := soulEditSince(workspacePath, confirmedAt); has {
			edits = append([]string{soulEdit}, edits...)
		}
		if len(edits) == 0 {
			continue
		}
		total := len(edits)
		if len(edits) > maxListedEdits {
			edits = append(edits[:maxListedEdits], fmt.Sprintf("…and %d earlier edit(s) — see planning/changelog/", total-maxListedEdits))
		}
		out = append(out, StoreEditEvidence{
			Store:            target.store,
			LastConfirmedAt:  ledger.LastConfirmedAt,
			LastConfirmedRun: ledger.LastConfirmedRun,
			ItemCount:        len(ledger.Items),
			EditsSince:       edits,
			Note:             fmt.Sprintf("%d owner-side edit(s) landed after this store was last confirmed. This is evidence, NOT a verdict: most edits invalidate nothing. Read the diffs against the actual files before concluding anything is stale.", total),
		})
	}
	return out
}
