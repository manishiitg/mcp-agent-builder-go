package videoproduct

import (
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/platformevents"
)

func TestExecutionEventsPreserveNormalizedFacts(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	user, err := store.CreateUser("activity@example.com", "Activity", []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(user, "Activity", "")
	if err != nil {
		t.Fatal(err)
	}
	want, err := store.AddExecutionEvent(platformevents.Event{
		ScopeID: project.ID, Type: platformevents.RunStarted, Name: "Scene plan",
		Status: "running", ExecutionID: "exec-scene", ParentExecutionID: "exec-workflow",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ExecutionEvents(user.ID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != want.ID || items[0].Type != platformevents.RunStarted || items[0].Name != "Scene plan" || items[0].Status != "running" || items[0].ExecutionID != "exec-scene" || items[0].ParentExecutionID != "exec-workflow" {
		t.Fatalf("execution events = %#v", items)
	}
}

func TestLegacyActivityEventsMigrateToNormalizedContract(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser("legacy-activity@example.com", "Legacy Activity", []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(user, "Legacy Activity", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TABLE activity_events (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL, type TEXT NOT NULL, name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT '', execution_id TEXT NOT NULL DEFAULT '', parent_execution_id TEXT NOT NULL DEFAULT '',
		message TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO activity_events(id,project_id,type,name,status,execution_id,parent_execution_id,message,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		"legacy-event", project.ID, "tool_call_error", "execute_shell_command", "failed", "tool-1", "", "failed", dbTime(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	items, err := store.ExecutionEvents(user.ID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Type != platformevents.ToolFailed || items[0].ExecutionID != "tool-1" {
		t.Fatalf("migrated execution events = %#v", items)
	}
}
