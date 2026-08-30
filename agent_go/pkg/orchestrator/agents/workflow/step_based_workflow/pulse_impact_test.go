package step_based_workflow

import (
	"context"
	"strings"
	"testing"
)

func pulseImpactFloat(value float64) *float64 { return &value }

func TestRecordPulseImpactUpdatePersistsComparableHistoryIdempotently(t *testing.T) {
	ctx := context.Background()
	workspace := concernsWorkspace(t)
	update := PulseImpactUpdate{
		Interventions: []PulseIntervention{{
			InterventionID:      "int-latency-gate",
			PulseRunID:          "pulse-1",
			Title:               "Use the proven latency measurement gate",
			CriterionID:         "voice-turn-latency",
			ImpactType:          "measurement",
			Metric:              "valid_latency_rate",
			ExpectedDirection:   "increase",
			Scope:               []string{"production", "dev"},
			Provenance:          "SELECT valid, total FROM latency_daily",
			BaselineWindow:      "runs 1-3",
			Checkpoint:          "after two producing runs",
			MinimumEvidenceRuns: 2,
			Sources: []PulseInterventionSource{{
				SourceType: "attempt",
				SourceID:   "attempt-1",
			}},
		}},
		Observations: []PulseGoalObservation{{
			CriterionID: "voice-turn-latency",
			Metric:      "valid_latency_rate",
			RunID:       "run-4",
			Route:       "production",
			Value:       pulseImpactFloat(0.97),
			Unit:        "ratio",
			ObservedAt:  "2026-08-03T10:00:00Z",
			Evidence:    []string{"latency_daily:run-4"},
		}},
		Assessments: []PulseImpactAssessment{{
			InterventionID: "int-latency-gate",
			Verdict:        "improved",
			BeforeWindow:   "runs 1-3",
			AfterWindow:    "runs 4-5",
			BeforeValue:    pulseImpactFloat(0.71),
			AfterValue:     pulseImpactFloat(0.97),
			AbsoluteChange: pulseImpactFloat(0.26),
			Confidence:     "high",
			Evidence:       []string{"latency_daily:runs-1-5"},
			AssessedAt:     "2026-08-03T11:00:00Z",
		}},
	}

	for attempt := 0; attempt < 2; attempt++ {
		ledger, err := RecordPulseImpactUpdate(ctx, workspace, update)
		if err != nil {
			t.Fatalf("record attempt %d: %v", attempt, err)
		}
		if len(ledger.Interventions) != 1 || len(ledger.Observations) != 1 || len(ledger.Assessments) != 1 {
			t.Fatalf("idempotent ledger counts = %d/%d/%d", len(ledger.Interventions), len(ledger.Observations), len(ledger.Assessments))
		}
		if got := ledger.Interventions[0].Sources; len(got) != 1 || got[0].SourceID != "attempt-1" {
			t.Fatalf("intervention source lost: %#v", got)
		}
		if got := ledger.Interventions[0].Status; got != "assessed" {
			t.Fatalf("matured intervention status = %q, want assessed", got)
		}
		if got := ledger.Observations[0].Value; got == nil || *got != 0.97 {
			t.Fatalf("observation value = %#v", got)
		}
		if got := ledger.Assessments[0].Verdict; got != "improved" {
			t.Fatalf("assessment verdict = %q", got)
		}
	}
}

func TestRecordPulseImpactUpdateRejectsUnsupportedClaimsAtomically(t *testing.T) {
	ctx := context.Background()
	workspace := concernsWorkspace(t)
	_, err := RecordPulseImpactUpdate(ctx, workspace, PulseImpactUpdate{
		Interventions: []PulseIntervention{{
			InterventionID:    "int-invalid",
			Title:             "Claim success without a supported impact class",
			CriterionID:       "goal",
			ImpactType:        "guaranteed_goal_win",
			Metric:            "wins",
			ExpectedDirection: "increase",
		}},
	})
	if err == nil {
		t.Fatal("invalid impact type was accepted")
	}
	ledger, loadErr := LoadPulseImpactLedger(ctx, workspace, 10)
	if loadErr != nil {
		t.Fatalf("load after rejected transaction: %v", loadErr)
	}
	if len(ledger.Interventions) != 0 {
		t.Fatalf("rejected transaction persisted data: %#v", ledger.Interventions)
	}
}

func TestStrategyExperimentsUseInterferenceDomainsInsteadOfGlobalLimit(t *testing.T) {
	ctx := context.Background()
	workspace := concernsWorkspace(t)
	experiment := func(id, status string, domains ...string) PulseIntervention {
		return PulseIntervention{
			InterventionID: id, Title: id, CriterionID: "growth", ImpactType: "direct_goal",
			Metric: "qualified_replies", ExpectedDirection: "increase", Kind: "strategy_experiment",
			Status: status, BaselineWindow: "prior 3 runs", Checkpoint: "after 2 producing runs",
			Guardrails: []string{"no unsolicited messages"}, RollbackCondition: "reply quality regresses",
			InterferenceDomains: domains,
		}
	}
	ledger, err := RecordPulseImpactUpdate(ctx, workspace, PulseImpactUpdate{Interventions: []PulseIntervention{
		experiment("video-format", "running", "control:post-format", "metric:video-replies"),
		experiment("reply-copy", "running", "control:reply-copy", "metric:qualified-replies"),
		experiment("future-channel", "approved"), // not started; no active slot or domain required
	}})
	if err != nil {
		t.Fatalf("non-conflicting experiments rejected: %v", err)
	}
	if len(ledger.Interventions) != 3 {
		t.Fatalf("interventions = %d, want 3", len(ledger.Interventions))
	}
	_, err = RecordPulseImpactUpdate(ctx, workspace, PulseImpactUpdate{Interventions: []PulseIntervention{
		experiment("competing-copy", "measuring", "control:reply-copy", "metric:conversion"),
	}})
	if err == nil || !strings.Contains(err.Error(), "control:reply-copy") {
		t.Fatalf("overlapping experiment error = %v, want named interference domain", err)
	}
}
