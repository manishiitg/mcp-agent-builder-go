package step_based_workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/manishiitg/multi-llm-provider-go/llmerrors"
)

// WorkflowCapacityWaitFilename is the run-level record of a run suspended
// because the provider has no capacity left, written beside the run's other
// evidence.
const WorkflowCapacityWaitFilename = "capacity_wait.json"

// ErrWorkflowWaitingForCapacity marks a step failure that is not a failure.
//
// PLAT-101. When a provider's quota window is exhausted the step cannot
// proceed, but nothing is wrong with the run: the steps that already completed
// did real work, and the remaining ones will succeed once the window reopens.
// Reporting that as an error is what produced the observed damage — the run
// went red, the whole step loop abandoned the remaining steps, and the next
// cron tick started a fresh run from step 1 that re-ran the completed steps'
// side effects and hit the same wall, repeating every tick until the window
// happened to reopen.
var ErrWorkflowWaitingForCapacity = errors.New("workflow waiting for provider capacity")

// WorkflowCapacityWait is the durable record of where a run stopped and when
// it may continue.
//
// It is deliberately a run-level file rather than in-memory state. A run may
// wait for hours, and the server can restart during that time; without a
// durable record the restart sweep converts the pause into a false
// "interrupted: server restarted" error — exactly the misdiagnosis this work
// exists to remove.
type WorkflowCapacityWait struct {
	SchemaVersion int    `json:"schema_version"`
	WorkspacePath string `json:"workspace_path,omitempty"`
	RunFolder     string `json:"run_folder,omitempty"`

	// StepNumber is 1-based, matching ExecutionOptions.ResumeFromStep, so a
	// resume can be armed from this record without re-deriving the index.
	StepNumber int    `json:"step_number"`
	StepID     string `json:"step_id,omitempty"`
	StepPath   string `json:"step_path,omitempty"`
	StepTitle  string `json:"step_title,omitempty"`

	// TotalSteps lets a reader state "3 of 7 completed" without loading the plan.
	TotalSteps int `json:"total_steps,omitempty"`

	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Window names the exhausted quota window ("five_hour", "seven_day") when
	// the provider stated one.
	Window string `json:"window,omitempty"`

	// RetryAt is the instant the window reopens. Zero means the provider did
	// not state one — an explicit unknown, never a guess. A resume cannot be
	// armed from a fabricated time: it would wake straight back into the wall.
	RetryAt time.Time `json:"retry_at,omitempty"`

	RecordedAt time.Time `json:"recorded_at"`
	Reason     string    `json:"reason,omitempty"`
}

// WorkflowCapacityWaitPath locates the record for a run.
func WorkflowCapacityWaitPath(workspacePath, runFolder string) string {
	base := strings.TrimSpace(workspacePath)
	if runFolder = strings.TrimSpace(runFolder); runFolder != "" {
		base = fmt.Sprintf("%s/runs/%s", base, runFolder)
	}
	return fmt.Sprintf("%s/%s", base, WorkflowCapacityWaitFilename)
}

// ParseWorkflowCapacityWait decodes a record. Callers outside this package
// read the file through their own workspace accessor and parse with this, so
// the shape has exactly one definition.
func ParseWorkflowCapacityWait(content string) (*WorkflowCapacityWait, bool) {
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	var wait WorkflowCapacityWait
	if err := json.Unmarshal([]byte(content), &wait); err != nil {
		return nil, false
	}
	return &wait, true
}

// ResumeDue reports whether a recorded wait has reached its reset instant.
//
// A wait with no stated reset is never automatically due. Waking on a guess
// resumes into the same wall and burns the run's remaining steps a second
// time; such a run waits for a person to decide, which is the honest outcome
// when the provider declined to say when capacity returns.
func (w *WorkflowCapacityWait) ResumeDue(now time.Time) bool {
	if w == nil || w.RetryAt.IsZero() {
		return false
	}
	return !now.Before(w.RetryAt)
}

// Describe renders the wait for a run-history row.
func (w *WorkflowCapacityWait) Describe() string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("waiting for provider capacity")
	if w.Window != "" {
		b.WriteString(" (" + w.Window + " window)")
	}
	if w.StepNumber > 0 {
		if w.TotalSteps > 0 {
			fmt.Fprintf(&b, "; paused at step %d of %d", w.StepNumber, w.TotalSteps)
		} else {
			fmt.Fprintf(&b, "; paused at step %d", w.StepNumber)
		}
	}
	if w.RetryAt.IsZero() {
		b.WriteString("; reset time unknown, resume must be triggered manually")
	} else {
		b.WriteString("; resumes " + w.RetryAt.UTC().Format(time.RFC3339))
	}
	return b.String()
}

// newWorkflowCapacityWait builds the record from the typed provider failure.
// Returns nil when err is not quota exhaustion, so callers can use it as the
// classification step as well.
func newWorkflowCapacityWait(err error, workspacePath, runFolder string, stepNumber, totalSteps int, stepID, stepPath, stepTitle string, now time.Time) *WorkflowCapacityWait {
	if err == nil || !llmerrors.IsQuotaExhausted(err) {
		return nil
	}
	wait := &WorkflowCapacityWait{
		SchemaVersion: 1,
		WorkspacePath: strings.TrimSpace(workspacePath),
		RunFolder:     strings.TrimSpace(runFolder),
		StepNumber:    stepNumber,
		StepID:        stepID,
		StepPath:      stepPath,
		StepTitle:     stepTitle,
		TotalSteps:    totalSteps,
		RetryAt:       llmerrors.RetryAtOrZero(err).UTC(),
		RecordedAt:    now.UTC(),
		Reason:        err.Error(),
	}
	var typed *llmerrors.Error
	if errors.As(err, &typed) {
		wait.Provider = typed.Provider
		wait.Model = typed.Model
		wait.Window = typed.Window
	}
	if wait.RetryAt.IsZero() {
		// Keep the zero value rather than a formatted epoch, so ResumeDue and
		// every reader see "unknown" instead of 1970.
		wait.RetryAt = time.Time{}
	}
	return wait
}

// recordWorkflowCapacityWait persists the record and returns the error the
// step loop should propagate, or nil when err was not quota exhaustion.
func (hcpo *StepBasedWorkflowOrchestrator) recordWorkflowCapacityWait(
	ctx context.Context,
	err error,
	stepNumber, totalSteps int,
	stepID, stepPath, stepTitle string,
) error {
	if hcpo == nil {
		return nil
	}
	wait := newWorkflowCapacityWait(err, hcpo.GetWorkspacePath(), hcpo.selectedRunFolder,
		stepNumber, totalSteps, stepID, stepPath, stepTitle, time.Now().UTC())
	if wait == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	path := WorkflowCapacityWaitPath(wait.WorkspacePath, wait.RunFolder)
	if body, marshalErr := json.MarshalIndent(wait, "", "  "); marshalErr == nil {
		if writeErr := hcpo.WriteWorkspaceFile(ctx, path, string(body)); writeErr != nil {
			// The record is what lets the scheduler suspend rather than fail,
			// and what survives a restart. If it cannot be written the run must
			// fall back to the ordinary failure path rather than pretend to be
			// suspended with nothing on disk to resume from.
			hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to persist capacity wait to %s: %v", path, writeErr))
			return nil
		}
	} else {
		hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to marshal capacity wait: %v", marshalErr))
		return nil
	}
	hcpo.GetLogger().Info(fmt.Sprintf("⏸️ Step %d hit a provider capacity wall; %s", stepNumber, wait.Describe()))
	return fmt.Errorf("%w: %s", ErrWorkflowWaitingForCapacity, wait.Describe())
}
