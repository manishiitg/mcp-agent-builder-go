package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type activeTurnSender interface {
	Send(context.Context, string) error
}

// activeTurns maps conversation id -> the agent driving that conversation's
// in-flight turn, so a concurrent request for the SAME conversation can inject
// a message into it live (steer) instead of waiting for it to finish and
// starting a wholly separate turn afterward.
//
// Keyed rather than a single slot. The old version held one global entry on
// the assumption that agentTurnMu meant only one turn could exist at a time.
// That assumption was already load-bearing in a way it should not have been:
// clearActiveTurn() erased whatever was registered, so with two turns the
// first to finish would deregister the second, silently disabling steering for
// a turn still running. Keying by conversation removes the assumption and is a
// precondition for running parent and child turns concurrently at all.
//
// Deliberately NOT mcpagent's own Agent.TurnInFlight(): that flag is only set
// by mcpagent's ContinueConversation, which agentsession.Session.Ask never
// calls (it goes straight to AskWithHistory) — so it would always read false
// on this call path. This registry is the actual source of truth.
var (
	activeTurnMu sync.Mutex
	activeTurns  = map[string]activeTurnSender{}
)

// registerActiveTurn records the agent driving a turn that's about to start,
// so a concurrent steer attempt for the same conversation id can find it.
// Call right before the blocking Ask/turn call; pair with a deferred
// clearActiveTurn.
func registerActiveTurn(conversationID string, sender activeTurnSender) {
	activeTurnMu.Lock()
	activeTurns[conversationID] = sender
	activeTurnMu.Unlock()
}

// clearActiveTurn removes THIS conversation's registration once its turn
// completes (success or error) — always via `defer`, right after
// registerActiveTurn. Takes the id so a finishing turn cannot deregister
// somebody else's.
func clearActiveTurn(conversationID string) {
	activeTurnMu.Lock()
	delete(activeTurns, conversationID)
	activeTurnMu.Unlock()
}

// trySteer attempts to inject message into the turn currently in flight for
// conversationID — actually redirecting what Quill is doing mid-turn, rather
// than waiting for it to finish. Returns true if delivered (the caller should
// return an immediate ack; the message will be reflected in whichever turn's
// blocking call is already running). Returns false if the caller should fall
// back to its normal behavior: no turn is in flight, it's a different
// conversation, the provider doesn't support live input (steering is only
// possible for the persistent-tmux CLI providers), or delivery itself
// failed/timed out.
//
// Bounded by its own short timeout, independent of the turn's own much longer
// turnTimeout: DeliverUserMessage blocks (doesn't error) if it lands while a
// session is still cold-starting its underlying tmux CLI, so a short deadline
// here lets a mistimed steer degrade quickly to "fall through" instead of
// tying up the request for a long time. No dedicated mutex is needed around
// the delivery call itself — multi-llm-provider-go's tmuxinput broker already
// serializes input delivery per tmux session with one worker goroutine, so
// rapid successive steers queue and run in submission order safely.
func trySteer(ctx context.Context, conversationID, message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	if experimentSteeringDisabled() {
		log.Printf("[steer] %q: refused — steering disabled via FAMILY_DISABLE_STEER", conversationID)
		return false
	}
	activeTurnMu.Lock()
	sender, ok := activeTurns[conversationID]
	activeTurnMu.Unlock()
	// Log every refusal with the reason. "Steering didn't work" is otherwise
	// indistinguishable between two very different causes — no turn was in
	// flight for THIS conversation, or the delivery itself failed inside the
	// provider — and they need different fixes. Without this the caller only
	// ever sees {"steered":false}.
	//
	// The lookup is now by conversation, so a turn running for a different
	// conversation is simply not found rather than being found and rejected.
	if !ok {
		log.Printf("[steer] %q: no turn in flight", conversationID)
		return false
	}
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := sender.Send(sctx, message)
	if err != nil {
		log.Printf("[steer] %q: delivery failed: %v", conversationID, err)
		return false
	}
	log.Printf("[steer] %q: delivered into the live turn", conversationID)
	return true
}

// handleParentSteer serves POST /api/parent/steer — the browser's fast-path
// attempt to redirect the parent conversation's turn while it's still
// running, instead of only ever queuing a follow-up for after. Deliberately a
// narrow, separate endpoint from /api/parent/message: it ONLY ever tries to
// steer and returns quickly, it never falls through to starting its own
// blocking turn. If steering isn't possible right now, the frontend's
// existing client-side queue (unchanged) is the fallback — it sends the
// message as an ordinary new turn once the current one finishes.
func handleParentSteer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]bool{"steered": false})
		return
	}
	convID := strings.TrimSpace(req.ConversationID)
	message := strings.TrimSpace(req.Message)
	if convID == "" || message == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"steered": false})
		return
	}
	ok := trySteer(r.Context(), convID, message)
	if ok {
		// Durably record the message right away — it's now part of the live
		// turn's context, but that turn's own eventual completion won't know
		// about it unless it reloads the freshest history first (see
		// persistConversationReply in chat.go), which this write makes possible.
		appendUserMessageToConversation("parent", convID, message)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"steered": ok})
}

// handleChildSteer serves POST /api/child/steer — same idea as
// handleParentSteer, for the child's own turn (see registerActiveTurn in
// child.go). conversation_id here is the activity dir, the child's natural
// per-activity conversation id.
func handleChildSteer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]bool{"steered": false})
		return
	}
	convID := strings.TrimSpace(req.ConversationID)
	message := strings.TrimSpace(req.Message)
	if convID == "" || message == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"steered": false})
		return
	}
	ok := trySteer(r.Context(), convID, message)
	if ok {
		appendUserMessageToConversation("child", convID, message)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"steered": ok})
}
