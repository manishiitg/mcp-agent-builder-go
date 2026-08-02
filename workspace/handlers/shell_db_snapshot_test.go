package handlers

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/coding-agent-loop/workspace/models"

	_ "modernc.org/sqlite"
)

func TestCreateReadonlyDBSnapshotIncludesCommittedWALRows(t *testing.T) {
	docsDir := t.TempDir()
	dbDir := filepath.Join(docsDir, "Workflow", "demo", "db")
	stepDir := filepath.Join(docsDir, "Workflow", "demo", "runs", "iteration-0", "default", "execution", "step-read")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(dbDir, "db.sqlite")
	dsn := (&url.URL{Scheme: "file", Path: source}).String() + "?_pragma=busy_timeout(5000)"
	live, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	live.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA wal_autocheckpoint=0",
		"CREATE TABLE facts (id INTEGER PRIMARY KEY, value TEXT NOT NULL)",
		"INSERT INTO facts(value) VALUES ('committed-in-wal')",
	} {
		if _, err := live.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if _, err := os.Stat(source + "-wal"); err != nil {
		t.Fatalf("expected live WAL sidecar: %v", err)
	}

	snapshot, cleanup, err := createReadonlyDBSnapshot(
		context.Background(),
		docsDir,
		map[string]string{"DB_PATH": source, "STEP_OUTPUT_DIR": stepDir},
		&models.FolderGuardConfig{Enabled: true, WritePaths: []string{filepath.Join("Workflow", "demo", "runs", "iteration-0", "default", "execution", "step-read")}},
	)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	defer cleanup()
	if snapshot == source {
		t.Fatal("read-only step received the live database path")
	}

	copyDB, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: snapshot}).String()+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var value string
	if err := copyDB.QueryRow("SELECT value FROM facts WHERE id=1").Scan(&value); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if value != "committed-in-wal" {
		t.Fatalf("snapshot value=%q", value)
	}
	if _, err := os.Stat(snapshot + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("standalone snapshot unexpectedly requires WAL sidecar: %v", err)
	}
}
