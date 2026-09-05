package server

import (
	"context"
	"testing"
)

func TestDismissDuplicateHumanInputTool(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	ctx := context.Background()
	ws := "Workflow/duplicate-test"
	for _, id := range []string{"orphan", "keep", "linked", "different"} {
		question := "Approve the same proposal?"
		if id == "different" {
			question = "Another proposal?"
		}
		if _, err := createReportHumanInput(ctx, ws, ReportHumanInputCreateRequest{InputID: id, Source: "pulse", Question: question}); err != nil {
			t.Fatal(err)
		}
	}
	_, db, err := openReportHumanInputDB(ctx, ws, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS pulse_finding_events (metadata_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO pulse_finding_events(metadata_json) VALUES ('{"human_input_id":"linked"}')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"linked", "different", "keep"} {
		if _, err := dismissDuplicateHumanInput(ctx, ws, id, "keep", "duplicate", ""); err == nil {
			t.Fatalf("accepted unsafe dismissal %s", id)
		}
	}
	_, executors, categories := createReportHumanInputTools()
	if categories["dismiss_duplicate_human_input_request"] != "human_tools" {
		t.Fatal("missing tool category")
	}
	exec := executors["dismiss_duplicate_human_input_request"].(func(context.Context, map[string]interface{}) (string, error))
	if _, err := exec(ctx, map[string]interface{}{"workspace_path": ws, "input_id": "orphan", "keep_input_id": "keep", "reason": "exact duplicate"}); err != nil {
		t.Fatal(err)
	}
	got, err := getReportHumanInputByID(ctx, db, ws, "orphan")
	if err != nil || got.Status != "dismissed" || got.AnsweredAt != "" || got.SelectedOptionID != "" {
		t.Fatalf("fabricated answer or wrong status: %+v %v", got, err)
	}
	keep, err := getReportHumanInputByID(ctx, db, ws, "keep")
	if err != nil || keep.Status != "pending" {
		t.Fatalf("changed retained request: %+v %v", keep, err)
	}
	var events int
	if err := db.QueryRow(`SELECT count(*) FROM report_human_input_events WHERE input_id='orphan' AND event_type='duplicate_dismissed'`).Scan(&events); err != nil || events != 1 {
		t.Fatalf("audit events=%d err=%v", events, err)
	}
	if _, err := dismissDuplicateHumanInput(ctx, ws, "orphan", "keep", "duplicate", ""); err == nil {
		t.Fatal("dismissed non-pending request")
	}
}
