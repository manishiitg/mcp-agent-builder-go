package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/manishiitg/coding-agent-loop/workspace/models"
)

// TestInitializeWorkflowDBAcceptsIdempotentDropTable proves DROP TABLE IF
// EXISTS is accepted, actually drops the table, and is a safe no-op on retry.
func TestInitializeWorkflowDBAcceptsIdempotentDropTable(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	create := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"CREATE TABLE IF NOT EXISTS scratch (id TEXT PRIMARY KEY)",
	}}
	if recorder := postWorkflowDBTest(t, router, "/api/db/initialize", create); recorder.Code != http.StatusOK {
		t.Fatalf("create failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	drop := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"DROP TABLE IF EXISTS scratch",
	}}
	for attempt := 0; attempt < 2; attempt++ {
		recorder := postWorkflowDBTest(t, router, "/api/db/initialize", drop)
		if recorder.Code != http.StatusOK {
			t.Fatalf("drop attempt %d failed: status=%d body=%s", attempt+1, recorder.Code, recorder.Body.String())
		}
	}

	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='scratch'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("table still present after DROP TABLE IF EXISTS")
	}
}

// TestInitializeWorkflowDBDestructiveMigrationSnapshotsFirst proves a
// destructive migration (here, DROP COLUMN) writes a pre-migration backup
// that still has the dropped data, since there is no approval gate on this
// route to catch a mistake before it runs.
func TestInitializeWorkflowDBDestructiveMigrationSnapshotsFirst(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	create := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"CREATE TABLE IF NOT EXISTS widgets (id TEXT PRIMARY KEY, retiring_field TEXT)",
	}}
	if recorder := postWorkflowDBTest(t, router, "/api/db/initialize", create); recorder.Code != http.StatusOK {
		t.Fatalf("create failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	seedDB, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seedDB.Exec("INSERT INTO widgets(id, retiring_field) VALUES ('w1', 'irreplaceable')"); err != nil {
		seedDB.Close()
		t.Fatal(err)
	}
	seedDB.Close()

	dropColumn := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"ALTER TABLE widgets DROP COLUMN retiring_field",
	}}
	recorder := postWorkflowDBTest(t, router, "/api/db/initialize", dropColumn)
	if recorder.Code != http.StatusOK {
		t.Fatalf("drop column failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response models.APIResponse[map[string]any]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	backupPath, _ := response.Data["backup_path"].(string)
	if backupPath == "" {
		t.Fatalf("destructive migration did not report a backup_path: %s", recorder.Body.String())
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing on disk: %v", err)
	}

	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	var recovered string
	if err := backupDB.QueryRow("SELECT retiring_field FROM widgets WHERE id='w1'").Scan(&recovered); err != nil {
		t.Fatalf("dropped column not recoverable from backup: %v", err)
	}
	if recovered != "irreplaceable" {
		t.Fatalf("backup has wrong value %q", recovered)
	}

	liveDB, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	defer liveDB.Close()
	var columnCount int
	rows, err := liveDB.Query("PRAGMA table_info(widgets)")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		columnCount++
	}
	rows.Close()
	if columnCount != 1 {
		t.Fatalf("live table still has the dropped column: %d columns", columnCount)
	}
}

// TestInitializeWorkflowDBAdditiveAlterDoesNotSnapshot proves ADD COLUMN,
// which can only add data, never triggers the destructive-migration backup
// path -- keeping the common additive case cheap.
func TestInitializeWorkflowDBAdditiveAlterDoesNotSnapshot(t *testing.T) {
	rel, _, router := setupWorkflowDBTest(t)
	create := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"CREATE TABLE IF NOT EXISTS widgets (id TEXT PRIMARY KEY)",
	}}
	if recorder := postWorkflowDBTest(t, router, "/api/db/initialize", create); recorder.Code != http.StatusOK {
		t.Fatalf("create failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	addColumn := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"ALTER TABLE widgets ADD COLUMN new_field TEXT",
	}}
	recorder := postWorkflowDBTest(t, router, "/api/db/initialize", addColumn)
	if recorder.Code != http.StatusOK {
		t.Fatalf("add column failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response models.APIResponse[map[string]any]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if backupPath, ok := response.Data["backup_path"]; ok {
		t.Fatalf("additive ALTER TABLE ADD COLUMN triggered a snapshot: %v", backupPath)
	}
}

// TestInitializeWorkflowDBAcceptsRenameForms proves ALTER TABLE ... RENAME TO
// and ALTER TABLE ... RENAME COLUMN are accepted and take effect.
func TestInitializeWorkflowDBAcceptsRenameForms(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	create := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"CREATE TABLE IF NOT EXISTS old_name (id TEXT PRIMARY KEY, old_col TEXT)",
	}}
	if recorder := postWorkflowDBTest(t, router, "/api/db/initialize", create); recorder.Code != http.StatusOK {
		t.Fatalf("create failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	renameColumn := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"ALTER TABLE old_name RENAME COLUMN old_col TO new_col",
	}}
	if recorder := postWorkflowDBTest(t, router, "/api/db/initialize", renameColumn); recorder.Code != http.StatusOK {
		t.Fatalf("rename column failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	renameTable := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"ALTER TABLE old_name RENAME TO new_name",
	}}
	if recorder := postWorkflowDBTest(t, router, "/api/db/initialize", renameTable); recorder.Code != http.StatusOK {
		t.Fatalf("rename table failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var col string
	if err := db.QueryRow("SELECT new_col FROM new_name LIMIT 1").Scan(&col); err != nil && err != sql.ErrNoRows {
		t.Fatalf("renamed table/column not queryable: %v", err)
	}
}

// TestInitializeWorkflowDBStillRejectsPragmaAndAttach proves the two
// deliberately excluded statement kinds remain blocked even though DROP and
// ALTER are now allowed: PRAGMA can change database-wide behavior other
// concurrent readers/writers depend on, and ATTACH opens an arbitrary
// filesystem path outside FolderGuard's authorization.
func TestInitializeWorkflowDBStillRejectsPragmaAndAttach(t *testing.T) {
	rel, _, router := setupWorkflowDBTest(t)
	for _, migration := range []string{
		"PRAGMA journal_mode=DELETE",
		"PRAGMA foreign_keys=OFF",
		"ATTACH DATABASE '/etc/passwd' AS evil",
		"DROP TABLE widgets",               // still rejected without IF EXISTS
		"RENAME TABLE widgets TO widgets2", // not SQLite's ALTER syntax
	} {
		recorder := postWorkflowDBTest(t, router, "/api/db/initialize", models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{migration}})
		if recorder.Code == http.StatusOK {
			t.Fatalf("migration %q was accepted, want rejected", migration)
		}
	}
}

// TestBackupDatabaseBeforeDestructiveMigrationSkipsMissingFile proves the
// backup helper is never called against a database that does not exist yet
// (InitializeWorkflowDB only backs up when os.Stat succeeds), so a first-run
// destructive migration against a brand-new database is not itself an error.
func TestInitializeWorkflowDBDestructiveMigrationOnFreshDatabaseNeedsNoBackup(t *testing.T) {
	rel, _, router := setupWorkflowDBTest(t)
	// DROP TABLE IF EXISTS on a table (and database) that has never existed
	// is a valid no-op; it must not fail merely because there was nothing to
	// snapshot yet.
	drop := models.InitializeDatabaseRequest{DBPath: rel, Migrations: []string{
		"DROP TABLE IF EXISTS never_existed",
	}}
	recorder := postWorkflowDBTest(t, router, "/api/db/initialize", drop)
	if recorder.Code != http.StatusOK {
		t.Fatalf("drop-if-exists on fresh database failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
