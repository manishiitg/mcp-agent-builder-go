package server

import (
	"testing"
	"time"
)

func TestClaimAgentWorksChatAllowsOneChatPlusOneSchedule(t *testing.T) {
	api := &StreamingAPI{activeSessions: make(map[string]*ActiveSessionInfo)}
	userID := "user-1"

	if blocking := api.claimAgentWorksChatSession("chat-1", userID, "hello", "manual"); blocking != nil {
		t.Fatalf("first chat unexpectedly blocked by %s", blocking.SessionID)
	}
	api.activeSessions["schedule-cron--workflow_1"] = &ActiveSessionInfo{
		SessionID:    "schedule-cron--workflow_1",
		AgentMode:    "multi-agent",
		Status:       "running",
		UserID:       userID,
		TriggeredBy:  "cron",
		LastActivity: time.Now(),
	}

	if blocking := api.claimAgentWorksChatSession("chat-1", userID, "follow up", "manual"); blocking != nil {
		t.Fatalf("same chat follow-up unexpectedly blocked by %s", blocking.SessionID)
	}
	if blocking := api.claimAgentWorksChatSession("chat-2", userID, "second chat", "manual"); blocking == nil || blocking.SessionID != "chat-1" {
		t.Fatalf("second chat blocker = %#v, want chat-1", blocking)
	}
}

func TestAgentWorksChatClaimIgnoresCompletedUnretainedChat(t *testing.T) {
	api := &StreamingAPI{activeSessions: map[string]*ActiveSessionInfo{
		"old-chat": {
			SessionID: "old-chat",
			AgentMode: "multi-agent",
			Status:    "completed",
			UserID:    "user-1",
		},
	}}

	if blocking := api.claimAgentWorksChatSession("new-chat", "user-1", "hello", "manual"); blocking != nil {
		t.Fatalf("completed chat unexpectedly blocked new chat: %#v", blocking)
	}
}
