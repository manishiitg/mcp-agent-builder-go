package server

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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
// builder conversation snapshot up with anything the Claude Code CLI's own
// on-disk transcript has recorded since the snapshot's last save (PLAT-178).
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

	since := parseBuilderConversationUpdatedAt(conv.UpdatedAt)
	newMessages, maxTimestamp, err := readNewClaudeTranscriptMessages(transcriptPath, since)
	if err != nil {
		log.Printf("[CHAT_HISTORY] Native transcript catch-up: failed to read %s: %v", transcriptPath, err)
		return conv
	}
	if len(newMessages) == 0 {
		return conv
	}

	conv.ConversationHistory = append(conv.ConversationHistory, newMessages...)
	if !maxTimestamp.IsZero() {
		conv.UpdatedAt = maxTimestamp.Format(time.RFC3339Nano)
	}

	record["conversation_history"] = conv.ConversationHistory
	record["updated_at"] = conv.UpdatedAt
	if encoded, err := json.MarshalIndent(record, "", "  "); err == nil {
		if err := writeRawFileToWorkspace(ctx, path, string(encoded)); err != nil {
			log.Printf("[CHAT_HISTORY] Native transcript catch-up: appended %d message(s) from %s but failed to persist to %s: %v", len(newMessages), transcriptPath, path, err)
		} else {
			log.Printf("[CHAT_HISTORY] Native transcript catch-up: appended %d message(s) to %s from %s (last save %s, native transcript through %s)",
				len(newMessages), path, transcriptPath, since.Format(time.RFC3339), maxTimestamp.Format(time.RFC3339))
		}
	}

	return conv
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
