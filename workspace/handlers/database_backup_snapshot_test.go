package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/coding-agent-loop/workspace/models"
)

func TestWorkflowDatabaseBackupSnapshotIncludesWALAndReplacesPriorImage(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE facts(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO facts(value) VALUES ('first');`); err != nil {
		t.Fatal(err)
	}

	request := models.WorkflowDatabaseBackupSnapshotRequest{DBPath: rel}
	first := postWorkflowDBTest(t, router, "/api/db/backup-snapshot", request)
	if first.Code != http.StatusOK {
		t.Fatalf("first snapshot failed: status=%d body=%s", first.Code, first.Body.String())
	}
	var response models.APIResponse[models.WorkflowDatabaseBackupSnapshotResult]
	if err := json.Unmarshal(first.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	wantRel := filepath.ToSlash(filepath.Join("Workflow", "wal-test", "backup", "database", "db.sqlite"))
	wantChecksumRel := wantRel + ".sha256"
	if response.Data.SnapshotPath != wantRel || response.Data.ChecksumPath != wantChecksumRel || response.Data.Integrity != "ok" || response.Data.SHA256 == "" || response.Data.SizeBytes == 0 {
		t.Fatalf("unexpected snapshot response: %+v", response.Data)
	}
	snapshotAbs := filepath.Join(filepath.Dir(filepath.Dir(abs)), "backup", "database", "db.sqlite")
	assertSnapshotFactCount(t, snapshotAbs, 1)
	checksum, err := os.ReadFile(snapshotAbs + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	if string(checksum) != response.Data.SHA256+"  db.sqlite\n" {
		t.Fatalf("checksum contents=%q, want hash sidecar", checksum)
	}

	if _, err := db.Exec(`INSERT INTO facts(value) VALUES ('second')`); err != nil {
		t.Fatal(err)
	}
	second := postWorkflowDBTest(t, router, "/api/db/backup-snapshot", request)
	if second.Code != http.StatusOK {
		t.Fatalf("replacement snapshot failed: status=%d body=%s", second.Code, second.Body.String())
	}
	assertSnapshotFactCount(t, snapshotAbs, 2)
	if info, err := os.Stat(snapshotAbs); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("snapshot mode=%#o, want read-only", info.Mode().Perm())
	}
}

func TestWorkflowDatabaseBackupSnapshotRejectsArbitraryPaths(t *testing.T) {
	_, _, router := setupWorkflowDBTest(t)
	for _, path := range []string{"notes.sqlite", "Workflow/wal-test/other.sqlite", "/tmp/db/db.sqlite", "../Workflow/wal-test/db/db.sqlite"} {
		recorder := postWorkflowDBTest(t, router, "/api/db/backup-snapshot", models.WorkflowDatabaseBackupSnapshotRequest{DBPath: path})
		if recorder.Code == http.StatusOK {
			t.Fatalf("unsafe db_path %q was accepted: %s", path, recorder.Body.String())
		}
	}
}

func assertSnapshotFactCount(t *testing.T, path string, want int) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM facts").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("snapshot fact count=%d, want %d", got, want)
	}
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("snapshot integrity=%q", integrity)
	}
}
