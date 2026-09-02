package services

import (
	"encoding/json"
	"log"
	"strings"
	"time"
)

// Provenance for outbound Gmail.
//
// Before multi-account there was nothing to record: one account sent
// everything, so "which identity sent this?" had a single constant answer. With
// several connections the sending account becomes a real variable, and a
// message that went out from the wrong one is otherwise invisible after the
// fact.
//
// Emitted as one JSON object per line behind a stable [GMAIL_AUDIT] prefix
// rather than written to a store. That keeps the send path free of a new I/O
// failure mode while staying greppable and machine-parseable. recordSend is the
// single choke point, so swapping in a real store later is one change here.
//
// Never carries message bodies, subjects, attachment contents, or any
// credential material — only routing facts.

// gmailAuditRecord is one outbound-mail event.
type gmailAuditRecord struct {
	Event string `json:"event"`
	At    string `json:"at"`

	// ConnectionID is the sending identity. Empty means the legacy singleton
	// config, i.e. an install that has not adopted connections.
	ConnectionID string `json:"connection_id,omitempty"`
	// From is the resolved sender address when it is already known from cached
	// auth. Empty rather than blocking the send to discover it.
	From string `json:"from,omitempty"`

	To           []string `json:"to,omitempty"`
	CC           []string `json:"cc,omitempty"`
	WorkflowName string   `json:"workflow,omitempty"`

	MessageID string `json:"message_id,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// recordSend logs one send attempt, successful or not.
//
// A failure is as much a provenance fact as a success: "this workflow tried to
// send from a disabled connection" is exactly the kind of thing an operator
// needs to see, and it is the failure mode multi-account introduces.
func (g *GmailService) recordSend(event, connectionID, workflowName, to string, cc []string, messageID string, sendErr error) {
	rec := gmailAuditRecord{
		Event:        event,
		At:           time.Now().UTC().Format(time.RFC3339),
		ConnectionID: strings.TrimSpace(connectionID),
		To:           splitEmailListPreservingCase(to),
		CC:           append([]string(nil), cc...),
		WorkflowName: strings.TrimSpace(workflowName),
		MessageID:    strings.TrimSpace(messageID),
		OK:           sendErr == nil,
		From:         g.cachedSenderAddress(connectionID),
	}
	if sendErr != nil {
		rec.Error = sendErr.Error()
	}
	line, err := json.Marshal(rec)
	if err != nil {
		// Never let audit formatting take down a send.
		log.Printf("[GMAIL_AUDIT] marshal failed for %s: %v", event, err)
		return
	}
	log.Printf("[GMAIL_AUDIT] %s", line)
}

// cachedSenderAddress resolves the sending address from already-cached auth.
//
// Strictly non-blocking: discovering an unknown address costs a ~5.5s
// subprocess, and provenance must never slow down or fail a send. An empty
// result means "not known yet", not "no sender".
func (g *GmailService) cachedSenderAddress(connectionID string) string {
	if id := strings.TrimSpace(connectionID); id != "" {
		if st, ok := g.AuthStatusForConnection(id); ok {
			return st.Email
		}
		return ""
	}
	return g.AuthStatusCached().Email
}
