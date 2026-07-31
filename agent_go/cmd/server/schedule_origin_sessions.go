package server

import (
	"context"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

// Terminals belong to whichever session ran them, and a scheduled run always
// mints its own session ("schedule-manual--<id>_<ts>"). So a run triggered from
// a chat records every one of its step and sub-agent terminals under that new
// session, and the chat that started it shows only its own agent — the work it
// asked for appears nowhere in the tab that asked.
//
// This maps a scheduled run's session back to the chat that triggered it, so a
// session-scoped terminal listing can include runs started from that chat. Only
// manual triggers carry an origin; cron runs have no originating chat and stay
// exactly as they are.
//
// Kept as a side table rather than a field on the terminal snapshot because the
// link is knowable only at trigger time, while snapshots are built later from
// streamed events — threading it through that pipeline would touch the
// scheduler, the event bridge, and the store to express one edge.
type scheduleOriginRegistry struct {
	mu sync.RWMutex
	// originBySchedule maps a scheduled run's session ID to the chat session
	// that triggered it.
	originBySchedule map[string]string
	// schedulesByOrigin is the reverse index, so a listing for one chat does not
	// scan every scheduled run the server has seen.
	schedulesByOrigin map[string]map[string]struct{}
}

// maxTrackedScheduleOrigins bounds the registry. A long-lived server triggers
// many runs, and an unbounded map of dead sessions is a slow leak. The link only
// matters while its terminals are still in the rail, so evicting the oldest
// entries loses nothing an operator would look for.
const maxTrackedScheduleOrigins = 512

var scheduleOrigins = &scheduleOriginRegistry{
	originBySchedule:  map[string]string{},
	schedulesByOrigin: map[string]map[string]struct{}{},
}

// rememberScheduleOrigin records that scheduleSessionID was triggered from
// originSessionID. Both empty and self-referential links are ignored.
func rememberScheduleOrigin(scheduleSessionID, originSessionID string) {
	scheduleSessionID = strings.TrimSpace(scheduleSessionID)
	originSessionID = strings.TrimSpace(originSessionID)
	if scheduleSessionID == "" || originSessionID == "" || scheduleSessionID == originSessionID {
		return
	}
	// A scheduled session as the origin would chain runs together and surface a
	// cron run's terminals in an unrelated tab.
	if isScheduledSessionIdentity(originSessionID, "") {
		return
	}

	scheduleOrigins.mu.Lock()
	defer scheduleOrigins.mu.Unlock()
	if _, exists := scheduleOrigins.originBySchedule[scheduleSessionID]; !exists {
		scheduleOrigins.evictOldestLocked()
	}
	scheduleOrigins.originBySchedule[scheduleSessionID] = originSessionID
	if scheduleOrigins.schedulesByOrigin[originSessionID] == nil {
		scheduleOrigins.schedulesByOrigin[originSessionID] = map[string]struct{}{}
	}
	scheduleOrigins.schedulesByOrigin[originSessionID][scheduleSessionID] = struct{}{}
}

// evictOldestLocked drops an arbitrary entry once the registry is full. Map
// order gives no age, and tracking one would cost more than the imprecision:
// the cap exists to bound memory, not to guarantee which link survives.
func (r *scheduleOriginRegistry) evictOldestLocked() {
	if len(r.originBySchedule) < maxTrackedScheduleOrigins {
		return
	}
	for scheduleSession, origin := range r.originBySchedule {
		delete(r.originBySchedule, scheduleSession)
		if siblings := r.schedulesByOrigin[origin]; siblings != nil {
			delete(siblings, scheduleSession)
			if len(siblings) == 0 {
				delete(r.schedulesByOrigin, origin)
			}
		}
		return
	}
}

// sessionsTriggeredFrom returns the scheduled-run sessions started from this
// chat, including the chat itself, for use as a terminal-listing scope.
func sessionsTriggeredFrom(originSessionID string) []string {
	originSessionID = strings.TrimSpace(originSessionID)
	if originSessionID == "" {
		return nil
	}
	scheduleOrigins.mu.RLock()
	defer scheduleOrigins.mu.RUnlock()

	triggered := scheduleOrigins.schedulesByOrigin[originSessionID]
	out := make([]string, 0, len(triggered)+1)
	out = append(out, originSessionID)
	for scheduleSession := range triggered {
		out = append(out, scheduleSession)
	}
	return out
}

// chatSessionIDFromContext resolves the chat session behind a tool call. Tools
// invoked by a coding agent arrive over a separate HTTP request, so the value
// can come from either the chat key or the MCP executor's session.
func chatSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if sid, ok := ctx.Value(common.ChatSessionIDKey).(string); ok && strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid)
	}
	return strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
}
