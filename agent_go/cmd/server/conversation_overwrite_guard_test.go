package server

import (
	"testing"

	llmtypes "github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func histEntry(role string) map[string]interface{} { return map[string]interface{}{"role": role} }
func msg(role string) llmtypes.MessageContent {
	return llmtypes.MessageContent{Role: llmtypes.ChatMessageType(role)}
}

// TestUserTurnsAreTheInvariantWorthGuarding.
//
// Assistant and tool entries are legitimately rewritten, summarized and trimmed
// by normal persistence, so a drop in their count proves nothing. User turns
// only ever grow — a session cannot un-ask a question — which is what makes a
// shrink detectable as a partial rebuild rather than ordinary cleaning.
func TestUserTurnsAreTheInvariantWorthGuarding(t *testing.T) {
	onDisk := []map[string]interface{}{
		histEntry("system"), histEntry("human"), histEntry("ai"), histEntry("tool"),
		histEntry("ai"), histEntry("human"), histEntry("ai"), histEntry("tool"), histEntry("tool"),
	}
	if got := conversationUserTurnCount(onDisk); got != 2 {
		t.Fatalf("user turns on disk = %d, want 2", got)
	}
	// "user" and "human" are both used across the codebase's history shapes.
	if got := conversationUserTurnCount([]map[string]interface{}{histEntry("user"), histEntry("HUMAN")}); got != 2 {
		t.Errorf("role spellings not both counted: %d", got)
	}

	incoming := []llmtypes.MessageContent{msg("system"), msg("human"), msg("ai")}
	if got := messageUserTurnCount(incoming); got != 1 {
		t.Fatalf("user turns incoming = %d, want 1", got)
	}
}

// TestAThinRebuildDoesNotReplaceAFullerRecord is the salesoutreach loss.
//
// A restart cleared the in-memory events, the resume rebuilt a thin history,
// and persistence wrote it straight over the full one — 242 user turns in
// Claude Code's own transcript, 2 in the file the app kept. One copy on disk,
// no backup.
func TestAThinRebuildDoesNotReplaceAFullerRecord(t *testing.T) {
	onDisk := make([]map[string]interface{}, 0, 40)
	for i := 0; i < 20; i++ {
		onDisk = append(onDisk, histEntry("human"), histEntry("ai"))
	}
	thin := []llmtypes.MessageContent{msg("human"), msg("ai")}

	if conversationUserTurnCount(onDisk) <= messageUserTurnCount(thin) {
		t.Fatal("test setup no longer represents a thin rebuild")
	}
	// The guard's comparison, isolated from workspace IO.
	if !(messageUserTurnCount(thin) < conversationUserTurnCount(onDisk)) {
		t.Error("a 2-turn rebuild was not recognized as lossy against a 20-turn record")
	}
}

// TestAGrowingConversationIsAlwaysAllowedThrough guards the direction that
// would be far worse than the bug: refusing legitimate writes would freeze
// every conversation at its first turn.
func TestAGrowingConversationIsAlwaysAllowedThrough(t *testing.T) {
	onDisk := []map[string]interface{}{histEntry("human"), histEntry("ai")}
	grown := []llmtypes.MessageContent{msg("human"), msg("ai"), msg("human"), msg("ai")}
	if messageUserTurnCount(grown) < conversationUserTurnCount(onDisk) {
		t.Error("a conversation that gained a turn was treated as lossy")
	}
	// Equal counts must pass: the same turn re-persisted with more tool detail
	// is the single most common write there is.
	same := []llmtypes.MessageContent{msg("human"), msg("ai"), msg("tool")}
	if messageUserTurnCount(same) < conversationUserTurnCount(onDisk) {
		t.Error("an equal-turn rewrite was treated as lossy")
	}
}
