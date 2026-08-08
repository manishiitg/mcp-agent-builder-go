package videoproduct

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

type fakeRunner struct{}

type autoNotificationRunner struct {
	calls chan ProjectContext
}

func TestDefaultClaudeModel(t *testing.T) {
	if DefaultClaudeModel != "claude-sonnet-5" {
		t.Fatalf("default model = %q, want claude-sonnet-5", DefaultClaudeModel)
	}
}

func (fakeRunner) Run(_ context.Context, project ProjectContext, emit func(AgentEvent)) (AgentResult, error) {
	emit(AgentEvent{Type: "tool", Tool: "execute_shell_command", ToolCallID: "shell-test", Status: "running"})
	emit(AgentEvent{Type: "delta", Text: "Video ready."})
	emit(AgentEvent{Type: "tool", Tool: "execute_shell_command", ToolCallID: "shell-test", Status: "completed", DurationMS: 12})
	if err := os.WriteFile(filepath.Join(project.WorkspacePath, "outputs", "demo.mp4"), []byte("fake-video"), 0600); err != nil {
		return AgentResult{}, err
	}
	return AgentResult{Reply: "Video ready.", SessionHandle: []byte(`{"provider":{"session_id":"claude-test"}}`)}, nil
}
func (fakeRunner) Steer(context.Context, string, string) error { return nil }
func (fakeRunner) Cancel(string) bool                          { return true }

func (r *autoNotificationRunner) Run(_ context.Context, project ProjectContext, _ func(AgentEvent)) (AgentResult, error) {
	r.calls <- project
	return AgentResult{Reply: "Research is ready. I reviewed the result and need your approval before moving to the creative proposal.", SessionHandle: []byte(`{"provider":{"session_id":"claude-auto"}}`)}, nil
}
func (*autoNotificationRunner) Steer(context.Context, string, string) error { return nil }
func (*autoNotificationRunner) Cancel(string) bool                          { return true }

type testClient struct {
	handler http.Handler
	cookie  *http.Cookie
}

func (c *testClient) request(t *testing.T, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:45678"
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	recorder := httptest.NewRecorder()
	c.handler.ServeHTTP(recorder, req)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			c.cookie = cookie
		}
	}
	return recorder
}

func newTestServer(t *testing.T) (*Server, *testClient) {
	t.Helper()
	server, err := NewServer(Config{DataDir: t.TempDir(), FrontendOrigin: DefaultFrontendOrigin, Runner: fakeRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, &testClient{handler: server.Handler()}
}

func loginUser(t *testing.T, client *testClient, username, password string) User {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": username, "password": password})
	response := client.request(t, http.MethodPost, "/api/auth/login", body, "application/json")
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", response.Code, response.Body.String())
	}
	var user User
	if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	return user
}

// seedProviderToken satisfies the gate that stops a session from starting
// without the user's own Claude Code token. It writes to the vault directly
// because the HTTP route validates against the real CLI, which a unit test must
// not depend on.
func seedProviderToken(t *testing.T, server *Server, userID string) {
	t.Helper()
	if err := server.store.PutSecret(userID, ClaudeCodeTokenSecret, "sk-ant-oat01-test-token"); err != nil {
		t.Fatal(err)
	}
}

func createProjectForTest(t *testing.T, client *testClient) Project {
	t.Helper()
	response := client.request(t, http.MethodPost, "/api/projects", []byte(`{"title":"Launch film","description":"A launch video"}`), "application/json")
	if response.Code != http.StatusCreated {
		t.Fatalf("create project status = %d: %s", response.Code, response.Body.String())
	}
	var project Project
	if err := json.Unmarshal(response.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	return project
}

func TestAuthProjectsAndWorkspaceIsolation(t *testing.T) {
	server, owner := newTestServer(t)
	loginUser(t, owner, "manish", "12345")
	project := createProjectForTest(t, owner)
	for _, directory := range []string{"uploads", "outputs", "work"} {
		info, err := os.Stat(filepath.Join(server.store.ProjectDir(project.ID), directory))
		if err != nil || !info.IsDir() {
			t.Fatalf("workspace directory %s was not created", directory)
		}
	}

	other := &testClient{handler: server.Handler()}
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.store.CreateUser("other", "Other User", hash); err != nil {
		t.Fatal(err)
	}
	loginUser(t, other, "other", "password123")
	response := other.request(t, http.MethodGet, "/api/projects/"+project.ID, nil, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-user project status = %d, want 404", response.Code)
	}
}

func TestAssetUploadAndClaudeChatStream(t *testing.T) {
	server, client := newTestServer(t)
	user := loginUser(t, client, "manish", "12345")
	seedProviderToken(t, server, user.ID)
	project := createProjectForTest(t, client)

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "reference.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("reference material"))
	_ = writer.Close()
	response := client.request(t, http.MethodPost, "/api/projects/"+project.ID+"/assets", upload.Bytes(), writer.FormDataContentType())
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", response.Code, response.Body.String())
	}

	response = client.request(t, http.MethodPost, "/api/projects/"+project.ID+"/chat", []byte(`{"message":"Make a short video"}`), "application/json")
	if response.Code != http.StatusOK {
		t.Fatalf("chat status = %d: %s", response.Code, response.Body.String())
	}
	stream := response.Body.String()
	for _, event := range []string{"event: status", "event: delta", "event: tool", "event: completed", `"agent":"claude-code"`, `"callId":"shell-test"`, `"durationMs":12`} {
		if !strings.Contains(stream, event) {
			t.Fatalf("chat stream missing %q: %s", event, stream)
		}
	}
	videos, err := server.projectVideos(user.ID, project.ID)
	if err != nil || len(videos) != 1 || videos[0].Name != "demo.mp4" {
		t.Fatalf("videos = %#v, err = %v", videos, err)
	}
	handle, err := server.store.SessionHandle(user.ID, project.ID)
	if err != nil || len(handle) == 0 {
		t.Fatalf("Claude session handle was not persisted: %v", err)
	}
}

// A session with no token would otherwise fall back to whichever Claude Code
// login exists on the machine — running, and billing, as someone else. The turn
// has to be refused before the agent is constructed.
func TestChatIsRefusedWithoutAClaudeCodeToken(t *testing.T) {
	_, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)

	response := client.request(t, http.MethodPost, "/api/projects/"+project.ID+"/chat", []byte(`{"message":"Make a short video"}`), "application/json")
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("chat status = %d, want 428: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "setup-token") {
		t.Fatalf("error should tell the user how to create a token: %s", response.Body.String())
	}
}

// The vault holds the token, but exporting it as a shell variable would let any
// command the agent runs re-authenticate on its own.
func TestProviderTokenIsNotExportedToTheShellEnvironment(t *testing.T) {
	server, client := newTestServer(t)
	user := loginUser(t, client, "manish", "12345")
	seedProviderToken(t, server, user.ID)
	if err := server.store.PutSecret(user.ID, "ELEVENLABS_API_KEY", "voice-key"); err != nil {
		t.Fatal(err)
	}

	env, err := server.store.SecretEnv(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, ClaudeCodeTokenSecret+"=") {
			t.Fatalf("provider token leaked into the shell environment: %v", env)
		}
	}
	if len(env) != 1 || !strings.HasPrefix(env[0], "ELEVENLABS_API_KEY=") {
		t.Fatalf("unrelated secrets should still be exported, got %v", env)
	}
}

func TestWorkflowAutoNotificationResumesProjectAgent(t *testing.T) {
	runner := &autoNotificationRunner{calls: make(chan ProjectContext, 1)}
	server, err := NewServer(Config{DataDir: t.TempDir(), FrontendOrigin: DefaultFrontendOrigin, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client := &testClient{handler: server.Handler()}
	user := loginUser(t, client, "manish", "12345")
	seedProviderToken(t, server, user.ID)
	project := createProjectForTest(t, client)
	if _, err := server.store.AddMessage(project.ID, user.ID, "user", user.Name, "Research this launch film"); err != nil {
		t.Fatal(err)
	}
	run, err := server.store.BeginWorkflowRun(project.ID, cinematicWorkflowName, "launch-film", copyCinematicSteps())
	if err != nil {
		t.Fatal(err)
	}

	notification := workflowAutoNotification{
		ProjectID: project.ID, UserID: user.ID, RunID: run.ID, FinalStatus: "ready",
		Message: "[AUTO-NOTIFICATION] Research completed for launch-film.\nResult: research.md was created.",
	}
	server.processWorkflowAutoNotification(context.Background(), notification)

	call := <-runner.calls
	if call.Project.ID != project.ID || call.UserID != user.ID {
		t.Fatalf("auto-notification project context = %+v", call)
	}
	if len(call.History) == 0 {
		t.Fatal("auto-notification turn received no history")
	}
	latest := call.History[len(call.History)-1]
	if latest.Role != "user" || latest.Body != notification.Message {
		t.Fatalf("latest auto-notification turn = %+v", latest)
	}

	messages, err := server.store.Messages(user.ID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || strings.Contains(messages[0].Body, "[AUTO-NOTIFICATION]") || strings.Contains(messages[1].Body, "[AUTO-NOTIFICATION]") {
		t.Fatalf("visible project messages = %+v", messages)
	}
	if !strings.Contains(messages[1].Body, "need your approval") {
		t.Fatalf("main-agent continuation was not persisted: %+v", messages[1])
	}
	runs, err := server.store.WorkflowRuns(user.ID, project.ID)
	if err != nil || len(runs) != 1 || runs[0].Status != "ready" {
		t.Fatalf("workflow runs = %+v, err=%v", runs, err)
	}
	handle, err := server.store.SessionHandle(user.ID, project.ID)
	if err != nil || !bytes.Contains(handle, []byte("claude-auto")) {
		t.Fatalf("resumed session handle = %q, err=%v", handle, err)
	}
}

func TestProjectFileContentIsWorkspaceScoped(t *testing.T) {
	server, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	notePath := filepath.Join(server.store.ProjectDir(project.ID), "work", "notes.md")
	if err := os.WriteFile(notePath, []byte("# Production notes"), 0600); err != nil {
		t.Fatal(err)
	}

	response := client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/files/content?path=work%2Fnotes.md", nil, "")
	if response.Code != http.StatusOK || response.Body.String() != "# Production notes" {
		t.Fatalf("workspace file response = %d %q", response.Code, response.Body.String())
	}
	response = client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/files/content?path=..%2Fsecrets.json", nil, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("traversal response = %d, want 404", response.Code)
	}
	if err := os.MkdirAll(filepath.Join(server.store.ProjectDir(project.ID), ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.store.ProjectDir(project.ID), ".claude", "settings.json"), []byte(`{"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	response = client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/files/content?path=.claude%2Fsettings.json", nil, "")
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":true}` {
		t.Fatalf("hidden project file response = %d %q", response.Code, response.Body.String())
	}
}

func TestProjectFilesReturnsAWorkspaceTree(t *testing.T) {
	server, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	root := server.store.ProjectDir(project.ID)
	if err := os.MkdirAll(filepath.Join(root, "work", "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "work", "notes", "brief.md"), []byte("brief"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "work", ".private"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".root-config.json"), []byte("config"), 0600); err != nil {
		t.Fatal(err)
	}

	response := client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/files", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("project files response = %d: %s", response.Code, response.Body.String())
	}
	var nodes []projectFileNode
	if err := json.Unmarshal(response.Body.Bytes(), &nodes); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(nodes)
	for _, expected := range []string{`"path":"work/notes/brief.md"`, `"path":"work/.private"`, `"path":".root-config.json"`, `"path":"workflow.json"`} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("project file tree missing %s: %s", expected, data)
		}
	}
	if len(nodes) <= 4 {
		t.Fatalf("project file tree = %s", data)
	}
}

func TestEmptyVideoListIsAnArray(t *testing.T) {
	_, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	messages := client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/messages", nil, "")
	if messages.Code != http.StatusOK || strings.TrimSpace(messages.Body.String()) != "[]" {
		t.Fatalf("new project messages = %d %q, want []", messages.Code, messages.Body.String())
	}
	response := client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/videos", nil, "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("empty videos response = %d %q, want []", response.Code, response.Body.String())
	}
}

func TestProjectCreatesCinematicWorkflowWithIsolatedRegularSteps(t *testing.T) {
	server, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	response := client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/workflows", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("workflow response = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Workflows []struct {
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Steps []WorkflowStep `json:"steps"`
		} `json:"workflows"`
		Runs []WorkflowRun `json:"runs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	// The UI is a static catalog of every supported pipeline. It does not change
	// based on the latest routed run.
	if len(payload.Workflows) != len(pipelineRegistry) || len(payload.Runs) != 0 {
		t.Fatalf("unexpected workflow payload: %+v", payload)
	}
	for i, pipeline := range pipelineRegistry {
		if payload.Workflows[i].ID != pipeline.ID || payload.Workflows[i].Name != pipeline.Name || len(payload.Workflows[i].Steps) != len(pipeline.Stages) {
			t.Fatalf("workflow catalog[%d] = %+v, want %s", i, payload.Workflows[i], pipeline.ID)
		}
	}
	totalStages := 0
	for _, pipeline := range pipelineRegistry {
		totalStages += len(pipeline.Stages)
	}
	configData, err := os.ReadFile(filepath.Join(server.store.ProjectDir(project.ID), "planning", "step_config.json"))
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configData)
	if strings.Count(configText, `"db_access": "none"`) != totalStages || strings.Count(configText, `"knowledgebase_access": "none"`) != totalStages || strings.Count(configText, `"learnings_access": "none"`) != totalStages {
		t.Fatalf("all workflow steps must disable DB, KB, and learnings access: %s", configText)
	}
	planData, err := os.ReadFile(filepath.Join(server.store.ProjectDir(project.ID), "planning", "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Every stage is a regular step; routing adds exactly one non-regular step.
	if strings.Count(string(planData), `"type": "regular"`) != totalStages || strings.Contains(string(planData), "message_sequence") {
		t.Fatalf("every pipeline stage must be a regular step: %s", planData)
	}
	if strings.Count(string(planData), `"type": "routing"`) != 1 {
		t.Fatalf("plan must contain exactly one routing step: %s", planData)
	}
}

func TestWorkflowPanelCatalogDoesNotChangeWithLatestRun(t *testing.T) {
	server, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	run, err := server.store.BeginWorkflowRun(project.ID, infographicPipeline.Name, "product-explainer", AllPipelineSteps())
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.FinishWorkflowRun(run.ID, "completed"); err != nil {
		t.Fatal(err)
	}

	response := client.request(t, http.MethodGet, "/api/projects/"+project.ID+"/workflows", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("workflow response = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Workflows []struct {
			ID string `json:"id"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Workflows) != len(pipelineRegistry) || payload.Workflows[0].ID != infographicPipeline.ID || payload.Workflows[1].ID != qualityPipeline.ID {
		t.Fatalf("workflow panel payload = %+v", payload)
	}
}

func TestProjectChatGetsOnlyTheReusableWorkflowTools(t *testing.T) {
	server, client := newTestServer(t)
	user := loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	tools, err := server.workflow.Tools(ProjectContext{Project: project, UserID: user.ID, WorkspacePath: server.store.ProjectDir(project.ID)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	want := []string{"execute_step", "query_step", "send_step_message", "stop_step", "stop_all_executions", "run_full_workflow"}
	if len(names) != len(want) {
		t.Fatalf("workflow tools = %v, want %v", names, want)
	}
	for _, name := range want {
		if !slices.Contains(names, name) {
			t.Fatalf("workflow tools = %v, missing %q", names, name)
		}
	}
}

func TestApprovedStagesContinueInOneWorkflowRun(t *testing.T) {
	server, client := newTestServer(t)
	loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	first, err := server.store.BeginWorkflowRun(project.ID, cinematicWorkflowName, "video-launch", copyCinematicSteps())
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetWorkflowStep(first.ID, "research", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.FinishWorkflowRun(first.ID, "ready"); err != nil {
		t.Fatal(err)
	}
	second, err := server.store.BeginWorkflowRun(project.ID, cinematicWorkflowName, "video-launch", copyCinematicSteps())
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("staged workflow created a new run: first=%s second=%s", first.ID, second.ID)
	}
}

func TestSecretsAreEncryptedAndValuesNeverReturned(t *testing.T) {
	server, client := newTestServer(t)
	user := loginUser(t, client, "manish", "12345")
	value := "top-secret-value-123"
	response := client.request(t, http.MethodPut, "/api/secrets/VIDEO_API_KEY", []byte(`{"value":"`+value+`"}`), "application/json")
	if response.Code != http.StatusOK {
		t.Fatalf("put secret status = %d: %s", response.Code, response.Body.String())
	}
	response = client.request(t, http.MethodGet, "/api/secrets", nil, "")
	if strings.Contains(response.Body.String(), value) || !strings.Contains(response.Body.String(), "VIDEO_API_KEY") {
		t.Fatalf("secret list leaked value or omitted name: %s", response.Body.String())
	}
	env, err := server.store.SecretEnv(user.ID)
	if err != nil || len(env) != 1 || env[0] != "VIDEO_API_KEY="+value {
		t.Fatalf("secret env = %#v, err = %v", env, err)
	}
	for _, suffix := range []string{"video-studio.db", "video-studio.db-wal"} {
		data, err := os.ReadFile(filepath.Join(server.store.DataDir(), suffix))
		if err == nil && bytes.Contains(data, []byte(value)) {
			t.Fatalf("plaintext secret found in %s", suffix)
		}
	}
}

func TestRejectsNonLocalRequests(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}
