package platformevents

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Repository is shared operational storage. ScopeID is intentionally generic:
// each product owns authorization and decides whether a scope is a project,
// conversation, workspace, or another domain object.
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS execution_events (
 id TEXT PRIMARY KEY, scope_id TEXT NOT NULL,
 type TEXT NOT NULL, name TEXT NOT NULL, status TEXT NOT NULL DEFAULT '',
 execution_id TEXT NOT NULL DEFAULT '', parent_execution_id TEXT NOT NULL DEFAULT '',
 message TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_execution_events_scope ON execution_events(scope_id, created_at, id);`)
	return err
}

func (r *Repository) Add(event Event) (Event, error) {
	if strings.TrimSpace(event.ScopeID) == "" || strings.TrimSpace(string(event.Type)) == "" || strings.TrimSpace(event.Name) == "" {
		return Event{}, errors.New("execution event scope, type, and name are required")
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.Exec(`INSERT INTO execution_events(id,scope_id,type,name,status,execution_id,parent_execution_id,message,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		event.ID, event.ScopeID, event.Type, event.Name, event.Status, event.ExecutionID, event.ParentExecutionID, event.Message, event.CreatedAt.UTC().Format(time.RFC3339Nano))
	return event, err
}

func (r *Repository) List(scopeID string) ([]Event, error) {
	rows, err := r.db.Query(`SELECT id,scope_id,type,name,status,execution_id,parent_execution_id,message,created_at FROM execution_events WHERE scope_id=? ORDER BY created_at,id`, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var item Event
		var eventType, created string
		if err := rows.Scan(&item.ID, &item.ScopeID, &eventType, &item.Name, &item.Status, &item.ExecutionID, &item.ParentExecutionID, &item.Message, &created); err != nil {
			return nil, err
		}
		item.Type = Type(eventType)
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
