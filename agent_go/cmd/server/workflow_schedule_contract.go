package server

import (
	"fmt"
	"strings"
)

// scheduleMessagesNeedExplicitReason distinguishes a compact trigger from a
// schedule-local procedure. Direct procedures are supported, but unlike planned
// routes they do not automatically receive the canonical step learning,
// validation, retry, and Pulse-attribution lifecycle. Requiring an explicit
// reason makes that architecture choice visible instead of silently banning it.
func scheduleMessagesNeedExplicitReason(messages []string) (bool, string) {
	nonEmptyCount := 0
	for index, raw := range messages {
		message := strings.TrimSpace(raw)
		if message == "" {
			continue
		}
		nonEmptyCount++
		if len(message) > 1600 {
			return true, fmt.Sprintf("messages[%d] is %d characters", index, len(message))
		}
		lower := strings.ToLower(message)
		for _, marker := range []string{
			"sqlite3 ", "select ", "insert ", "update ", "curl ", "git ",
			"execute_step(", "notify_user", "backup/", "step 1", "step 2",
		} {
			if strings.Contains(lower, marker) {
				return true, fmt.Sprintf("messages[%d] contains procedure marker %q", index, marker)
			}
		}
	}
	if nonEmptyCount > 1 {
		return true, fmt.Sprintf("schedule has %d sequential workshop messages", nonEmptyCount)
	}
	return false, ""
}

func validateScheduleMessages(messages []string, directMessagesReason string) error {
	needsReason, signal := scheduleMessagesNeedExplicitReason(messages)
	if needsReason && strings.TrimSpace(directMessagesReason) == "" {
		return fmt.Errorf("direct schedule messages are supported, but this queue needs direct_messages_reason because %s; explain why this work is genuinely schedule-specific and acknowledge that it lacks canonical step-level learnings, validation/retry, and Pulse attribution, or move the durable procedure into a planned route", signal)
	}
	return nil
}

func scheduleMessagesAdvisory(messages []string, directMessagesReason string) string {
	needsReason, _ := scheduleMessagesNeedExplicitReason(messages)
	if !needsReason {
		return ""
	}
	return fmt.Sprintf("Direct schedule sequence retained intentionally (%s). It runs as workshop conversation turns, not canonical plan steps, so step-level learnings, validation/retry, and Pulse attribution are not automatic.", strings.TrimSpace(directMessagesReason))
}
