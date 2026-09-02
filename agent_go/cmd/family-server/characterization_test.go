package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
)

// Characterization tests for the turns families actually run. Each drives a
// real handler end to end with the coding-agent session replaced by a fake
// (turn_session.go), so what is pinned down is everything AROUND the model:
// which prompt and tools a persona gets, where it is allowed to work, what
// is persisted, and what streams back. These are the precondition for any
// restructuring of family-server (docs/design/reusable_vertical_product_platform.md).

// fakeSession records the configuration a turn was built with and answers
// with a canned reply, optionally emitting stream deltas first.
type fakeSession struct {
	cfg    agentsession.Config
	reply  string
	deltas []string
	asked  []agentsession.Message
}

func (f *fakeSession) Ask(_ context.Context, history []agentsession.Message) (string, error) {
	f.asked = history
	for _, d := range f.deltas {
		if f.cfg.StreamCallback != nil {
			f.cfg.StreamCallback(d)
		}
	}
	return f.reply, nil
}
func (f *fakeSession) Send(context.Context, string) error { return nil }
func (f *fakeSession) Resumed() bool                      { return false }
func (f *fakeSession) Handle() *agentsession.Handle       { return nil }
func (f *fakeSession) Close()                             {}

// installFakeSession swaps the session factory for the test and returns the
// captured sessions in creation order.
func installFakeSession(t *testing.T, reply string, deltas ...string) func() []*fakeSession {
	t.Helper()
	var mu sync.Mutex
	var made []*fakeSession
	prev := newAgentSession
	newAgentSession = func(_ context.Context, cfg agentsession.Config) (turnSession, error) {
		f := &fakeSession{cfg: cfg, reply: reply, deltas: deltas}
		mu.Lock()
		made = append(made, f)
		mu.Unlock()
		return f, nil
	}
	t.Cleanup(func() { newAgentSession = prev })
	return func() []*fakeSession { mu.Lock(); defer mu.Unlock(); return append([]*fakeSession(nil), made...) }
}

// setupFamily points the server at a fresh data dir with an engine chosen
// and a child profile created — the state after onboarding.
func setupFamily(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FAMILY_DATA_DIR", dir)
	if err := scaffoldFamilyFolders(); err != nil {
		t.Fatal(err)
	}
	stateMu.Lock()
	s := loadState()
	s.Engine = "claude-code"
	s.Child = &Child{Name: "Maya", Grade: "6", Board: "CBSE"}
	err := saveState(s)
	stateMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func postJSON(t *testing.T, h http.HandlerFunc, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func toolNames(tools []agentsession.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tl := range tools {
		out = append(out, tl.Name)
	}
	return out
}

func hasTool(tools []agentsession.Tool, name string) bool {
	for _, tl := range tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

func TestParentTurnPromptToolsAndPersistence(t *testing.T) {
	dir := setupFamily(t)
	made := installFakeSession(t, "Here is a quick plan for Maya.")

	rec, out := postJSON(t, handleParentMessage, `{"conversation_id":"conv-1","messages":[{"role":"user","text":"plan her week"}]}`)
	if rec.Code != http.StatusOK || out["reply"] != "Here is a quick plan for Maya." {
		t.Fatalf("reply: %d %s", rec.Code, rec.Body.String())
	}
	sessions := made()
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	cfg := sessions[0].cfg
	if cfg.WorkingDir != filepath.Join(dir, "workspace") {
		t.Fatalf("parent works in the workspace root, got %q", cfg.WorkingDir)
	}
	if cfg.SessionID != "conv-1" {
		t.Fatalf("session id should be the conversation id, got %q", cfg.SessionID)
	}
	if !strings.Contains(cfg.SystemPrompt, "Maya") {
		t.Fatal("parent prompt must name the child")
	}
	for _, name := range []string{"open_activity", "set_child_profile", "suggest_actions"} {
		if !hasTool(cfg.Tools, name) {
			t.Fatalf("parent tools missing %s: %v", name, toolNames(cfg.Tools))
		}
	}
	if len(cfg.Skills) == 0 {
		t.Fatal("parent session should carry the embedded skills")
	}
	if len(sessions[0].asked) != 1 || sessions[0].asked[0].Text != "plan her week" {
		t.Fatalf("history handed to the model: %+v", sessions[0].asked)
	}
	// Persisted: the parent transcript holds the user turn and the reply.
	stored, ok := loadStoredConversation("parent", "conv-1")
	if !ok || len(stored.Messages) < 2 {
		t.Fatalf("conversation not persisted: ok=%v messages=%+v", ok, stored.Messages)
	}
	last := stored.Messages[len(stored.Messages)-1]
	if last.Role != "assistant" || last.Text != "Here is a quick plan for Maya." {
		t.Fatalf("last persisted message: %+v", last)
	}
}

func TestParentTurnRequiresEngine(t *testing.T) {
	setupFamily(t)
	stateMu.Lock()
	s := loadState()
	s.Engine = ""
	_ = saveState(s)
	stateMu.Unlock()
	rec, out := postJSON(t, handleParentMessage, `{"messages":[{"role":"user","text":"hi"}]}`)
	if rec.Code != http.StatusBadRequest || out["error"] != "no learning engine is selected" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func writeActivity(t *testing.T, dir, subject, topic, slug, title string) string {
	t.Helper()
	rel := filepath.Join(subject, topic, slug)
	abs := filepath.Join(dir, "workspace", rel)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(activityManifest{Title: title, Subject: subject, Topic: topic, Items: []string{"sheet.html"}})
	if err := os.WriteFile(filepath.Join(abs, activityManifestName), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(abs, "sheet.html"), []byte("<p>q1</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

func TestChildTurnRefusedUntilHandoff(t *testing.T) {
	setupFamily(t)
	installFakeSession(t, "never asked")
	rec, out := postJSON(t, handleChildMessage, `{"messages":[{"role":"user","text":"hi"}]}`)
	if rec.Code != http.StatusBadRequest || out["error"] != "no activity has been handed off yet" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandoffThenChildTurnIsScopedToTheActivity(t *testing.T) {
	dir := setupFamily(t)
	act := writeActivity(t, dir, "Maths", "Fractions", "quick-check-1", "Fractions quick check")
	other := writeActivity(t, dir, "Science", "Plants", "reading-1", "Plants reading")

	// Parent hands the activity over; a different activity means a new session.
	rec, out := postJSON(t, handleHandoff, `{"dir":"`+act+`"}`)
	if rec.Code != http.StatusOK || out["dir"] != act || out["new_session"] != true || out["title"] != "Fractions quick check" {
		t.Fatalf("handoff: %d %s", rec.Code, rec.Body.String())
	}
	if currentActivityDir() != act {
		t.Fatalf("current activity pointer = %q", currentActivityDir())
	}
	// Handing the same activity over again resumes rather than restarts.
	_, again := postJSON(t, handleHandoff, `{"dir":"`+act+`"}`)
	if again["new_session"] != false {
		t.Fatalf("same activity should resume: %v", again)
	}

	made := installFakeSession(t, "Let's look at question 1.")
	rec, out = postJSON(t, handleChildMessage, `{"messages":[{"role":"user","text":"I'm stuck"}]}`)
	if rec.Code != http.StatusOK || out["reply"] != "Let's look at question 1." {
		t.Fatalf("child reply: %d %s", rec.Code, rec.Body.String())
	}
	cfg := made()[0].cfg
	if cfg.WorkingDir != filepath.Join(dir, "workspace", filepath.FromSlash(act)) {
		t.Fatalf("child must work inside the handed-off activity only, got %q", cfg.WorkingDir)
	}
	if !strings.Contains(cfg.SystemPrompt, act) {
		t.Fatal("child prompt should name its activity folder")
	}
	if strings.Contains(cfg.SystemPrompt, other) {
		t.Fatal("child prompt must not mention another activity")
	}
	if !hasTool(cfg.Tools, "celebrate") || !hasTool(cfg.Tools, "open_file") {
		t.Fatalf("child tools: %v", toolNames(cfg.Tools))
	}
	for _, parentOnly := range []string{"open_activity", "set_child_profile", "set_child_schedule", "set_parent_label", "suggest_actions"} {
		if hasTool(cfg.Tools, parentOnly) {
			t.Fatalf("child must not get the parent tool %s: %v", parentOnly, toolNames(cfg.Tools))
		}
	}
	// The child's transcript lives inside the activity, not in the parent's.
	if _, ok := loadStoredConversation("child", act); !ok {
		t.Fatal("child conversation not persisted inside the activity")
	}
	if _, ok := loadStoredConversation("parent", "parent"); ok {
		t.Fatal("child turn must not write the parent conversation")
	}
}

// The child's shell is deny-by-default and scoped to the current activity.
// Exercised through the real isolator when the host can sandbox (sandbox-exec
// on macOS, Landlock on Linux); otherwise only the policy is checked.
func TestChildShellCannotLeaveTheActivity(t *testing.T) {
	// Not t.TempDir(): on macOS that lives under /var, which the strict
	// sandbox profile deliberately leaves readable for system state, so a
	// workspace there would make every read succeed and prove nothing. Real
	// installs live under $HOME (~/.sunlit-learning), so the test does too.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	dir, err := os.MkdirTemp(home, ".sparkquill-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("FAMILY_DATA_DIR", dir)
	if err := scaffoldFamilyFolders(); err != nil {
		t.Fatal(err)
	}
	act := writeActivity(t, dir, "Maths", "Fractions", "qc", "QC")
	other := writeActivity(t, dir, "Science", "Plants", "rd", "RD")
	saveCurrentActivity(act)
	_ = os.WriteFile(filepath.Join(dir, "workspace", "memory", "interests.md"), []byte("secret"), 0o644)

	// The shell runs from the workspace root with only the activity folder
	// readable and writable, so paths are workspace-relative.
	tool := childShellTool()
	inside, err := tool.Handler(context.Background(), map[string]interface{}{"command": "cat " + act + "/sheet.html"})
	if err != nil && strings.Contains(err.Error(), "SANDBOX_UNAVAILABLE") {
		t.Skip("no sandbox on this host")
	}
	if err != nil || !strings.Contains(inside, "q1") {
		t.Fatalf("reading inside the activity should work: out=%q err=%v", inside, err)
	}
	for _, cmd := range []string{"cat " + other + "/sheet.html", "cat memory/interests.md", "cat " + filepath.Join(dir, "workspace", "memory", "interests.md")} {
		out, err := tool.Handler(context.Background(), map[string]interface{}{"command": cmd})
		if err == nil && (strings.Contains(out, "q1") || strings.Contains(out, "secret")) {
			t.Fatalf("child shell escaped its activity with %q: %q", cmd, out)
		}
	}
}

func TestParentTurnStreamsDeltasToStatusSubscribers(t *testing.T) {
	setupFamily(t)
	installFakeSession(t, "done", "wor", "king")
	ch, unsubscribe := statusHubs.subscribe("parent:conv-s")
	defer unsubscribe()
	rec, _ := postJSON(t, handleParentMessage, `{"conversation_id":"conv-s","messages":[{"role":"user","text":"go"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("turn failed: %s", rec.Body.String())
	}
	// Status labels may interleave with deltas; collect deltas until the
	// reply has streamed through, bounded so a broken stream fails fast.
	var got []string
	deadline := time.After(3 * time.Second)
	for strings.Join(got, "") != "working" {
		select {
		case ev := <-ch:
			if ev.Type == "delta" {
				got = append(got, ev.Text)
			}
		case <-deadline:
			t.Fatalf("streamed deltas = %q, want \"working\"", got)
		}
	}
}

func TestPinGate(t *testing.T) {
	setupFamily(t)
	if rec, out := postJSON(t, handleSetPin, `{"pin":"123"}`); rec.Code != http.StatusBadRequest || out["error"] == nil {
		t.Fatal("a 3-digit PIN must be rejected")
	}
	// No PIN set: the gate is open (documented behaviour of a fresh install).
	if _, out := postJSON(t, handleVerifyPin, `{"pin":"0000"}`); out["ok"] != true {
		t.Fatal("without a PIN the gate should be open")
	}
	if rec, _ := postJSON(t, handleSetPin, `{"pin":"2468"}`); rec.Code != http.StatusOK {
		t.Fatalf("set pin: %d", rec.Code)
	}
	if _, out := postJSON(t, handleVerifyPin, `{"pin":"1111"}`); out["ok"] != false {
		t.Fatal("wrong PIN accepted")
	}
	if _, out := postJSON(t, handleVerifyPin, `{"pin":"2468"}`); out["ok"] != true {
		t.Fatal("right PIN rejected")
	}
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if strings.Contains(s.PinHash, "2468") || len(s.PinHash) != 64 {
		t.Fatalf("PIN must be stored hashed, got %q", s.PinHash)
	}
}

func TestWhatsAppModeSwitchRouting(t *testing.T) {
	cases := []struct{ in, rest, mode string }{
		{"@child she got stuck on Q5", "she got stuck on Q5", waRoutingModeChild},
		{"here is her working @child", "here is her working", waRoutingModeChild},
		{"@parent back to me", "back to me", waRoutingModeParent},
		{"plain message", "plain message", ""},
	}
	for _, c := range cases {
		rest, mode := extractModeSwitch(c.in)
		if rest != c.rest || mode != c.mode {
			t.Fatalf("%q -> (%q,%q), want (%q,%q)", c.in, rest, mode, c.rest, c.mode)
		}
	}
}

func TestEmbeddedSkillsExcludeSharedReference(t *testing.T) {
	list := embeddedSkills()
	names := map[string]bool{}
	for _, sk := range list {
		names[sk.Name] = true
		if sk.Description == "" || sk.Content == "" {
			t.Fatalf("skill %s parsed without description or body", sk.Name)
		}
	}
	for _, want := range []string{"create-test", "create-study-material", "read-file"} {
		if !names[want] {
			t.Fatalf("missing embedded skill %s in %v", want, names)
		}
	}
	if names["_shared"] {
		t.Fatal("_shared is reference material, not a skill")
	}
}

// The archive housekeeping renames any non-reserved top-level folder idle for
// a week. Platform folders must never be candidates, whatever the data dir.
func TestPlatformFoldersAreNeverSubjects(t *testing.T) {
	dir := setupFamily(t)
	for _, name := range []string{"_users", "Workflow", "pulse", "memories", "config", "chat_history", "Chats", "Downloads"} {
		if err := os.MkdirAll(filepath.Join(dir, "workspace", name, "Topic", "slug"), 0o755); err != nil {
			t.Fatal(err)
		}
		if isSubjectDir(name) {
			t.Fatalf("%s treated as a Subject", name)
		}
	}
	for _, act := range listActivities() {
		for _, name := range []string{"_users", "Workflow", "pulse"} {
			if strings.HasPrefix(act.Dir, name+"/") {
				t.Fatalf("activity discovered under platform folder: %s", act.Dir)
			}
		}
	}
}
