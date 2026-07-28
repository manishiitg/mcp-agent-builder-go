package step_based_workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSoulScaffoldContainsOnlyStableIntentSections(t *testing.T) {
	scaffold := SoulScaffold("Demo Workflow")

	for _, want := range []string{"# Demo Workflow", "## Objective", "## Success Criteria"} {
		if !strings.Contains(scaffold, want) {
			t.Fatalf("SoulScaffold missing %q:\n%s", want, scaffold)
		}
	}
	for _, forbidden := range []string{"## Why", "## Decisions & Constraints", "## Key References", "architecture", "decision log"} {
		if strings.Contains(scaffold, forbidden) {
			t.Fatalf("SoulScaffold should not invite revisable content %q:\n%s", forbidden, scaffold)
		}
	}
}

const soulWithConstraints = `# Trading Workflow

## Objective
Beat the benchmark.

## Success Criteria
Outperformance > 0.

## Constraints

- **Risk budget (owner-approved 2026-07-03):** max 1.5% capital risk per trade, up to 7 concurrent
  real positions, no single archetype > 60% of deployed capital.

## Notifications
Send to slack.
`

func readerFor(content string) func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return content, nil }
}

// The `## Constraints` section was a documented soul.md convention that nothing
// parsed, so owner-approved values were retyped into step descriptions and
// learnings and drifted from the owner's decision. It must extract cleanly and
// stop at the next H2 — pulling in `## Notifications` would inject delivery
// preferences into every agent prompt as if they were binding policy.
func TestReadWorkflowSoulSectionsExtractsConstraints(t *testing.T) {
	obj, sc, constraints, err := ReadWorkflowSoulSections(context.Background(), "Workflow/trading", readerFor(soulWithConstraints))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj != "Beat the benchmark." {
		t.Fatalf("objective = %q", obj)
	}
	if sc != "Outperformance > 0." {
		t.Fatalf("successCriteria = %q", sc)
	}
	if !strings.Contains(constraints, "up to 7 concurrent") || !strings.Contains(constraints, "1.5%") {
		t.Fatalf("constraints body not captured: %q", constraints)
	}
	if strings.Contains(constraints, "Notifications") || strings.Contains(constraints, "Send to slack") {
		t.Fatalf("constraints bled into the next H2 section: %q", constraints)
	}
}

// A workflow with no `## Constraints` section is the common case; it must yield
// an empty block so templates render nothing rather than an empty header.
func TestBuildWorkflowConstraintsBlockEmptyWhenAbsent(t *testing.T) {
	_, _, constraints, err := ReadWorkflowSoulSections(context.Background(), "Workflow/x", readerFor("# X\n\n## Objective\nDo a thing.\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := BuildWorkflowConstraintsBlock(constraints); got != "" {
		t.Fatalf("expected empty block when no Constraints section, got:\n%s", got)
	}
	if got := BuildWorkflowConstraintsBlock("   \n  "); got != "" {
		t.Fatalf("expected empty block for whitespace-only constraints, got:\n%s", got)
	}
}

// The rendered block carries the three rules that make a single source of truth
// actually work: the constraint wins over a stale copy, don't make new copies,
// and report rather than edit (steps have soul/ on read paths, never write).
func TestBuildWorkflowConstraintsBlockCarriesBindingRules(t *testing.T) {
	block := BuildWorkflowConstraintsBlock("max 1.5% risk per trade, up to 7 concurrent positions")
	for _, want := range []string{
		"BINDING CONSTRAINTS",
		"max 1.5% risk per trade",
		"the constraint above wins",
		"Do NOT restate a constraint value",
		"READ-ONLY",
		"CONCERNS:",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("constraints block missing %q:\n%s", want, block)
		}
	}
}

// A missing soul.md is a valid intermediate state (workflow not yet scaffolded)
// and must not surface as an error, or every step would log a spurious failure.
func TestReadWorkflowSoulSectionsTolerantOfMissingFile(t *testing.T) {
	_, _, constraints, err := ReadWorkflowSoulSections(context.Background(), "Workflow/x",
		func(context.Context, string) (string, error) { return "", errors.New("file not found") })
	if err != nil {
		t.Fatalf("missing soul.md should not error, got %v", err)
	}
	if constraints != "" {
		t.Fatalf("expected empty constraints, got %q", constraints)
	}
}
