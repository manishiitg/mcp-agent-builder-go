package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const pulseContextRecordsSchema = `CREATE TABLE IF NOT EXISTS pulse_context_records (
	context_id TEXT PRIMARY KEY,
	section TEXT NOT NULL,
	context_text TEXT NOT NULL,
	example_note TEXT NOT NULL DEFAULT '',
	path TEXT NOT NULL,
	created_at TEXT NOT NULL
)`

// PulseContextRecord is the typed, user-authoritative receipt for one durable
// context capture. The context file remains the runtime source consumed by
// steps; this table makes the author, wording, and time visible in Pulse.
type PulseContextRecord struct {
	ContextID   string `json:"context_id"`
	Section     string `json:"section"`
	ContextText string `json:"context_text"`
	ExampleNote string `json:"example_note,omitempty"`
	Path        string `json:"path"`
	CreatedAt   string `json:"created_at"`
}

func ensurePulseContextSchema(ctx context.Context, db pulseFindingLifecycleDB) error {
	_, err := db.ExecContext(ctx, pulseContextRecordsSchema)
	return err
}

// RecordPulseContextRecord is idempotent for the same normalized user rule.
// The context file write happens in the caller first; a record is only a
// receipt of that durable source, never a second competing source of truth.
func RecordPulseContextRecord(ctx context.Context, workspacePath, section, contextText, exampleNote, path string) (*PulseContextRecord, error) {
	section = strings.TrimSpace(section)
	contextText = strings.TrimSpace(contextText)
	exampleNote = strings.TrimSpace(exampleNote)
	path = strings.TrimSpace(path)
	if section == "" || contextText == "" || path == "" {
		return nil, fmt.Errorf("Pulse context record requires section, context_text, and path")
	}

	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := ensurePulseContextSchema(ctx, db); err != nil {
		return nil, err
	}

	record := &PulseContextRecord{
		ContextID:   "ctx-" + pulseImpactID(section, contextText),
		Section:     section,
		ContextText: contextText,
		ExampleNote: exampleNote,
		Path:        path,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_context_records
		(context_id, section, context_text, example_note, path, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(context_id) DO NOTHING`,
		record.ContextID, record.Section, record.ContextText, record.ExampleNote, record.Path, record.CreatedAt); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, `SELECT context_id, section, context_text, example_note, path, created_at
		FROM pulse_context_records WHERE context_id=?`, record.ContextID).Scan(
		&record.ContextID, &record.Section, &record.ContextText, &record.ExampleNote, &record.Path, &record.CreatedAt,
	); err != nil {
		return nil, err
	}
	return record, nil
}

func LoadPulseContextRecords(ctx context.Context, workspacePath string, limit int) ([]PulseContextRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return []PulseContextRecord{}, err
	}
	defer db.Close()
	if err := ensurePulseContextSchema(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT context_id, section, context_text, example_note, path, created_at
		FROM pulse_context_records ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []PulseContextRecord{}
	for rows.Next() {
		var record PulseContextRecord
		if err := rows.Scan(&record.ContextID, &record.Section, &record.ContextText, &record.ExampleNote, &record.Path, &record.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
