package server

import (
	"context"
	"fmt"
	"strings"
	"sync"

	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

// pulseWorklistRecordMu serializes the short read-then-write window used to
// make one conversation's Gate decision immutable once fully recorded.
var pulseWorklistRecordMu sync.Mutex

// pulseRunIDForSession resolves the only convenience value exposed to agents.
// A Pulse run is the active conversation, so its durable correlation id is the
// session id. It is not a lease, capability, or separately granted identity.
func pulseRunIDForSession(ctx context.Context, requestedRunID string) string {
	requestedRunID = strings.TrimSpace(requestedRunID)
	if requestedRunID == "current" {
		return strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
	}
	return requestedRunID
}

// validatePulseToolRunID validates only that the call has a durable correlation
// id. Workflow conversation exclusivity is the writer boundary; Pulse itself
// owns no extra lease, capability, or session authorization layer.
func validatePulseToolRunID(ctx context.Context, requestedRunID string) error {
	requestedRunID = pulseRunIDForSession(ctx, requestedRunID)
	if requestedRunID == "" {
		return fmt.Errorf("pulse_run_id is required; use \"current\" inside a conversation")
	}
	return nil
}
