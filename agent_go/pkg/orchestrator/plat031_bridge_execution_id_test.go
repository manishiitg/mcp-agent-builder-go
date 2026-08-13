package orchestrator

import (
	"context"
	"testing"
	"time"

	unifiedevents "github.com/manishiitg/mcpagent/events"
)

// sendTestTokenUsageEvent pushes a synthetic TokenUsageEvent through the
// bridge the same way TestWorkflowCostBucketingThroughBridgeReal does, but
// without a real provider call — these tests only care about step
// attribution, not token accounting.
func sendTestTokenUsageEvent(t *testing.T, bridge *ContextAwareEventBridge, persister *recordingTokenPersister) persistCall {
	t.Helper()
	before := len(persister.snapshot())
	if err := bridge.HandleEvent(context.Background(), &unifiedevents.AgentEvent{
		Type:      unifiedevents.TokenUsage,
		Timestamp: time.Now(),
		Component: "test",
		Data: &unifiedevents.TokenUsageEvent{
			ModelID:          "claude-opus-5",
			Provider:         "anthropic",
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}); err != nil {
		t.Fatalf("bridge.HandleEvent: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls := persister.snapshot(); len(calls) > before {
			return calls[len(calls)-1]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("persister did not receive a call in time")
	return persistCall{}
}

// TestBridgeStampsStableExecutionIDAcrossTurnsAndDiffersAcrossBridges proves
// the ExecutionID a bridge attaches to StepTokenData is (a) stable across
// multiple turns of the same run and (b) unique per run, which is what lets
// two executions that reuse the same run folder stay distinguishable
// (PLAT-031).
func TestBridgeStampsStableExecutionIDAcrossTurnsAndDiffersAcrossBridges(t *testing.T) {
	bridgeA := NewContextAwareEventBridge(&noopListener{}, &silentLoggerV2{})
	persisterA := &recordingTokenPersister{}
	bridgeA.SetTokenPersister(persisterA)
	bridgeA.SetIterationFolder("runs/test-run-001")
	bridgeA.PushContext("execution", 1, "fetch-data", "worker-agent")

	call1 := sendTestTokenUsageEvent(t, bridgeA, persisterA)
	call2 := sendTestTokenUsageEvent(t, bridgeA, persisterA)
	bridgeA.PopContext()

	if call1.stepTokenData.ExecutionID == "" {
		t.Fatal("ExecutionID is empty; PLAT-031 identity was not attached")
	}
	if call1.stepTokenData.ExecutionID != call2.stepTokenData.ExecutionID {
		t.Fatalf("ExecutionID changed within one run: turn1=%q turn2=%q",
			call1.stepTokenData.ExecutionID, call2.stepTokenData.ExecutionID)
	}

	bridgeB := NewContextAwareEventBridge(&noopListener{}, &silentLoggerV2{})
	persisterB := &recordingTokenPersister{}
	bridgeB.SetTokenPersister(persisterB)
	bridgeB.SetIterationFolder("runs/test-run-001") // same run folder reused by a second execution
	bridgeB.PushContext("execution", 1, "fetch-data", "worker-agent")
	call3 := sendTestTokenUsageEvent(t, bridgeB, persisterB)
	bridgeB.PopContext()

	if call3.stepTokenData.ExecutionID == call1.stepTokenData.ExecutionID {
		t.Fatal("two separate bridge instances (two executions) produced the same ExecutionID")
	}
}

// TestBridgeReclassifiesNumericStepIDAsScheduleMessagePhase covers the
// exact PLAT-031 evidence shape: a cost event attributed with a bare
// numeric step ID (e.g. a schedule-message counter) must never be bucketed
// under a step-shaped phase like "execution_only" — it gets moved to the
// explicit "schedule_message" phase instead. A real, descriptive step ID is
// left untouched.
func TestBridgeReclassifiesNumericStepIDAsScheduleMessagePhase(t *testing.T) {
	cases := []struct {
		name       string
		phase      string
		stepID     string
		wantPhase  string
		wantStepID string
	}{
		{
			name:       "explicit execution_only phase with numeric stepID gets reclassified",
			phase:      "execution_only",
			stepID:     "10",
			wantPhase:  "schedule_message",
			wantStepID: "10",
		},
		{
			name:       "auto-derived phase with numeric stepID gets reclassified",
			phase:      "",
			stepID:     "10",
			wantPhase:  "schedule_message",
			wantStepID: "10",
		},
		{
			name:       "descriptive stepID is left untouched",
			phase:      "execution_only",
			stepID:     "fetch-data",
			wantPhase:  "execution_only",
			wantStepID: "fetch-data",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bridge := NewContextAwareEventBridge(&noopListener{}, &silentLoggerV2{})
			persister := &recordingTokenPersister{}
			bridge.SetTokenPersister(persister)
			bridge.SetIterationFolder("runs/test-run-001")
			bridge.PushContext(tc.phase, 10, tc.stepID, "worker-agent")
			call := sendTestTokenUsageEvent(t, bridge, persister)
			bridge.PopContext()

			if call.stepTokenData.Phase != tc.wantPhase {
				t.Fatalf("Phase = %q, want %q", call.stepTokenData.Phase, tc.wantPhase)
			}
			if call.stepTokenData.StepID != tc.wantStepID {
				t.Fatalf("StepID = %q, want %q", call.stepTokenData.StepID, tc.wantStepID)
			}
		})
	}
}
