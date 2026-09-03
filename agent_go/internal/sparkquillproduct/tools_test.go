package sparkquillproduct

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	orchestratorevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// fakeWorkspace is the slice of the workspace API the family tools use:
// GET and PUT /api/documents/<path>.
type fakeWorkspace struct {
	mu    sync.Mutex
	files map[string]string
}

func (f *fakeWorkspace) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/documents/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api/documents/")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			content, ok := f.files[p]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "not found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"filepath": p, "content": content}})
		case http.MethodPut:
			var body struct {
				Content string `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.files[p] = body.Content
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

type eventSink struct {
	mu     sync.Mutex
	events []*orchestratorevents.ProductInteractionEvent
}

func (s *eventSink) emit(ev any) {
	if e, ok := ev.(*orchestratorevents.ProductInteractionEvent); ok {
		s.mu.Lock()
		s.events = append(s.events, e)
		s.mu.Unlock()
	}
}

func (s *eventSink) kinds() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, e := range s.events {
		out = append(out, e.Kind)
	}
	return out
}

func newToolHarness(t *testing.T) (*fakeWorkspace, *eventSink, agentprofiles.ToolRuntimeContext, string) {
	t.Helper()
	fake := &fakeWorkspace{files: map[string]string{}}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	sink := &eventSink{}
	rt := agentprofiles.ToolRuntimeContext{UserID: "u1", SessionID: "s1", WorkspacePath: "Chats/SparkQuill", Emit: sink.emit}
	return fake, sink, rt, srv.URL
}

func build(t *testing.T, factory agentprofiles.ToolFactory, rt agentprofiles.ToolRuntimeContext) agentprofiles.ToolSpec {
	t.Helper()
	spec, err := factory(rt, nil)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestFamilyStateToolsWriteFamilyJSONAndMirrors(t *testing.T) {
	fake, sink, rt, url := newToolHarness(t)
	ctx := context.Background()

	profile := build(t, setChildProfileFactory(url), rt)
	if _, err := profile.Execute(ctx, map[string]interface{}{"name": "Maya", "grade": "6"}); err != nil {
		t.Fatal(err)
	}
	if _, err := profile.Execute(ctx, map[string]interface{}{"board": "CBSE"}); err != nil {
		t.Fatal(err)
	}
	label := build(t, setParentLabelFactory(url), rt)
	if _, err := label.Execute(ctx, map[string]interface{}{"label": "mom"}); err != nil {
		t.Fatal(err)
	}
	var state FamilyState
	if err := json.Unmarshal([]byte(fake.files["_users/u1/Chats/SparkQuill/family.json"]), &state); err != nil {
		t.Fatal(err)
	}
	if state.Child == nil || state.Child.Name != "Maya" || state.Child.Grade != "6" || state.Child.Board != "CBSE" || state.ParentLabel != "mom" {
		t.Fatalf("family.json = %+v", state)
	}
	if !strings.Contains(fake.files["_users/u1/Chats/SparkQuill/memory/child-profile.json"], `"Maya"`) {
		t.Fatalf("memory mirrors missing: %v", fake.files)
	}
	if k := sink.kinds(); len(k) != 3 || k[0] != "family_updated" {
		t.Fatalf("events = %v", k)
	}
	if vars := ParentPromptVariables(state); vars["CHILD_WHO"] != "Maya, Grade 6 (CBSE)" || vars["CHILD_INFO_NUDGE"] != "" {
		t.Fatalf("prompt variables should now see the saved family: %+v", vars)
	}
}

func TestCreateLearningActivityWritesManifestAndProject(t *testing.T) {
	fake, sink, rt, url := newToolHarness(t)
	ctx := context.Background()
	create := build(t, createLearningActivityFactory(url), rt)

	if _, err := create.Execute(ctx, map[string]interface{}{"dir": "Math/Fractions/x", "title": "T"}); err == nil {
		t.Fatal("nested subject/topic paths must be refused now that activities are flat")
	}
	if _, err := create.Execute(ctx, map[string]interface{}{"dir": "activities/2026-09-03-fractions", "title": "T", "items": []interface{}{"quick-check.html"}}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing item must be refused: %v", err)
	}
	fake.files["_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions/quick-check.html"] = "<html>"
	fake.files["_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions/quick-check-KEY.md"] = "answers"
	out, err := create.Execute(ctx, map[string]interface{}{"dir": "_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions", "title": "Fractions — Quick Check", "subject": "Math", "topic": "Fractions",
		"items": []interface{}{"quick-check.html", "quick-check-KEY.md"}, "goal": "answer all ten on her own", "persona": "playful coach"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"items":1`) {
		t.Fatalf("answer key must never be an item: %s", out)
	}
	var manifest ActivityManifest
	if err := json.Unmarshal([]byte(fake.files["_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions/activity.json"]), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Subject != "Math" || manifest.Goal == "" || len(manifest.Items) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	var project activityProject
	if err := json.Unmarshal([]byte(fake.files["_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions/product.json"]), &project); err != nil {
		t.Fatal(err)
	}
	if project.Product != ChildProfileID || project.ID != "2026-09-03-fractions" || !strings.HasPrefix(project.SessionID, "product-") {
		t.Fatalf("product.json must bind the folder to the child profile: %+v", project)
	}
	// Re-finalizing keeps the child's conversation.
	if _, err := create.Execute(ctx, map[string]interface{}{"dir": "activities/2026-09-03-fractions", "title": "Fractions — Quick Check v2", "goal": "same"}); err != nil {
		t.Fatal(err)
	}
	var again activityProject
	_ = json.Unmarshal([]byte(fake.files["_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions/product.json"]), &again)
	if again.SessionID != project.SessionID || again.Title != "Fractions — Quick Check v2" {
		t.Fatalf("session id must survive re-finalizing: %+v vs %+v", again, project)
	}
	if k := sink.kinds(); len(k) != 2 || k[0] != "activity_created" {
		t.Fatalf("events = %v", k)
	}
}

func TestTransientToolsEmitProductInteractions(t *testing.T) {
	_, sink, rt, _ := newToolHarness(t)
	ctx := context.Background()
	suggest := build(t, suggestActionsFactory(), rt)
	if _, err := suggest.Execute(ctx, map[string]interface{}{"actions": []interface{}{map[string]interface{}{"label": "How's she doing?", "message": "progress"}, map[string]interface{}{"label": "", "message": "x"}}}); err != nil {
		t.Fatal(err)
	}
	celebrate := build(t, celebrateFactory(), rt)
	if out, err := celebrate.Execute(ctx, map[string]interface{}{"stars": float64(7), "reason": "finished!"}); err != nil || !strings.Contains(out, `"stars_awarded":3`) {
		t.Fatalf("stars must clamp to 3: %s %v", out, err)
	}
	scene := build(t, showSceneFactory(), rt)
	if _, err := scene.Execute(ctx, map[string]interface{}{"html": "<b>hi</b>"}); err != nil {
		t.Fatal(err)
	}
	if k := strings.Join(sink.kinds(), ","); k != "suggestions,celebrate,scene" {
		t.Fatalf("events = %s", k)
	}
	if actions, _ := sink.events[0].Payload["actions"].([]map[string]interface{}); len(actions) != 1 {
		t.Fatalf("blank suggestions must be dropped: %+v", sink.events[0].Payload)
	}
	for _, e := range sink.events {
		if e.Product != ProductName || e.GetEventType() != orchestratorevents.ProductInteraction {
			t.Fatalf("event mis-tagged: %+v", e)
		}
	}
}

func TestOpenToolsRefuseMissingTargetsAndUndeclaredPresentation(t *testing.T) {
	fake, _, rt, url := newToolHarness(t)
	ctx := context.Background()
	undeclared := build(t, openFileFactory(url, false), rt)
	if _, err := undeclared.Execute(ctx, map[string]interface{}{"path": "reports/progress.html"}); err == nil || !strings.Contains(err.Error(), "presentation kind") {
		t.Fatalf("open_file without a presentation binding must fail closed: %v", err)
	}
	rt.Presentation = &agentprofiles.PresentationBinding{Kind: "document.file"}
	open := build(t, openFileFactory(url, false), rt)
	if _, err := open.Execute(ctx, map[string]interface{}{"path": "reports/progress.html"}); err == nil || !strings.Contains(err.Error(), "no file") {
		t.Fatalf("missing file must be refused before any presentation is written: %v", err)
	}
	fake.files["_users/u1/Chats/SparkQuill/activities/a1/activity.json"] = "{not json"
	rt.Presentation = &agentprofiles.PresentationBinding{Kind: "sparkquill.activity"}
	openActivity := build(t, openActivityFactory(url), rt)
	if _, err := openActivity.Execute(ctx, map[string]interface{}{"dir": "activities/a1"}); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("corrupt manifest must be reported: %v", err)
	}
	if _, err := openActivity.Execute(ctx, map[string]interface{}{"dir": "activities/nope"}); err == nil || !strings.Contains(err.Error(), "no activity") {
		t.Fatalf("missing activity must be reported: %v", err)
	}
}

func TestInboxNoteNamesTheUnfiledUploads(t *testing.T) {
	if got := InboxNote(nil); got != "" {
		t.Fatalf("empty inbox produced a note: %q", got)
	}
	got := InboxNote([]string{"_users/u1/Chats/SparkQuill/inbox/worksheet.pdf", "_users/u1/Chats/SparkQuill/inbox/photo.jpg"})
	for _, want := range []string{"2 file(s)", "worksheet.pdf", "photo.jpg", "process-file"} {
		if !strings.Contains(got, want) {
			t.Fatalf("inbox note %q lacks %q", got, want)
		}
	}
}

func TestParseFolderListingAcceptsTheDocumentsAPIShapes(t *testing.T) {
	cases := map[string]string{
		"bare array":     `[{"filepath":"inbox/a.pdf","type":"file"}]`,
		"data wrapper":   `{"success":true,"data":[{"filepath":"inbox/a.pdf","type":"file"}]}`,
		"single folder":  `{"filepath":"inbox","type":"folder","children":[{"filepath":"inbox/a.pdf","type":"file"}]}`,
		"wrapped folder": `{"data":{"filepath":"inbox","type":"folder","children":[{"filepath":"inbox/a.pdf","type":"file"}]}}`,
	}
	for name, raw := range cases {
		got := parseFolderListing([]byte(raw))
		if len(got) != 1 || got[0].FilePath != "inbox/a.pdf" {
			t.Fatalf("%s: parsed %+v", name, got)
		}
	}
}
