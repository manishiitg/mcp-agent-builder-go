package costledger

import (
	"path/filepath"
	"testing"
	"time"
)

func newEmptySummary() *Summary {
	return &Summary{
		ByDate:  make(map[string]*DateAggregate),
		ByModel: make(map[string]*Aggregate),
		ByScope: make(map[string]*ScopeAggregate),
	}
}

// TestAddEntryToSummaryBuildsPhaseBreakdownWithinOneExecution pins PLAT-166:
// two entries sharing one ExecutionID but different Phase values land in the
// same ByExecution bucket (combined total unchanged — a workflow step's
// execution and reflection turns are still billed as one execution) AND
// populate a ByPhase breakdown that sums back to that same total.
func TestAddEntryToSummaryBuildsPhaseBreakdownWithinOneExecution(t *testing.T) {
	summary := newEmptySummary()
	base := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	addEntryToSummary(summary, "2026-08-21", Entry{
		Timestamp:    base,
		Scope:        "workflow_execution",
		ExecutionID:  "session-1:review-measure",
		Phase:        "execution_only",
		LLMCallCount: 1,
		TotalCostUSD: 1.00,
		PromptTokens: 1000,
	})
	addEntryToSummary(summary, "2026-08-21", Entry{
		Timestamp:    base.Add(time.Minute),
		Scope:        "workflow_execution",
		ExecutionID:  "session-1:review-measure",
		Phase:        "reflection",
		LLMCallCount: 1,
		TotalCostUSD: 0.25,
		PromptTokens: 200,
	})

	execution := summary.ByScope["workflow_execution"].ByExecution["session-1:review-measure"]
	if execution == nil {
		t.Fatal("execution bucket missing")
	}
	if got, want := execution.TotalCostUSD, 1.25; got != want {
		t.Fatalf("combined total_cost_usd = %v, want %v", got, want)
	}
	if execution.ByPhase == nil {
		t.Fatal("by_phase missing")
	}
	if got, want := execution.ByPhase["execution_only"].TotalCostUSD, 1.00; got != want {
		t.Fatalf("execution_only cost = %v, want %v", got, want)
	}
	if got, want := execution.ByPhase["reflection"].TotalCostUSD, 0.25; got != want {
		t.Fatalf("reflection cost = %v, want %v", got, want)
	}
	summed := execution.ByPhase["execution_only"].TotalCostUSD + execution.ByPhase["reflection"].TotalCostUSD
	if summed != execution.TotalCostUSD {
		t.Fatalf("by_phase sum = %v, want combined total %v", summed, execution.TotalCostUSD)
	}

	// The same must hold one level down, in the per-date breakdown — the two
	// construction sites in addEntryToSummary must stay in sync.
	dateExecution := summary.ByDate["2026-08-21"].ByScope["workflow_execution"].ByExecution["session-1:review-measure"]
	if dateExecution == nil || dateExecution.ByPhase == nil {
		t.Fatal("date-scoped by_phase missing")
	}
	if dateExecution.ByPhase["execution_only"].TotalCostUSD != 1.00 || dateExecution.ByPhase["reflection"].TotalCostUSD != 0.25 {
		t.Fatalf("date-scoped by_phase = %+v", dateExecution.ByPhase)
	}
}

// TestAddEntryToSummaryOmitsPhaseBreakdownWhenEntryHasNone proves the
// backward-compatible case: an entry with no Phase (every non-workflow scope,
// and every entry written before PLAT-166) never populates by_phase — the
// aggregation change is inert for existing data and existing consumers.
func TestAddEntryToSummaryOmitsPhaseBreakdownWhenEntryHasNone(t *testing.T) {
	summary := newEmptySummary()
	addEntryToSummary(summary, "2026-08-21", Entry{
		Timestamp:    time.Now(),
		Scope:        "chat",
		ExecutionID:  "session-1",
		LLMCallCount: 1,
		TotalCostUSD: 0.5,
	})
	execution := summary.ByScope["chat"].ByExecution["session-1"]
	if execution == nil {
		t.Fatal("execution bucket missing")
	}
	if execution.ByPhase != nil {
		t.Fatalf("by_phase = %+v, want nil for an entry with no phase", execution.ByPhase)
	}
	if execution.TotalCostUSD != 0.5 {
		t.Fatalf("total_cost_usd = %v, want 0.5 (unaffected by the phase change)", execution.TotalCostUSD)
	}
}

// TestSQLitePhasePersistsThroughAppendAndSummarize is the full round trip:
// the phase column survives NewSQLiteLedger's migration, append's INSERT, and
// summarizeWindow's SELECT, ending up in the same by_phase shape the two
// in-memory tests above pin.
func TestSQLitePhasePersistsThroughAppendAndSummarize(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "costs.sqlite")
	ledger, err := NewSQLiteLedger(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteLedger error = %v", err)
	}
	defer ledger.Close()

	base := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	if err := ledger.Append(Entry{
		EventID: "ev-1", IdempotencyKey: "ev-1", Timestamp: base,
		WorkflowID: "Workflow/test", Scope: "workflow_execution",
		ExecutionID: "session-1:review-measure", Phase: "execution_only",
		LLMCallCount: 1, TotalCostUSD: 1.00,
	}); err != nil {
		t.Fatalf("append execution entry: %v", err)
	}
	if err := ledger.Append(Entry{
		EventID: "ev-2", IdempotencyKey: "ev-2", Timestamp: base.Add(time.Minute),
		WorkflowID: "Workflow/test", Scope: "workflow_execution",
		ExecutionID: "session-1:review-measure", Phase: "reflection",
		LLMCallCount: 1, TotalCostUSD: 0.25,
	}); err != nil {
		t.Fatalf("append reflection entry: %v", err)
	}

	summary, err := ledger.SummarizeWorkflow("Workflow/test")
	if err != nil {
		t.Fatalf("SummarizeWorkflow error = %v", err)
	}
	execution := summary.ByScope["workflow_execution"].ByExecution["session-1:review-measure"]
	if execution == nil {
		t.Fatal("execution bucket missing after SQLite round trip")
	}
	if got, want := execution.TotalCostUSD, 1.25; got != want {
		t.Fatalf("combined total = %v, want %v", got, want)
	}
	if execution.ByPhase["execution_only"].TotalCostUSD != 1.00 {
		t.Fatalf("execution_only cost after round trip = %+v", execution.ByPhase["execution_only"])
	}
	if execution.ByPhase["reflection"].TotalCostUSD != 0.25 {
		t.Fatalf("reflection cost after round trip = %+v", execution.ByPhase["reflection"])
	}
}

// TestSQLiteLedgerOpensExistingDatabaseMissingPhaseColumn proves the
// migration path: a database created before PLAT-166 (no phase column) opens
// cleanly and gets the column added, rather than failing to open or silently
// losing existing rows.
func TestSQLiteLedgerOpensExistingDatabaseMissingPhaseColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "costs.sqlite")

	first, err := NewSQLiteLedger(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteLedger(first) error = %v", err)
	}
	if err := first.Append(Entry{
		EventID: "pre-migration", IdempotencyKey: "pre-migration",
		Timestamp: time.Now(), WorkflowID: "Workflow/test",
		Scope: "workflow_execution", ExecutionID: "session-1:step",
		LLMCallCount: 1, TotalCostUSD: 1.0,
	}); err != nil {
		t.Fatalf("append before reopen: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first ledger: %v", err)
	}

	second, err := NewSQLiteLedger(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteLedger(second, reopen) error = %v", err)
	}
	defer second.Close()

	summary, err := second.SummarizeWorkflow("Workflow/test")
	if err != nil {
		t.Fatalf("SummarizeWorkflow after reopen: %v", err)
	}
	execution := summary.ByScope["workflow_execution"].ByExecution["session-1:step"]
	if execution == nil || execution.TotalCostUSD != 1.0 {
		t.Fatalf("pre-migration row lost or corrupted after reopen: %+v", execution)
	}
}
