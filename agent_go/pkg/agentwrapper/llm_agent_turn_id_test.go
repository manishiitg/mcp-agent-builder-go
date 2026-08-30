package agent

import (
	"context"
	"testing"
	"time"

	mcpagent "github.com/manishiitg/mcpagent/agent"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

type turnIDTestModel struct{}

func (turnIDTestModel) GenerateContent(context.Context, []llmtypes.MessageContent, ...llmtypes.CallOption) (*llmtypes.ContentResponse, error) {
	return &llmtypes.ContentResponse{Choices: []*llmtypes.ContentChoice{{Content: "done"}}}, nil
}

func (turnIDTestModel) GetModelID() string { return "turn-id-test" }

func (turnIDTestModel) GetModelMetadata(string) (*llmtypes.ModelMetadata, error) {
	return &llmtypes.ModelMetadata{ModelID: "turn-id-test"}, nil
}

// PLAT-180. currentTurnID is the glue between agent_go's tool-call hook and
// mcpagent's Session.ActiveTurnID -- it must ask the live session rather than
// unconditionally trusting the fallback (the stale, closure-captured turn ID
// that caused the original bug). The "does ActiveTurnID itself reflect a
// retained turn's own, distinct ID" question is covered live, with real
// startRetainedCompletionWatch machinery, by
// TestActiveTurnIDReflectsTheRetainedTurnNotAnEarlierCachedTurn in
// mcpagent/agent/turn_session_retained_completion_test.go. These tests cover
// the delegation itself: no session, and a session with nothing active.
func TestCurrentTurnIDFallsBackWhenSessionIsNil(t *testing.T) {
	wrapper := &LLMAgentWrapper{}
	if got := wrapper.currentTurnID("fallback-id"); got != "fallback-id" {
		t.Fatalf("currentTurnID() with nil session = %q, want fallback-id", got)
	}
}

func TestCurrentTurnIDFallsBackWhenSessionHasNoActiveTurn(t *testing.T) {
	const sessionID = "wrapper-current-turn-id-idle-test"
	mcpagent.CloseSession(sessionID)
	t.Cleanup(func() { mcpagent.CloseSession(sessionID) })

	runtime := mcpagent.RuntimeConfig{
		Model:         turnIDTestModel{},
		Generation:    mcpagent.GenerationRuntimeConfig{MaxTurns: 1},
		MCP:           mcpagent.MCPRuntimeConfig{SessionID: sessionID},
		Observability: mcpagent.ObservabilityRuntimeConfig{Logger: loggerv2.NewNoop()},
	}
	draft, err := mcpagent.NewAgentFromDefinition(context.Background(), mcpagent.AgentDefinition{}, runtime)
	if err != nil {
		t.Fatal(err)
	}
	session, err := draft.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	wrapper := &LLMAgentWrapper{
		agent:   draft,
		session: session,
		config:  LLMAgentConfig{},
		metrics: &agentMetricsImpl{MinLatency: time.Duration(^uint64(0) >> 1)},
		logger:  loggerv2.NewNoop(),
		runtime: runtime,
	}

	// No Run/Send has happened yet on this session -- nothing active.
	if got := wrapper.currentTurnID("fallback-id"); got != "fallback-id" {
		t.Fatalf("currentTurnID() on an idle session = %q, want fallback-id", got)
	}
}
