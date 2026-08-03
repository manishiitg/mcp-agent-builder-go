package step_based_workflow

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
)

func TestPulseReviewResultReferenceRequiresDatedRunAndModule(t *testing.T) {
	path, err := pulseReviewResultPath("2026-07-21T00-08-44.123Z_pulse-run-1", "bug_review")
	if err != nil {
		t.Fatalf("pulseReviewResultPath: %v", err)
	}
	if want := "pulse_review_log:2026-07-21T00-08-44.123Z_pulse-run-1:bug_review"; path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	for _, tc := range []struct {
		runID  string
		module string
	}{
		{"pulse-run-1", "bug_review"},
		{"2026-07-21T00-08-44.123Z_../escape", "bug_review"},
		{"2026-07-21T00-08-44.123Z_pulse-run-1", "unknown"},
	} {
		if _, err := pulseReviewResultPath(tc.runID, tc.module); err == nil {
			t.Fatalf("pulseReviewResultPath(%q, %q) unexpectedly succeeded", tc.runID, tc.module)
		}
	}
}

func TestNewManualPulseReviewIdentityProducesValidUniqueReviewIDs(t *testing.T) {
	firstTime := time.Date(2026, 7, 31, 9, 8, 7, 123456789, time.FixedZone("IST", 5*60*60+30*60))
	firstPulseID, firstReviewID := newManualPulseReviewIdentity(firstTime, "standalone bug review", "bug_review")
	if firstPulseID == "" {
		t.Fatal("manual pulse id is empty")
	}
	if _, err := pulseReviewResultPath(firstReviewID, "bug_review"); err != nil {
		t.Fatalf("manual review identity is invalid: %v", err)
	}
	if !strings.Contains(firstReviewID, "_manual-bug-review-standalone-bug-review-") {
		t.Fatalf("manual review id %q does not preserve its module/task identity", firstReviewID)
	}

	_, secondReviewID := newManualPulseReviewIdentity(firstTime.Add(time.Nanosecond), "standalone bug review", "bug_review")
	if firstReviewID == secondReviewID {
		t.Fatalf("manual review ids must be unique: %q", firstReviewID)
	}
}

func TestNewDerivedPulseReviewIdentityPreservesScheduledPulseID(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 987654321, time.UTC)
	pulseID, reviewID := newDerivedPulseReviewIdentity(
		now,
		"pulse-2026-07-31-001",
		"goal-advisor-pipeline",
		"goal_advisor",
	)
	if pulseID != "pulse-2026-07-31-001" {
		t.Fatalf("pulse id = %q", pulseID)
	}
	if _, err := pulseReviewResultPath(reviewID, "goal_advisor"); err != nil {
		t.Fatalf("scheduled derived review identity is invalid: %v", err)
	}
}

func TestPulseReviewResultMarkdownCarriesIdentityAndFindings(t *testing.T) {
	completedAt := time.Date(2026, 7, 21, 0, 8, 44, 123000000, time.UTC)
	body := pulseReviewResultMarkdown("pulse-run-1", "2026-07-21T00-08-44.123Z_pulse-run-1", "eval_health", "completed", "Verdict: clean", completedAt)
	for _, want := range []string{
		"Pulse run: `pulse-run-1`",
		"Review run: `2026-07-21T00-08-44.123Z_pulse-run-1`",
		"Module: `eval_health`",
		"Status: `completed`",
		"Verdict: clean",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("result markdown missing %q:\n%s", want, body)
		}
	}
}

func TestPulseReviewerSlotsEnforceMaximumTwo(t *testing.T) {
	slots := make(chan struct{}, pulsemodules.ReviewerMaxConcurrency)
	release := make(chan struct{})
	var active atomic.Int32
	var peak atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := acquirePulseReviewerSlot(context.Background(), slots); err != nil {
				t.Errorf("acquirePulseReviewerSlot: %v", err)
				return
			}
			current := active.Add(1)
			for {
				seen := peak.Load()
				if current <= seen || peak.CompareAndSwap(seen, current) {
					break
				}
			}
			<-release
			active.Add(-1)
			<-slots
		}()
	}

	deadline := time.Now().Add(time.Second)
	for peak.Load() < pulsemodules.ReviewerMaxConcurrency && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := peak.Load(); got != pulsemodules.ReviewerMaxConcurrency {
		t.Fatalf("peak concurrency = %d, want %d", got, pulsemodules.ReviewerMaxConcurrency)
	}
	close(release)
	wg.Wait()
	if got := peak.Load(); got != pulsemodules.ReviewerMaxConcurrency {
		t.Fatalf("final peak concurrency = %d, want %d", got, pulsemodules.ReviewerMaxConcurrency)
	}
}

func TestPulseReviewerPersistenceContextSurvivesCallerCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	execCtx := context.WithValue(context.Background(), struct{ name string }{"review"}, "alive")

	persistCtx, cancelPersist := pulseReviewerPersistenceContext(execCtx)
	defer cancelPersist()

	if requestCtx.Err() == nil {
		t.Fatal("request context should be canceled")
	}
	if err := persistCtx.Err(); err != nil {
		t.Fatalf("persistence context inherited unrelated request cancellation: %v", err)
	}
	if got := persistCtx.Value(struct{ name string }{"review"}); got != "alive" {
		t.Fatalf("persistence context lost execution identity: %v", got)
	}
}
