package server

import (
	"context"
	"sync"
	"testing"

	internalevents "github.com/manishiitg/coding-agent-loop/agent_go/internal/events"
	mcpagent "github.com/manishiitg/mcpagent/agent"
	"github.com/manishiitg/mcpagent/llm"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// TestSteerBackgroundAgentCompletionFallsBackForFailedLiveDelivery verifies
// that an unconfirmed tmux send is not hidden in the agent steer queue. The
// caller-owned notification queue remains responsible for retrying it.
func TestSteerBackgroundAgentCompletionFallsBackForFailedLiveDelivery(t *testing.T) {
	store := internalevents.NewEventStore(10)
	defer store.Stop()

	sessionID := "busy-steerable-session"
	runningAgent := testCodingAgent(llm.ProviderCodexCLI, "codex-cli")

	api := &StreamingAPI{
		eventStore:       store,
		runningAgents:    map[string]*mcpagent.Agent{sessionID: runningAgent},
		runningAgentsMux: sync.RWMutex{},
		// An active foreground turn cancel handle is the proof that makes the
		// session steerable (canSteerSession).
		agentCancelFuncs: map[string]context.CancelFunc{sessionID: func() {}},
		agentCancelMux:   sync.RWMutex{},
		bgAgentRegistry:  NewBackgroundAgentRegistry(),
	}

	bg := &BackgroundAgent{
		ID:        "workflow-full-1",
		Name:      "full-workflow [default / iteration-0]",
		SessionID: sessionID,
		Status:    BGAgentFailed,
		Error:     "step 12 execution failed: input file not found",
	}
	api.bgAgentRegistry.Register(sessionID, bg)

	if !api.canSteerSession(sessionID) {
		t.Fatal("precondition: expected session to be steerable")
	}

	if api.steerBackgroundAgentCompletion(sessionID, bg.ID) {
		t.Fatal("steerBackgroundAgentCompletion = true for queued injection; want false so caller queues")
	}

	// Failed live delivery is not definitive, so the completion must not be
	// flagged notified. The caller's queue/drain backstop will redeliver it later.
	bg.mu.RLock()
	notified := bg.notified
	bg.mu.RUnlock()
	if notified {
		t.Fatal("agent.notified = true after queued injection; want false so the queue path can retry")
	}

	if api.steerBackgroundAgentCompletion(sessionID, bg.ID) {
		t.Fatal("second queued-injection steer call = true; want false until delivery is confirmed")
	}
}

// TestSteerBackgroundAgentCompletionFallsBackWhenNoRunningAgent verifies that the
// steer path declines (returns false) when there is no running agent to receive
// the message, so the caller falls back to the queue + drain-on-idle backstop.
func TestSteerBackgroundAgentCompletionFallsBackWhenNoRunningAgent(t *testing.T) {
	api := &StreamingAPI{
		runningAgents:    map[string]*mcpagent.Agent{},
		runningAgentsMux: sync.RWMutex{},
		bgAgentRegistry:  NewBackgroundAgentRegistry(),
	}
	const sessionID = "no-running-agent"
	bg := &BackgroundAgent{ID: "bg-1", SessionID: sessionID, Status: BGAgentCompleted, Result: "done"}
	api.bgAgentRegistry.Register(sessionID, bg)

	if api.steerBackgroundAgentCompletion(sessionID, bg.ID) {
		t.Fatal("steerBackgroundAgentCompletion = true with no running agent; want false so caller queues")
	}
	bg.mu.RLock()
	notified := bg.notified
	bg.mu.RUnlock()
	if notified {
		t.Fatal("agent.notified = true after a declined steer; the queue path must still own delivery")
	}
}

func TestSteerBackgroundAgentCompletionDefersPlainDelegation(t *testing.T) {
	store := internalevents.NewEventStore(10)
	defer store.Stop()

	sessionID := "busy-chief-session"
	runningAgent := testCodingAgent(llm.ProviderClaudeCode, "claude-code")

	api := &StreamingAPI{
		eventStore:       store,
		runningAgents:    map[string]*mcpagent.Agent{sessionID: runningAgent},
		runningAgentsMux: sync.RWMutex{},
		agentCancelFuncs: map[string]context.CancelFunc{sessionID: func() {}},
		agentCancelMux:   sync.RWMutex{},
		bgAgentRegistry:  NewBackgroundAgentRegistry(),
	}

	bg := &BackgroundAgent{
		ID:        "delegate-1",
		Name:      "Research task",
		SessionID: sessionID,
		Kind:      "delegation",
		Status:    BGAgentCompleted,
		Result:    "done",
	}
	api.bgAgentRegistry.Register(sessionID, bg)

	if api.steerBackgroundAgentCompletion(sessionID, bg.ID) {
		t.Fatal("plain delegation completion should not be live-steered; it needs a separate synthetic completion turn")
	}

	bg.mu.RLock()
	notified := bg.notified
	bg.mu.RUnlock()
	if notified {
		t.Fatal("plain delegation completion was marked notified before the synthetic turn")
	}
}

func testCodingAgent(provider llm.Provider, model string) *mcpagent.Agent {
	agent := &mcpagent.Agent{}
	mcpagent.ApplyAgentResumeHandle(agent, &mcpagent.AgentSessionHandle{
		Provider: llmtypes.CodingProviderSessionHandle{Provider: string(provider), Model: model},
	})
	return agent
}
