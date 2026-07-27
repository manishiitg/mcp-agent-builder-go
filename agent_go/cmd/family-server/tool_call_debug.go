package main

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
)

// debugToolCall is one raw tool invocation captured for the tool-call
// visibility feature in the UI — lets the parent/child see exactly what the
// agent called each turn, and (once it returns) what it got back, so a
// confusing reply ("I can't reach the workspace tools") can actually be
// checked against what really happened instead of taken on faith.
type debugToolCall struct {
	Tool   string `json:"tool"`
	Args   string `json:"args,omitempty"`
	Result string `json:"result,omitempty"`
	Err    string `json:"err,omitempty"`
}

// withToolCallDebug wraps every tool's Handler to record its name plus a short
// args summary before running it — regardless of whether the tool also has a
// withLiveStatus label — AND publish it live on conversationID's SSE status
// stream (see statusHub.publishToolCall) so the UI shows each call the moment
// it happens instead of only once the whole turn finishes (a turn with many
// tool calls could otherwise look totally silent for minutes, then dump every
// debug bubble at once right at the end). Purely observational: never changes
// a tool's behavior, arguments, or return value.
// It also feeds the turn's latency trace (turntrace.go) — this is already the
// one place every tool call funnels through, so timing them here keeps that
// measurement identical across surfaces and engines instead of per-tool.
// trace may be nil.
func withToolCallDebug(mu *sync.Mutex, calls *[]debugToolCall, conversationID string, trace *turnTrace, tools []agentsession.Tool) []agentsession.Tool {
	out := make([]agentsession.Tool, len(tools))
	for i, t := range tools {
		name := t.Name
		orig := t.Handler
		t.Handler = func(ctx context.Context, args map[string]interface{}) (string, error) {
			argSummary := summarizeToolArgs(args)
			trace.tool(name)
			statusHubs.publishToolCall(conversationID, name, argSummary)
			result, err := orig(ctx, args)
			call := debugToolCall{Tool: name, Args: argSummary, Result: truncateForDebug(result)}
			if err != nil {
				call.Err = err.Error()
			}
			mu.Lock()
			*calls = append(*calls, call)
			mu.Unlock()
			return result, err
		}
		out[i] = t
	}
	return out
}

// summarizeToolArgs renders a tool call's arguments as compact JSON, truncated
// so a large payload (e.g. a shell command's full script) doesn't blow up the
// debug panel.
func summarizeToolArgs(args map[string]interface{}) string {
	b, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return truncateForDebug(string(b))
}

// truncateForDebug caps a tool call's result (e.g. a shell command's full
// stdout, or a long file listing) so one verbose call can't blow up the
// debug panel or the response payload.
func truncateForDebug(s string) string {
	const maxLen = 2000
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}
