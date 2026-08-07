package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateReadSQLAllowsSafeIntegrityPragmas(t *testing.T) {
	t.Parallel()
	for _, query := range []string{
		"PRAGMA integrity_check",
		"PRAGMA main.integrity_check(100);",
		"PRAGMA integrity_check(1000)",
		"pragma quick_check",
		"PRAGMA quick_check(25)",
		"PRAGMA foreign_key_check",
		"PRAGMA foreign_key_check(accounts)",
		`PRAGMA main.foreign_key_check("account entries")`,
	} {
		query := query
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			if err := validateReadSQL(query); err != nil {
				t.Fatalf("validateReadSQL(%q): %v", query, err)
			}
		})
	}
}

func TestValidateReadSQLRejectsMutatingOrMalformedPragmas(t *testing.T) {
	t.Parallel()
	for _, query := range []string{
		"PRAGMA writable_schema=ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA integrity_check; DELETE FROM users",
		"PRAGMA integrity_check(all)",
		"PRAGMA quick_check(0)",
		"PRAGMA quick_check(1001)",
		"PRAGMA integrity_check(999999999999999999999999)",
		"PRAGMA foreign_key_check(accounts; DROP TABLE accounts)",
		"PRAGMA unknown_read_thing",
	} {
		query := query
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			if err := validateReadSQL(query); err == nil {
				t.Fatalf("validateReadSQL(%q) unexpectedly succeeded", query)
			}
		})
	}
}

func TestIntegrityPragmasExecuteThroughQueryOnlyConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflow.sqlite")
	file, err := os.Create(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	writeDB, err := openMutationDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeDB.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE parent(id INTEGER PRIMARY KEY); CREATE TABLE child(parent_id INTEGER REFERENCES parent(id))`); err != nil {
		t.Fatal(err)
	}
	_ = writeDB.Close()

	readDB, err := openQueryOnlyDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readDB.Close()
	for _, query := range []string{"PRAGMA integrity_check", "PRAGMA quick_check(10)", "PRAGMA foreign_key_check"} {
		if err := validateReadSQL(query); err != nil {
			t.Fatalf("policy rejected %q: %v", query, err)
		}
		rows, err := readDB.Query(query)
		if err != nil {
			t.Fatalf("execute %q: %v", query, err)
		}
		if _, _, _, err := scanRows(rows, 100); err != nil {
			_ = rows.Close()
			t.Fatalf("scan %q: %v", query, err)
		}
		_ = rows.Close()
	}
}
