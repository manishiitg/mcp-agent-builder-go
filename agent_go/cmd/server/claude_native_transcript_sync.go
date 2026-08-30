package server

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// claudeNativeTranscriptRuntime is the minimal subset of a persisted builder
// conversation's "runtime" object needed to locate the coding CLI's own
// on-disk transcript for the session that produced it.
type claudeNativeTranscriptRuntime struct {
	Provider           string `json:"provider"`
	ExternalSessionID  string `json:"external_session_id"`
	AgentSessionHandle *struct {
		Provider *struct {
			Provider        string `json:"provider"`
			NativeSessionID string `json:"native_session_id"`
			WorkingDir      string `json:"working_dir"`
		} `json:"provider"`
	} `json:"agent_session_handle"`
}

type claudeTranscriptEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type claudeTranscriptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeTranscriptContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// refreshLatestBuilderConversationFromNativeTranscript catches a persisted
// builder conversation snapshot up with the Claude Code CLI's own on-disk
// transcript (PLAT-178).
//
// Best-effort throughout: any resolution failure (no runtime info, no
// matching transcript file, nothing new) returns log unchanged. A
// successful catch-up is also persisted back to path so later reads of the
// same file -- not just this one restore response -- see it too.
func (api *StreamingAPI) refreshLatestBuilderConversationFromNativeTranscript(ctx context.Context, path, rawContent string, conv builderConversationLog) builderConversationLog {
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(rawContent), &record); err != nil {
		return conv
	}
	runtimeRaw, ok := record["runtime"]
	if !ok {
		return conv
	}
	runtimeBytes, err := json.Marshal(runtimeRaw)
	if err != nil {
		return conv
	}
	var runtime claudeNativeTranscriptRuntime
	if err := json.Unmarshal(runtimeBytes, &runtime); err != nil {
		return conv
	}

	provider := strings.ToLower(strings.TrimSpace(runtime.Provider))
	nativeSessionID := strings.TrimSpace(runtime.ExternalSessionID)
	workingDir := ""
	if runtime.AgentSessionHandle != nil && runtime.AgentSessionHandle.Provider != nil {
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(runtime.AgentSessionHandle.Provider.Provider))
		}
		if nativeSessionID == "" {
			nativeSessionID = strings.TrimSpace(runtime.AgentSessionHandle.Provider.NativeSessionID)
		}
		workingDir = strings.TrimSpace(runtime.AgentSessionHandle.Provider.WorkingDir)
	}
	if provider != "claude-code" || nativeSessionID == "" || workingDir == "" {
		return conv
	}

	transcriptPath, err := resolveClaudeNativeTranscriptPath(workingDir, nativeSessionID)
	if err != nil || transcriptPath == "" {
		return conv
	}

	// Do not use conv.UpdatedAt as a transcript cursor. The live-input path
	// advances updated_at when it persists each human message, but it cannot
	// persist the corresponding assistant reply. A later human message can
	// therefore advance updated_at past an earlier missing reply. Reading the
	// full native transcript and sequence-merging it is what recovers those
	// interleaved replies without duplicating the already-persisted humans.
	nativeMessages, maxTimestamp, err := readNewClaudeTranscriptMessages(transcriptPath, time.Time{})
	if err != nil {
		log.Printf("[CHAT_HISTORY] Native transcript catch-up: failed to read %s: %v", transcriptPath, err)
		return conv
	}
	if len(nativeMessages) == 0 {
		return conv
	}

	previousCount := len(conv.ConversationHistory)
	merged := mergeBuilderConversationHistory(conv.ConversationHistory, nativeMessages)
	if builderConversationHistoriesEqual(merged, conv.ConversationHistory) {
		return conv
	}
	conv.ConversationHistory = merged
	if current := parseBuilderConversationUpdatedAt(conv.UpdatedAt); maxTimestamp.After(current) {
		conv.UpdatedAt = maxTimestamp.Format(time.RFC3339Nano)
	}

	record["conversation_history"] = conv.ConversationHistory
	record["updated_at"] = conv.UpdatedAt
	if encoded, err := json.MarshalIndent(record, "", "  "); err == nil {
		if err := writeRawFileToWorkspace(ctx, path, string(encoded)); err != nil {
			log.Printf("[CHAT_HISTORY] Native transcript catch-up: merged %d missing message(s) from %s but failed to persist to %s: %v", len(merged)-previousCount, transcriptPath, path, err)
		} else {
			log.Printf("[CHAT_HISTORY] Native transcript catch-up: merged %d missing message(s) into %s from %s (native transcript through %s)",
				len(merged)-previousCount, path, transcriptPath, maxTimestamp.Format(time.RFC3339))
		}
	}

	return conv
}

// mergeBuilderConversationHistory returns a shortest practical union of the
// persisted and native message sequences. Persisted-only messages are kept;
// native-only messages (most importantly assistant replies omitted by the
// asynchronous live-input writer) are inserted at their native position; and
// matching live-input human messages are not appended a second time.
//
// Positions are indexed by a normalized role+text key, so matching is linear
// apart from binary searches and repeated identical messages retain their
// multiplicity in sequence order.
func mergeBuilderConversationHistory(persisted, native []builderConversationMessage) []builderConversationMessage {
	if len(persisted) == 0 {
		return append([]builderConversationMessage(nil), native...)
	}
	if len(native) == 0 {
		return append([]builderConversationMessage(nil), persisted...)
	}

	positions := make(map[string][]int, len(persisted))
	for index, message := range persisted {
		positions[builderConversationMessageKey(message)] = append(positions[builderConversationMessageKey(message)], index)
	}

	// Find the first shared anchor. Persisted history may include conversation
	// from before the native session began, so that prefix must remain first.
	firstNative, firstPersisted := -1, -1
	for nativeIndex, message := range native {
		if candidates := positions[builderConversationMessageKey(message)]; len(candidates) > 0 {
			firstNative, firstPersisted = nativeIndex, candidates[0]
			break
		}
	}
	if firstNative < 0 {
		merged := append([]builderConversationMessage(nil), persisted...)
		return append(merged, native...)
	}

	merged := make([]builderConversationMessage, 0, len(persisted)+len(native))
	merged = append(merged, persisted[:firstPersisted]...)
	merged = append(merged, native[:firstNative]...)
	merged = append(merged, persisted[firstPersisted])
	persistedCursor := firstPersisted + 1

	for _, message := range native[firstNative+1:] {
		candidates := positions[builderConversationMessageKey(message)]
		candidateIndex := sort.SearchInts(candidates, persistedCursor)
		if candidateIndex < len(candidates) {
			matchedPersisted := candidates[candidateIndex]
			merged = append(merged, persisted[persistedCursor:matchedPersisted+1]...)
			persistedCursor = matchedPersisted + 1
			continue
		}
		merged = append(merged, message)
	}
	merged = append(merged, persisted[persistedCursor:]...)
	return merged
}

func builderConversationMessageKey(message builderConversationMessage) string {
	texts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		if text := strings.Join(strings.Fields(part.Text), " "); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.ToLower(strings.TrimSpace(message.Role)) + "\x00" + strings.Join(texts, "\n")
}

func builderConversationHistoriesEqual(left, right []builderConversationMessage) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if builderConversationMessageKey(left[index]) != builderConversationMessageKey(right[index]) {
			return false
		}
	}
	return true
}

// resolveClaudeNativeTranscriptPath locates Claude Code's own JSONL
// transcript for a session, mirroring the working-directory-to-project-slug
// scheme multi-llm-provider-go's claudecode adapter uses
// (pkg/adapters/claudecode/claudecode_transcript_path.go) plus its
// session-ID glob fallback for when that escaping scheme has changed across
// CLI versions. Duplicated here (rather than imported) because that
// resolver is unexported and this is a narrow, self-contained lookup.
func resolveClaudeNativeTranscriptPath(workingDir, sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	candidates := []string{workingDir}
	if abs, err := filepath.Abs(workingDir); err == nil {
		candidates = append(candidates, abs)
	}
	if resolved, err := filepath.EvalSymlinks(workingDir); err == nil {
		candidates = append(candidates, resolved)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, dir := range candidates {
		slug := claudeNativeTranscriptProjectSlug(dir)
		if slug == "" {
			continue
		}
		if _, dup := seen[slug]; dup {
			continue
		}
		seen[slug] = struct{}{}
		path := filepath.Join(home, ".claude", "projects", slug, sessionID+".jsonl")
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			return path, nil
		}
	}

	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return "", err
	}
	return matches[0], nil
}

func claudeNativeTranscriptProjectSlug(workingDir string) string {
	workingDir = filepath.Clean(strings.TrimSpace(workingDir))
	if workingDir == "" || workingDir == "." {
		return ""
	}
	return strings.NewReplacer(
		"/", "-",
		"\\", "-",
		"_", "-",
		".", "-",
		":", "-",
	).Replace(workingDir)
}

// readNewClaudeTranscriptMessages parses Claude Code's JSONL transcript and
// returns only the human-visible user/assistant messages timestamped after
// since, converted to the same builderConversationMessage shape the rest of
// the builder conversation log already uses.
func readNewClaudeTranscriptMessages(path string, since time.Time) ([]builderConversationMessage, time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}

	var messages []builderConversationMessage
	maxTimestamp := since
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry claudeTranscriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			continue
		}
		if !ts.After(since) {
			continue
		}
		if len(entry.Message) == 0 {
			continue
		}
		var msg claudeTranscriptMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		text := extractClaudeTranscriptText(msg.Content)
		if text == "" {
			continue
		}

		role := "ai"
		if entry.Type == "user" {
			role = "human"
		}
		messages = append(messages, builderConversationMessage{
			Role:  role,
			Parts: []builderConversationPart{{Text: text}},
		})
		if ts.After(maxTimestamp) {
			maxTimestamp = ts
		}
	}
	return messages, maxTimestamp, nil
}

// extractClaudeTranscriptText pulls the human-visible text out of a
// transcript entry's message.content, which is either a plain string (a
// real typed message) or a list of content blocks (thinking/text/tool_use
// for assistant turns, tool_result for a user-role entry that is actually
// just a tool's output echoed back, not something the user typed). Only
// plain-string content and "text"-typed blocks are kept -- tool_use,
// tool_result, and thinking are execution detail the chat history was never
// meant to display, and including them would clutter it, not fix it.
func extractClaudeTranscriptText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(content, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var blocks []claudeTranscriptContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != "text" {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
