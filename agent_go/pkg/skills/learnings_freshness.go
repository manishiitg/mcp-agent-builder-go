package skills

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// Surfaces contradicted learnings at the moment an agent decides what to read.
//
// The pointer skill hands agents a directory and lets them choose. Every file in
// it looks equally authoritative, so an entry that a newer plan or soul.md edit
// has retired reads exactly like a current one — which is how a rule that had
// been superseded stayed in active use for a week in a live workflow, while the
// evidence that it was stale sat in _freshness.json the whole time.
//
// The backend stamps stale_since / stale_reason at run end (store_staleness.go in
// the orchestrator). This is the read side: put the warning in front of the agent
// as it picks a file, rather than leaving it in a ledger nobody opens.

// freshnessLedgerView mirrors the on-disk shape of _freshness.json.
//
// Deliberately a local copy rather than an import: the orchestrator package that
// owns the writer already imports this package, so importing it back would be a
// cycle. The JSON tags are the contract, and only the fields this read path needs
// are declared — same approach the server package takes for the plan changelog.
type freshnessLedgerView struct {
	Items map[string]freshnessItemView `json:"items,omitempty"`
}

type freshnessItemView struct {
	LastConfirmedAt string `json:"last_confirmed_at,omitempty"`
	StaleSince      string `json:"stale_since,omitempty"`
	StaleReason     string `json:"stale_reason,omitempty"`
}

// staleLearningEntry is one contradicted item, ready to render.
type staleLearningEntry struct {
	Name            string
	StaleSince      string
	StaleReason     string
	LastConfirmedAt string
}

// maxListedStaleLearnings caps the rendered list. This block goes into every
// session that attaches the skill, so an unbounded list would trade one context
// problem for another; the ledger keeps the complete set.
const maxListedStaleLearnings = 8

// collectStaleLearnings returns contradicted items, most recently invalidated
// first. Returns nil when the ledger is absent, unparseable, or clean — a
// workflow with healthy learnings must add nothing to the prompt.
func collectStaleLearnings(raw string) []staleLearningEntry {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var ledger freshnessLedgerView
	if err := json.Unmarshal([]byte(raw), &ledger); err != nil {
		// A malformed ledger is a reason to stay quiet, not to guess. Claiming
		// knowledge is stale on bad evidence would train agents to ignore the flag.
		return nil
	}
	var out []staleLearningEntry
	for name, item := range ledger.Items {
		if strings.TrimSpace(item.StaleSince) == "" {
			continue
		}
		out = append(out, staleLearningEntry{
			Name:            name,
			StaleSince:      strings.TrimSpace(item.StaleSince),
			StaleReason:     strings.TrimSpace(item.StaleReason),
			LastConfirmedAt: strings.TrimSpace(item.LastConfirmedAt),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StaleSince != out[j].StaleSince {
			return out[i].StaleSince > out[j].StaleSince
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// renderStaleLearningsWarning formats the block, or "" when there is nothing to warn about.
func renderStaleLearningsWarning(entries []staleLearningEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n**⚠ Some of this knowledge is contradicted by a newer decision.** ")
	b.WriteString("The files below were last confirmed BEFORE an owner-side plan or soul.md edit, so they may describe behavior that has since been retired. ")
	b.WriteString("They are still present and still look authoritative — treat them as suspect. Before relying on one, check it against the current step description and soul.md; prefer those when they disagree, and report the contradiction with a `CONCERNS:` line rather than silently picking a side.\n\n")

	shown := entries
	if len(shown) > maxListedStaleLearnings {
		shown = shown[:maxListedStaleLearnings]
	}
	for _, e := range shown {
		b.WriteString(fmt.Sprintf("- `%s` — stale since %s", e.Name, e.StaleSince))
		if e.StaleReason != "" {
			b.WriteString(fmt.Sprintf(" (%s)", e.StaleReason))
		}
		if e.LastConfirmedAt != "" {
			b.WriteString(fmt.Sprintf("; last confirmed %s", e.LastConfirmedAt))
		}
		b.WriteString("\n")
	}
	if len(entries) > len(shown) {
		b.WriteString(fmt.Sprintf("- …and %d more — see `learnings/_global/_freshness.json` for the full set.\n", len(entries)-len(shown)))
	}
	return b.String()
}

// buildStaleLearningsWarning reads the ledger and renders the warning.
//
// Best-effort by design: the pointer skill must still attach when the ledger is
// missing or unreadable. A workflow that has never had a stale item — the normal
// case — adds nothing.
func buildStaleLearningsWarning(client *WorkspaceAPIClient, workflowPath string) string {
	if client == nil {
		return ""
	}
	ledgerPath := path.Join(workflowPath, "learnings", "_global", "_freshness.json")
	raw, err := client.ReadFile(ledgerPath)
	if err != nil {
		return ""
	}
	return renderStaleLearningsWarning(collectStaleLearnings(raw))
}
