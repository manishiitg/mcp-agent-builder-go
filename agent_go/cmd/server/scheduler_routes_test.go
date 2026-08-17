package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// Org Pulse itself is gone (DefaultBuiltinSchedules returns empty -- see
// builtin_schedules.go), so it is no longer synthesized into a listing, or
// toggle-able, when no schedule file already carries a stale entry for it.
// These tests cover the resulting graceful-absence behavior in place of the
// old resurrection tests, mirroring builtin_schedules_test.go's approach.

func TestListScheduledJobsWithoutScheduleFileHasNoSyntheticOrgPulse(t *testing.T) {
	api := &mockWorkspaceAPI{files: map[string]string{}}
	server := httptest.NewServer(api)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs?mode=multi-agent", nil)
	rec := httptest.NewRecorder()
	listScheduledJobsHandler(NewSchedulerService(nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Jobs []ScheduledJobResponse `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v\n%s", err, rec.Body.String())
	}

	for _, job := range resp.Jobs {
		if job.ID == deprecatedAutoEnrichMemoryID {
			t.Fatalf("deprecated auto-enrich memory should not be listed: %+v", resp.Jobs)
		}
		if job.ID == builtinOrgPulseID {
			t.Fatalf("removed builtin org pulse should not be synthesized without a persisted entry: %+v", resp.Jobs)
		}
	}
}

func TestEnableBuiltinOrgPulseWithoutPersistedEntryIsNotFound(t *testing.T) {
	api := &mockWorkspaceAPI{files: map[string]string{}}
	server := httptest.NewServer(api)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs/"+builtinOrgPulseID+"/enable", nil)
	req = mux.SetURLVars(req, map[string]string{"id": builtinOrgPulseID})
	rec := httptest.NewRecorder()

	enableScheduledJobHandler(NewSchedulerService(nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s; want 404 now that org pulse has no builtin to resolve", rec.Code, rec.Body.String())
	}
}

func TestDeprecatedAutoEnrichScheduleIsFiltered(t *testing.T) {
	schedules := MergeBuiltinSchedules([]WorkflowSchedule{
		{
			ID:             deprecatedAutoEnrichMemoryID,
			Name:           "Auto-enrich memory",
			Enabled:        true,
			Mode:           "multi-agent",
			CronExpression: "0 */3 * * *",
			Query:          "Run enrich_memory.",
		},
	})
	for _, sched := range schedules {
		if sched.ID == deprecatedAutoEnrichMemoryID {
			t.Fatalf("deprecated auto-enrich memory schedule should be filtered: %+v", schedules)
		}
	}
}
