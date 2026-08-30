package server

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	llmtypes "github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// conversationUserTurnCount counts the human turns in a persisted record.
//
// User turns are the one part of a conversation that only ever grows. Assistant
// and tool entries get rewritten, summarised and trimmed by normal persistence
// (cleanChatHistoryForPersistence exists to do exactly that), so their count
// shrinking proves nothing. A session cannot un-ask a question.
func conversationUserTurnCount(history []map[string]interface{}) int {
	count := 0
	for _, entry := range history {
		role, _ := entry["role"].(string)
		switch strings.TrimSpace(strings.ToLower(role)) {
		case "human", "user":
			count++
		}
	}
	return count
}

// messageUserTurnCount counts the human turns in what is about to be written.
func messageUserTurnCount(history []llmtypes.MessageContent) int {
	count := 0
	for _, message := range history {
		switch strings.TrimSpace(strings.ToLower(string(message.Role))) {
		case "human", "user":
			count++
		}
	}
	return count
}

// conversationOverwriteWouldLoseTurns reports whether writing this history over
// the record already on disk would drop user turns, and how many each side has.
//
// salesoutreach, 2026-08-18: a chat with 242 user turns in Claude Code's own
// transcript was persisted as 2 after a server restart. The restart cleared the
// in-memory events, the resume rebuilt a thin history from what was left, and
// persistence wrote that thin version straight over the full one. There was a
// single copy on disk and no backup, so the app's record of that conversation
// was gone — recoverable only because the CLI keeps its own transcript.
//
// Refusing the write is the right failure. A stale-but-complete record is
// strictly better than a fresh-but-empty one: the next successful turn rewrites
// it correctly, whereas a lost conversation does not come back.
func conversationOverwriteWouldLoseTurns(ctx context.Context, conversationPath string, next []llmtypes.MessageContent) (bool, int, int) {
	if strings.TrimSpace(conversationPath) == "" {
		return false, 0, 0
	}
	content, exists, err := readFileFromWorkspace(ctx, conversationPath)
	if err != nil || !exists || strings.TrimSpace(content) == "" {
		return false, 0, 0
	}
	var existing struct {
		ConversationHistory []map[string]interface{} `json:"conversation_history"`
	}
	if err := json.Unmarshal([]byte(content), &existing); err != nil {
		// An unreadable record cannot be compared, and refusing to write would
		// mean a corrupt file could never be replaced.
		return false, 0, 0
	}
	existingTurns := conversationUserTurnCount(existing.ConversationHistory)
	nextTurns := messageUserTurnCount(next)
	return nextTurns < existingTurns, existingTurns, nextTurns
}

// refuseConversationOverwrite logs the refusal in the terms that matter: what
// would have been lost, and where the authoritative copy still is.
func refuseConversationOverwrite(conversationPath string, existingTurns, nextTurns int) {
	log.Printf("[CHAT HISTORY] REFUSED to overwrite %s: on disk has %d user turn(s), this write has %d. "+
		"A session cannot lose user turns, so this is a partial rebuild (usually after a restart cleared in-memory events). "+
		"Keeping the fuller record; the next complete turn will rewrite it.",
		conversationPath, existingTurns, nextTurns)
}
