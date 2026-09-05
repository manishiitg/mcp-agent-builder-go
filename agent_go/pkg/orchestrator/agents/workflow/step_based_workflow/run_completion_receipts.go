package step_based_workflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var errCompletionExecutionChanged = errors.New("run execution changed while recording completion")

// Completion receipts replace disabled steps_done persistence for recovery
// proof only. They are not used to choose which steps should execute.
func (hcpo *StepBasedWorkflowOrchestrator) beginCompletionReceipts(ctx context.Context, steps []PlanStepInterface, execCtx *ExecutionContext) (string, string, string) {
	folder := hcpo.selectedRunFolder
	if execCtx != nil && execCtx.StepPathOverride != "" {
		return folder, "", ""
	}
	raw, err := hcpo.ReadWorkspaceFile(ctx, workflowRunMetadataPath(folder))
	if err != nil {
		return folder, "", ""
	}
	var meta map[string]interface{}
	if json.Unmarshal([]byte(raw), &meta) != nil {
		return folder, "", ""
	}
	id, _ := meta["execution_id"].(string)
	if id == "" {
		return folder, "", ""
	}
	revision, err := hcpo.ensureExecutablePlanRevision(ctx)
	if err != nil {
		return folder, "", ""
	}
	err = hcpo.updateCompletionMetadata(ctx, folder, id, func(m map[string]interface{}) {
		if m["execution_id"] != id {
			return
		}
		if execCtx == nil || !execCtx.RunSingleStepOnly {
			ids := make([]string, 0, len(steps))
			for _, step := range steps {
				ids = append(ids, step.GetID())
			}
			m["required_step_ids"] = ids
			m["completion_receipts"] = map[string]interface{}{}
		} else if execCtx.SingleStepTarget >= 0 && execCtx.SingleStepTarget < len(steps) {
			if receipts, ok := m["completion_receipts"].(map[string]interface{}); ok {
				delete(receipts, steps[execCtx.SingleStepTarget].GetID())
			}
		}
	})
	if err != nil {
		if !errors.Is(err, errCompletionExecutionChanged) {
			hcpo.recordRunPersistenceError(ctx, "completion_receipts", err)
		}
		return folder, "", ""
	}
	return folder, id, revision
}

func (hcpo *StepBasedWorkflowOrchestrator) finishCompletionReceipts(ctx context.Context, folder, id, revision string, steps []PlanStepInterface, progress *StepProgress) {
	if id == "" || progress == nil {
		return
	}
	err := hcpo.updateCompletionMetadata(ctx, folder, id, func(meta map[string]interface{}) {
		if meta["execution_id"] != id {
			return
		}
		receipts, _ := meta["completion_receipts"].(map[string]interface{})
		if receipts == nil {
			receipts = map[string]interface{}{}
		}
		for _, index := range progress.CompletedStepIndices {
			if index >= 0 && index < len(steps) {
				receipts[steps[index].GetID()] = map[string]interface{}{"execution_id": id, "plan_revision": revision, "completed_at": formatRFC3339UTC(time.Now().UTC())}
			}
		}
		meta["completion_receipts"] = receipts
	})
	if err != nil && !errors.Is(err, errCompletionExecutionChanged) {
		hcpo.recordRunPersistenceError(ctx, "completion_receipts", err)
	}
}

// Recovery evidence must never recreate missing or corrupt run metadata.
func (hcpo *StepBasedWorkflowOrchestrator) updateCompletionMetadata(ctx context.Context, folder, id string, mutate func(map[string]interface{})) error {
	unlock := lockRunMetadata(hcpo.GetWorkspacePath(), folder)
	defer unlock()
	path := workflowRunMetadataPath(folder)
	raw, err := hcpo.ReadWorkspaceFile(ctx, path)
	if err != nil {
		return err
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return err
	}
	if meta["execution_id"] != id {
		return errCompletionExecutionChanged
	}
	mutate(meta)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return hcpo.WriteWorkspaceFile(ctx, path, string(data))
}
