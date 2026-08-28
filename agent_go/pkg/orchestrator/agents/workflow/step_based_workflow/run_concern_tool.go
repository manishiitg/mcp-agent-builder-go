package step_based_workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PLAT-055. A step used to have exactly one way to report anything that was not
// a learning: a `CONCERNS:` line in its completion summary, scraped by
// ParseConcernLines. That guarantees capture of *lines*, not concerns — the line
// is a single string with no severity, classification, or evidence, so a real
// finding cannot survive it intact.
//
// The measured cost: 39 of 103 active Upwork concerns carry no detail row at
// all. And the findings that needed the bandwidth most simply went elsewhere —
// one Social Media learnings file holds a multi-paragraph validator-bug analysis
// in which the agent explicitly writes that it should "surface it as a
// harness/validator bug report", then files it into learnings because that was
// the only destination with room for it.
//
// This gives steps the same structured shape Pulse reviewers already use
// (PulseFindingDetails), writing through the same lifecycle rows so dedup by
// fingerprint and the open → acknowledged → resolved progression are unchanged.
// The difference from RecordPulseReviewFinding is ownership: a step files
// against its own step ID and execution phase, and never manages the backlog —
// no issue_id updates, no merges. Steps report; Pulse curates.

// StepRunConcernInput is one structured concern raised by a step.
type StepRunConcernInput struct {
	// Concern is the one-line statement that identifies this issue. It is what
	// dedup fingerprints against, so it should describe the defect rather than
	// this particular occurrence of it.
	Concern string `json:"concern"`
	PulseFindingDetails
}

// StepRunConcernRecord is what the caller gets back.
type StepRunConcernRecord struct {
	IssueID     string `json:"issue_id"`
	Fingerprint string `json:"-"`
	Phase       string `json:"phase"`
	StepID      string `json:"step_id"`
	Recorded    bool   `json:"recorded"`
	// Duplicate reports that this run already filed the same concern; the row
	// was left alone rather than inflating its recurrence count.
	Duplicate bool `json:"duplicate,omitempty"`
}

// validateStepRunConcern mirrors validateTypedPulseReviewFinding, minus the
// advisor-route rules that only apply to Pulse modules.
func validateStepRunConcern(stepID, phase string, input StepRunConcernInput) (pulseFindingDetailMarker, error) {
	marker := pulseFindingDetailMarker{
		Concern:             strings.TrimSpace(input.Concern),
		Module:              strings.TrimSpace(stepID),
		PulseFindingDetails: normalizePulseFindingDetails(input.PulseFindingDetails),
	}
	if marker.Concern == "" {
		return marker, fmt.Errorf("concern is required: state the defect in one line")
	}
	if marker.Module == "" {
		return marker, fmt.Errorf("step_id is required")
	}
	switch phase {
	case ConcernPhaseExecution, ConcernPhaseLearnings, ConcernPhaseKBReview, ConcernPhaseMessageSequence:
	default:
		return marker, fmt.Errorf("phase %q is not a step phase; steps may file execution, learnings, kb-review, or message-sequence concerns", phase)
	}
	if marker.IssueKind != IssueKindWorkflow && marker.IssueKind != IssueKindHarness {
		return marker, fmt.Errorf("issue_kind must be %s or %s", IssueKindWorkflow, IssueKindHarness)
	}
	if marker.Classification == "" || marker.Severity == "" || marker.Summary == "" || marker.Impact == "" || len(marker.Evidence) == 0 {
		return marker, fmt.Errorf("classification, severity, summary, impact, and evidence are required")
	}
	// A harness issue leaves the workflow entirely, so it has to carry enough
	// for someone outside it to act without re-deriving the observation.
	if marker.IssueKind == IssueKindHarness && (marker.TargetKey == "" || marker.Reproduction.Expected == "" || marker.Reproduction.Observed == "") {
		return marker, fmt.Errorf("%s requires target_key and reproduction.expected/reproduction.observed so it can be acted on outside this workflow", IssueKindHarness)
	}
	return marker, nil
}

// RecordStepRunConcern files one structured concern from a step.
//
// Best-effort by contract, like RecordRunConcerns: a step that did its work must
// not fail because its concern could not be filed. Callers log and continue.
func RecordStepRunConcern(
	ctx context.Context,
	workspacePath, runFolder, groupName, stepID, phase string,
	input StepRunConcernInput,
) (StepRunConcernRecord, error) {
	marker, err := validateStepRunConcern(stepID, phase, input)
	if err != nil {
		return StepRunConcernRecord{}, err
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return StepRunConcernRecord{}, err
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return StepRunConcernRecord{}, err
	}

	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	fingerprint := concernFingerprint(marker.Module, marker.Concern)
	if canonical := canonicalFingerprintForMergedIssue(ctx, db, fingerprint); canonical != "" {
		fingerprint = canonical
	}
	normalizedConcern := strings.ToLower(strings.Join(strings.Fields(marker.Concern), " "))
	fingerprints := map[string]string{normalizedConcern: fingerprint}

	record := StepRunConcernRecord{
		IssueID:     pulseIssueID(PulseFindingLifecycle{Fingerprint: fingerprint}),
		Fingerprint: fingerprint, Phase: phase, StepID: marker.Module,
	}

	// A retried turn can replay its tool calls. Filing the same concern twice in
	// one run must not manufacture recurrence evidence — seen_count is what Gate
	// weighs when deciding a root cause is worth repairing.
	var lastSeenRun string
	err = db.QueryRowContext(ctx, `SELECT last_seen_run FROM run_concerns WHERE fingerprint=?`, fingerprint).Scan(&lastSeenRun)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return record, err
	}
	if strings.TrimSpace(lastSeenRun) == strings.TrimSpace(runFolder) && strings.TrimSpace(runFolder) != "" {
		record.Duplicate = true
	} else {
		if _, err := recordRunConcernLinesAtWithFingerprints(
			ctx, db, runFolder, groupName, marker.Module, phase,
			[]string{marker.Concern}, observedAt, fingerprints,
		); err != nil {
			return record, err
		}
		record.Recorded = true
	}

	// Detail is refreshed even on a duplicate: the second observation may carry
	// better evidence than the first, and the detail row is keyed by fingerprint
	// rather than appended.
	if err := recordPulseFindingDetailAt(ctx, db, workspacePath, runFolder, marker.Module, marker, fingerprint, observedAt); err != nil {
		return record, err
	}
	return record, nil
}
