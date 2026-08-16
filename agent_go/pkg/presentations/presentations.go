// Package presentations owns the ui_presentations contract: the one
// mechanism by which any product tool shows something durable in its
// product's UI (a video, and in future a report, a document, or any other
// kind a product declares).
//
// It exists because the previous version of this — inline in
// showVideoFactory — hand-rolled its own SQL upsert and hand-wrote an Emit
// map literal once, in one place. A second tool doing the same thing would
// have copied both by hand, and the two would have drifted the moment one
// changed and the other didn't. That is the exact shape of every drift bug
// this codebase hit today: one decision (what a presentation row looks like,
// what event announces it) encoded independently at more than one site.
//
// A tool factory that presents something should do three things and no
// more: validate its own domain rules (a video needs a passing QA report;
// a future kind will have its own rules), build a Presentation, and call
// Upsert. Everything about how that becomes a database row and a
// frontend-visible event lives here, once.
package presentations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	orchestratorevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	mcpagentevents "github.com/manishiitg/mcpagent/events"
)

var _ mcpagentevents.EventData = (*Event)(nil)

// Presentation is what a tool factory has decided to show. Kind should come
// from ToolRuntimeContext.Presentation.Kind (the profile's declaration), not
// be hardcoded by the factory — see agentprofiles.PresentationBinding.
type Presentation struct {
	// Kind identifies which frontend renderer displays this (e.g. "media.video").
	Kind string
	// IdentityKey determines the presentation's row identity: the same key
	// upserts in place (revision increments) rather than creating a new row.
	// For a file-backed presentation this is normally the project-relative
	// path, so re-showing the same file updates it instead of duplicating it.
	IdentityKey   string
	Title         string
	WorkspacePath string
	SessionID     string
	// Payload, Resources, Actions are product-owned shapes specific to Kind.
	// This package does not interpret them; it stores and forwards them
	// verbatim, the same way the frontend's kind-keyed renderer registry
	// does not need to know a video's payload shape to dispatch to it.
	Payload   map[string]interface{}
	Resources []map[string]string
	Actions   []map[string]interface{}
}

// Event is a type alias, not a wrapper: Upsert returns the real
// orchestrator_events.PresentationUpdatedEvent, registered in cmd/schema-gen
// (EventDataUnion, EventRegistry, and UnifiedEvent) so it gets a generated
// TypeScript interface the same way tool_call_end and every other typed
// event does.
//
// That registration is what earlier iterations of this package skipped: they
// returned a map[string]interface{}, which server.emitAgentProfileEvent could
// only wrap in unifiedevents.GenericEventData -- an untyped escape hatch that
// (a) gives the frontend no generated type to import, so consumers hand-roll
// their own guess at the shape, and (b) serializes one JSON level deeper than
// a typed event, because GenericEventData has its own "data" field wrapping
// an arbitrary map. Both were confirmed the hard way: a first version of the
// frontend listener assumed a shallower shape than what a live emission
// actually produced.
//
// It deliberately carries no revision number. The upsert SQL below
// increments revision in the database, but the workspace mutation API does
// not return the resulting row, so any value put here would be a guess
// dressed as a fact. A consumer that needs the true revision reads it from
// ui_presentations, the same place every other consumer already does.
type Event = orchestratorevents.PresentationUpdatedEvent

// IdentityFromKey derives a stable presentation id from an identity key. Two
// calls with the same key upsert the same row; this is what makes re-showing
// the same video update it in place instead of accumulating duplicates.
func IdentityFromKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return "presentation-" + hex.EncodeToString(hash[:8])
}

// Upsert writes one presentation row to <WorkspacePath>/db/db.sqlite and
// returns the Event to emit. On a repeat call with the same IdentityKey, the
// row updates in place and Revision increments; it does not create a
// duplicate.
//
// Kind, IdentityKey, Title, and WorkspacePath are required; Upsert returns an
// error rather than silently writing a malformed row, since a malformed row
// is exactly the kind of failure that surfaces as "nothing on the panel"
// with no diagnosable cause.
func Upsert(ctx context.Context, client *workspace.Client, p Presentation) (Event, error) {
	kind := strings.TrimSpace(p.Kind)
	identityKey := strings.TrimSpace(p.IdentityKey)
	title := strings.TrimSpace(p.Title)
	workspacePath := strings.TrimSpace(p.WorkspacePath)
	if kind == "" {
		return Event{}, fmt.Errorf("presentation kind is required")
	}
	if identityKey == "" {
		return Event{}, fmt.Errorf("presentation identity key is required")
	}
	if title == "" {
		return Event{}, fmt.Errorf("presentation title is required")
	}
	if workspacePath == "" {
		return Event{}, fmt.Errorf("presentation workspace path is required")
	}

	presentationID := IdentityFromKey(kind + ":" + identityKey)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	payload := p.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal presentation payload: %w", err)
	}
	resources := p.Resources
	if resources == nil {
		resources = []map[string]string{}
	}
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return Event{}, fmt.Errorf("marshal presentation resources: %w", err)
	}
	actions := p.Actions
	if actions == nil {
		actions = []map[string]interface{}{}
	}
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return Event{}, fmt.Errorf("marshal presentation actions: %w", err)
	}

	dbPath := filepath.ToSlash(filepath.Join(workspacePath, "db/db.sqlite"))
	_, err = client.MutateAuthorizedWorkflowDB(ctx, workspace.MutateWorkflowDBParams{
		DBPath: dbPath,
		Statements: []workspace.WorkflowDBMutationStatement{{
			SQL: `INSERT INTO ui_presentations
				(id, kind, schema_version, session_id, title, payload_json, resources_json, actions_json, status, revision, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					session_id = excluded.session_id,
					title = excluded.title,
					payload_json = excluded.payload_json,
					resources_json = excluded.resources_json,
					actions_json = excluded.actions_json,
					status = excluded.status,
					revision = ui_presentations.revision + 1,
					updated_at = excluded.updated_at`,
			Params: []interface{}{
				presentationID, kind, 1, p.SessionID, title,
				string(payloadJSON), string(resourcesJSON), string(actionsJSON),
				"ready", 1, now, now,
			},
		}},
	})
	if err != nil {
		return Event{}, fmt.Errorf("persist presentation: %w", err)
	}

	return Event{
		BaseEventData:  mcpagentevents.BaseEventData{Timestamp: time.Now()},
		PresentationID: presentationID,
		Kind:           kind,
		Title:          title,
		WorkspacePath:  workspacePath,
		Payload:        payload,
	}, nil
}
