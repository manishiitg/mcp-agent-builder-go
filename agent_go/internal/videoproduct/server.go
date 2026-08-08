package videoproduct

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/claudeauth"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/platformevents"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "video_session"

type Server struct {
	config        Config
	store         *Store
	runner        AgentRunner
	workflow      *WorkflowService
	mux           *http.ServeMux
	busyMu        sync.Mutex
	busy          map[string]bool
	notifyCtx     context.Context
	notifyCancel  context.CancelFunc
	notifyWG      sync.WaitGroup
	notifications chan workflowAutoNotification
}

func NewServer(config Config) (*Server, error) {
	if strings.TrimSpace(config.DataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	store, err := OpenStore(config.DataDir)
	if err != nil {
		return nil, err
	}
	if _, _, err := store.UserByEmail("manish"); errors.Is(err, ErrNotFound) {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte("12345"), bcrypt.DefaultCost)
		if hashErr != nil {
			store.Close()
			return nil, fmt.Errorf("prepare local login: %w", hashErr)
		}
		if _, createErr := store.CreateUser("manish", "Manish", hash); createErr != nil {
			store.Close()
			return nil, fmt.Errorf("create local login: %w", createErr)
		}
	} else if err != nil {
		store.Close()
		return nil, fmt.Errorf("check local login: %w", err)
	}
	if strings.TrimSpace(config.WorkspaceAPIURL) == "" {
		config.WorkspaceAPIURL = DefaultWorkspaceURL
	}
	_ = os.Setenv("WORKSPACE_DOCS_PATH", config.DataDir)
	_ = os.Setenv("WORKSPACE_API_URL", config.WorkspaceAPIURL)
	// Publish this product's skills to the shared resolver before any agent
	// exists, so a stage naming one in enabled_skills resolves it.
	if err := RegisterProductSkills(); err != nil {
		store.Close()
		return nil, err
	}
	workflow := NewWorkflowService(store, config.WorkspaceAPIURL, config.MCPConfigPath)
	// Start the shared MCP bridge now, before any request can build a
	// workflow tool registry that would otherwise snapshot MCP_API_TOKEN
	// before the bridge has set it. See WarmSharedBridge for the race this
	// avoids.
	if err := agentsession.WarmSharedBridge(loggerv2.NewDefault()); err != nil {
		log.Printf("warm shared MCP bridge: %v", err)
	}
	runner := config.Runner
	if runner == nil {
		runner = NewClaudeRunner(workflow)
	}
	notifyCtx, notifyCancel := context.WithCancel(context.Background())
	s := &Server{
		config: config, store: store, runner: runner, workflow: workflow, mux: http.NewServeMux(), busy: map[string]bool{},
		notifyCtx: notifyCtx, notifyCancel: notifyCancel, notifications: make(chan workflowAutoNotification, 64),
	}
	workflow.SetAutoNotificationHandler(s.enqueueWorkflowAutoNotification)
	workflow.SetExecutionEventHandler(s.recordExecutionEvent)
	s.notifyWG.Add(1)
	go s.runWorkflowAutoNotifications()
	s.routes()
	return s, nil
}

func (s *Server) Close() error {
	s.notifyCancel()
	s.notifyWG.Wait()
	s.workflow.Close()
	return s.store.Close()
}
func (s *Server) Handler() http.Handler { return s.localOnly(s.cors(s.mux)) }
func (s *Server) Store() *Store         { return s.store }

func (s *Server) enqueueWorkflowAutoNotification(notification workflowAutoNotification) {
	select {
	case s.notifications <- notification:
	case <-s.notifyCtx.Done():
	}
}

func (s *Server) recordExecutionEvent(event platformevents.Event) {
	_, _ = s.store.AddExecutionEvent(event)
}

func (s *Server) recordAgentExecutionEvent(projectID string, event AgentEvent) {
	if event.Type != "tool" {
		return
	}
	eventType := platformevents.ToolCompleted
	if event.Status == "running" {
		eventType = platformevents.ToolStarted
	} else if event.Status == "failed" {
		eventType = platformevents.ToolFailed
	}
	name := event.Tool
	if event.Workflow != "" {
		name = event.Workflow
		if event.Step != "" {
			name += " → " + event.Step
		}
	}
	s.recordExecutionEvent(platformevents.Event{ScopeID: projectID, Type: eventType, Name: name, Status: event.Status, ExecutionID: event.ToolCallID})
}

func (s *Server) runWorkflowAutoNotifications() {
	defer s.notifyWG.Done()
	for {
		select {
		case <-s.notifyCtx.Done():
			return
		case notification := <-s.notifications:
			s.processWorkflowAutoNotification(s.notifyCtx, notification)
		}
	}
}

// processWorkflowAutoNotification mirrors AgentWorks' synthetic completion
// turn: wait until the user's real turn is finished, resume the same provider
// session with [AUTO-NOTIFICATION] as its newest message, and persist only the
// main agent's user-facing continuation.
func (s *Server) processWorkflowAutoNotification(parent context.Context, notification workflowAutoNotification) {
	if strings.TrimSpace(notification.ProjectID) == "" || strings.TrimSpace(notification.UserID) == "" {
		return
	}
	for !s.setBusy(notification.ProjectID, true) {
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-parent.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	defer s.setBusy(notification.ProjectID, false)

	// Finalize before resuming Claude so an already-approved next execute_step
	// reuses this workflow run instead of creating a duplicate run for the group.
	if notification.RunID != "" && notification.FinalStatus != "" {
		_ = s.store.FinishWorkflowRun(notification.RunID, notification.FinalStatus)
	}

	project, err := s.store.Project(notification.UserID, notification.ProjectID)
	if err != nil {
		return
	}
	history, err := s.store.Messages(notification.UserID, notification.ProjectID)
	if err != nil {
		return
	}
	handle, err := s.store.SessionHandle(notification.UserID, notification.ProjectID)
	if err != nil {
		return
	}
	secretEnv, err := s.store.SecretEnv(notification.UserID)
	if err != nil {
		return
	}
	// No token, no resumed turn. There is nobody watching this path to prompt,
	// and the alternative is billing the machine's login for a background run.
	providerToken, err := s.store.Secret(notification.UserID, ClaudeCodeTokenSecret)
	if err != nil || strings.TrimSpace(providerToken) == "" {
		return
	}
	history = append(history, Message{ProjectID: notification.ProjectID, UserID: notification.UserID, Role: "user", Author: "System", Body: notification.Message, CreatedAt: time.Now().UTC()})
	ctx, cancel := context.WithTimeout(parent, 60*time.Minute)
	defer cancel()
	result, runErr := s.runner.Run(ctx, ProjectContext{
		Project: project, UserID: notification.UserID, WorkspacePath: s.store.ProjectDir(notification.ProjectID),
		SessionHandle: handle, History: history, SecretEnv: secretEnv, ProviderToken: providerToken,
	}, func(event AgentEvent) { s.recordAgentExecutionEvent(notification.ProjectID, event) })
	if runErr != nil {
		_, _ = s.store.AddMessage(notification.ProjectID, "", "assistant", "Studio agent", "The workflow finished, but I couldn't continue the conversation automatically. Send me a message and I'll pick up from the completed result.")
		return
	}
	if len(result.SessionHandle) > 0 {
		_ = s.store.SaveSessionHandle(notification.ProjectID, result.SessionHandle)
	}
	if reply := strings.TrimSpace(result.Reply); reply != "" {
		_, _ = s.store.AddMessage(notification.ProjectID, "", "assistant", "Studio agent", reply)
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("OPTIONS /api/{rest...}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)
	s.mux.HandleFunc("GET /api/auth/me", s.withUser(s.me))
	s.mux.HandleFunc("GET /api/projects", s.withUser(s.listProjects))
	s.mux.HandleFunc("POST /api/projects", s.withUser(s.createProject))
	s.mux.HandleFunc("GET /api/projects/{projectID}", s.withUser(s.getProject))
	s.mux.HandleFunc("PATCH /api/projects/{projectID}", s.withUser(s.updateProject))
	s.mux.HandleFunc("DELETE /api/projects/{projectID}", s.withUser(s.archiveProject))
	s.mux.HandleFunc("GET /api/projects/{projectID}/messages", s.withUser(s.listMessages))
	s.mux.HandleFunc("GET /api/projects/{projectID}/execution-events", s.withUser(s.listExecutionEvents))
	s.mux.HandleFunc("GET /api/projects/{projectID}/assets", s.withUser(s.listAssets))
	s.mux.HandleFunc("POST /api/projects/{projectID}/assets", s.withUser(s.uploadAsset))
	s.mux.HandleFunc("GET /api/projects/{projectID}/assets/{assetID}/content", s.withUser(s.assetContent))
	s.mux.HandleFunc("DELETE /api/projects/{projectID}/assets/{assetID}", s.withUser(s.deleteAsset))
	s.mux.HandleFunc("GET /api/projects/{projectID}/videos", s.withUser(s.listVideos))
	s.mux.HandleFunc("GET /api/projects/{projectID}/workflows", s.withUser(s.listWorkflows))
	s.mux.HandleFunc("GET /api/projects/{projectID}/videos/{videoID}/content", s.withUser(s.videoContent))
	s.mux.HandleFunc("GET /api/projects/{projectID}/files", s.withUser(s.projectFiles))
	s.mux.HandleFunc("GET /api/projects/{projectID}/files/content", s.withUser(s.projectFileContent))
	s.mux.HandleFunc("POST /api/projects/{projectID}/chat", s.withUser(s.chat))
	s.mux.HandleFunc("POST /api/projects/{projectID}/chat/steer", s.withUser(s.steer))
	s.mux.HandleFunc("POST /api/projects/{projectID}/chat/cancel", s.withUser(s.cancel))
	s.mux.HandleFunc("GET /api/secrets", s.withUser(s.listSecrets))
	s.mux.HandleFunc("PUT /api/secrets/{name}", s.withUser(s.putSecret))
	s.mux.HandleFunc("DELETE /api/secrets/{name}", s.withUser(s.deleteSecret))
	s.mux.HandleFunc("GET /api/provider-token", s.withUser(s.getProviderToken))
	s.mux.HandleFunc("PUT /api/provider-token", s.withUser(s.putProviderToken))
	s.mux.HandleFunc("DELETE /api/provider-token", s.withUser(s.deleteProviderToken))
	s.mux.HandleFunc("GET /api/skills", s.withUser(s.listSkills))
}

func (s *Server) localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip := net.ParseIP(host)
			if ip != nil && !ip.IsLoopback() {
				writeError(w, http.StatusForbidden, "local access only")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == s.config.FrontendOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			w.Header().Add("Vary", "Origin")
		}
		if origin != "" && origin != s.config.FrontendOrigin {
			writeError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "video-studio", "agent": "claude-code", "dataDir": s.store.DataDir()})
}

type userHandler func(http.ResponseWriter, *http.Request, User)

func (s *Server) withUser(next userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		u, err := s.store.UserBySession(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		next(w, r, u)
	}
}

type authInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in authInput
	if !decodeJSON(w, r, &in) {
		return
	}
	u, hash, err := s.store.UserByEmail(in.Email)
	if err != nil || bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err = s.startSession(w, u); err != nil {
		writeError(w, 500, "could not start session")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
func (s *Server) startSession(w http.ResponseWriter, u User) error {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(data)
	expires := time.Now().Add(30 * 24 * time.Hour)
	if err := s.store.CreateSession(u.ID, token, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", Expires: expires, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: false})
	return nil
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) me(w http.ResponseWriter, r *http.Request, u User) { writeJSON(w, 200, u) }

func (s *Server) isBusy(id string) bool { s.busyMu.Lock(); defer s.busyMu.Unlock(); return s.busy[id] }
func (s *Server) setBusy(id string, value bool) bool {
	s.busyMu.Lock()
	defer s.busyMu.Unlock()
	if value && s.busy[id] {
		return false
	}
	if value {
		s.busy[id] = true
	} else {
		delete(s.busy, id)
	}
	return true
}
func (s *Server) decorate(p Project) Project {
	if s.isBusy(p.ID) {
		p.SessionStatus = "working"
	} else {
		p.SessionStatus = "ready"
	}
	return p
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, u User) {
	items, err := s.store.Projects(u.ID)
	if err != nil {
		writeError(w, 500, "could not list projects")
		return
	}
	for i := range items {
		items[i] = s.decorate(items[i])
	}
	if items == nil {
		items = []Project{}
	}
	writeJSON(w, 200, items)
}

type projectInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, u User) {
	var in projectInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 120 {
		writeError(w, 400, "project title is required and must be under 120 characters")
		return
	}
	p, err := s.store.CreateProject(u, in.Title, in.Description)
	if err != nil {
		writeError(w, 500, "could not create project")
		return
	}
	if err := s.workflow.EnsureProject(p); err != nil {
		writeError(w, 500, "project was created but its workflow could not be prepared")
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request, u User) {
	projectID := r.PathValue("projectID")
	p, err := s.store.Project(u.ID, projectID)
	if !handleStoreError(w, err) {
		return
	}
	if err := s.workflow.EnsureProject(p); err != nil {
		writeError(w, 500, "could not prepare project workflows")
		return
	}
	runs, err := s.store.WorkflowRuns(u.ID, projectID)
	if !handleStoreError(w, err) {
		return
	}
	if runs == nil {
		runs = []WorkflowRun{}
	}
	workflows := make([]map[string]interface{}, 0, len(pipelineRegistry))
	for _, pipeline := range pipelineRegistry {
		workflows = append(workflows, map[string]interface{}{
			"id": pipeline.ID, "name": pipeline.Name, "description": pipeline.Description,
			"steps": pipeline.Steps(),
		})
	}
	writeJSON(w, 200, map[string]interface{}{
		"workflows": workflows, "runs": runs,
	})
}
func (s *Server) getProject(w http.ResponseWriter, r *http.Request, u User) {
	p, err := s.store.Project(u.ID, r.PathValue("projectID"))
	if !handleStoreError(w, err) {
		return
	}
	writeJSON(w, 200, s.decorate(p))
}
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request, u User) {
	var in projectInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		writeError(w, 400, "project title is required")
		return
	}
	p, err := s.store.UpdateProject(u.ID, r.PathValue("projectID"), in.Title, in.Description)
	if !handleStoreError(w, err) {
		return
	}
	writeJSON(w, 200, p)
}
func (s *Server) archiveProject(w http.ResponseWriter, r *http.Request, u User) {
	if s.isBusy(r.PathValue("projectID")) {
		writeError(w, 409, "cancel the running agent first")
		return
	}
	err := s.store.ArchiveProject(u.ID, r.PathValue("projectID"))
	if !handleStoreError(w, err) {
		return
	}
	w.WriteHeader(204)
}
func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, u User) {
	items, err := s.store.Messages(u.ID, r.PathValue("projectID"))
	if !handleStoreError(w, err) {
		return
	}
	if items == nil {
		items = []Message{}
	}
	writeJSON(w, 200, items)
}

func (s *Server) listExecutionEvents(w http.ResponseWriter, r *http.Request, u User) {
	items, err := s.store.ExecutionEvents(u.ID, r.PathValue("projectID"))
	if !handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request, u User) {
	items, err := s.store.Assets(u.ID, r.PathValue("projectID"))
	if !handleStoreError(w, err) {
		return
	}
	if items == nil {
		items = []Asset{}
	}
	writeJSON(w, 200, items)
}
func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request, u User) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, 400, "invalid or oversized upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "file is required")
		return
	}
	defer file.Close()
	a, err := s.store.SaveAsset(u.ID, r.PathValue("projectID"), header.Filename, header.Header.Get("Content-Type"), file)
	if !handleStoreError(w, err) {
		return
	}
	writeJSON(w, 201, a)
}
func (s *Server) assetContent(w http.ResponseWriter, r *http.Request, u User) {
	path, name, err := s.store.AssetPath(u.ID, r.PathValue("projectID"), r.PathValue("assetID"))
	if !handleStoreError(w, err) {
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", name))
	http.ServeFile(w, r, path)
}
func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request, u User) {
	err := s.store.DeleteAsset(u.ID, r.PathValue("projectID"), r.PathValue("assetID"))
	if !handleStoreError(w, err) {
		return
	}
	w.WriteHeader(204)
}

var videoExtensions = map[string]bool{".mp4": true, ".mov": true, ".webm": true, ".m4v": true}

func (s *Server) projectVideos(userID, projectID string) ([]Video, error) {
	if _, err := s.store.Project(userID, projectID); err != nil {
		return nil, err
	}
	projectRoot := s.store.ProjectDir(projectID)
	items := []Video{}

	// Prefer what the agent chose to show. A run leaves many video files behind
	// — raw shots, silent intermediates, byte-identical delivery copies — so
	// listing every file presents them all as equally finished and buries the
	// one the user actually wants. Fall back to scanning only while the agent has
	// not presented anything, so a project is never silently empty.
	if presented, err := s.store.PresentedVideos(userID, projectID); err != nil {
		return nil, err
	} else if len(presented) > 0 {
		for _, video := range presented {
			path := filepath.Join(projectRoot, filepath.FromSlash(video.Path))
			info, statErr := os.Stat(path)
			if statErr != nil || info.IsDir() {
				continue // presented then deleted; skip rather than 404 the panel
			}
			id := base64.RawURLEncoding.EncodeToString([]byte(video.Path))
			items = append(items, Video{
				ID: id, Name: video.Title, Size: info.Size(), CreatedAt: video.CreatedAt, Note: video.Note,
				ContentURL: "/api/projects/" + projectID + "/videos/" + id + "/content",
			})
		}
		return items, nil
	}
	for _, folder := range []string{"outputs", "runs"} {
		root := filepath.Join(projectRoot, folder)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if d.IsDir() || !videoExtensions[strings.ToLower(filepath.Ext(d.Name()))] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(projectRoot, path)
			if err != nil {
				return err
			}
			id := base64.RawURLEncoding.EncodeToString([]byte(filepath.ToSlash(rel)))
			items = append(items, Video{ID: id, Name: d.Name(), Size: info.Size(), CreatedAt: info.ModTime(), ContentURL: "/api/projects/" + projectID + "/videos/" + id + "/content"})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (s *Server) listVideos(w http.ResponseWriter, r *http.Request, u User) {
	items, err := s.projectVideos(u.ID, r.PathValue("projectID"))
	if !handleStoreError(w, err) {
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) videoPath(userID, projectID, videoID string) (string, error) {
	if _, err := s.store.Project(userID, projectID); err != nil {
		return "", err
	}
	decoded, err := base64.RawURLEncoding.DecodeString(videoID)
	if err != nil {
		return "", ErrNotFound
	}
	root := s.store.ProjectDir(projectID)
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(string(decoded))))
	if !strings.HasPrefix(path, filepath.Clean(root)+string(os.PathSeparator)) {
		return "", ErrNotFound
	}
	if !videoExtensions[strings.ToLower(filepath.Ext(path))] {
		return "", ErrNotFound
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", ErrNotFound
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 || (parts[0] != "outputs" && parts[0] != "runs") {
		return "", ErrNotFound
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", ErrNotFound
	}
	return path, nil
}
func (s *Server) videoContent(w http.ResponseWriter, r *http.Request, u User) {
	path, err := s.videoPath(u.ID, r.PathValue("projectID"), r.PathValue("videoID"))
	if !handleStoreError(w, err) {
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(path)))
	http.ServeFile(w, r, path)
}

func (s *Server) projectFilePath(userID, projectID, relativePath string) (string, error) {
	if _, err := s.store.Project(userID, projectID); err != nil {
		return "", err
	}
	relativePath = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relativePath)))
	if relativePath == "." || filepath.IsAbs(relativePath) {
		return "", ErrNotFound
	}
	root := filepath.Clean(s.store.ProjectDir(projectID))
	path := filepath.Clean(filepath.Join(root, relativePath))
	if !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", ErrNotFound
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", ErrNotFound
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.HasPrefix(filepath.Clean(resolved), filepath.Clean(resolvedRoot)+string(os.PathSeparator)) {
		return "", ErrNotFound
	}
	if info, err := os.Stat(resolved); err != nil || !info.Mode().IsRegular() {
		return "", ErrNotFound
	}
	return resolved, nil
}

type projectFileNode struct {
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	Type     string            `json:"type"`
	Size     int64             `json:"size"`
	Children []projectFileNode `json:"children,omitempty"`
}

func readProjectFileNodes(root, relative string) ([]projectFileNode, int64, error) {
	directory := filepath.Join(root, filepath.FromSlash(relative))
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []projectFileNode{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	nodes := make([]projectFileNode, 0, len(entries))
	var total int64
	for _, entry := range entries {
		path := filepath.ToSlash(filepath.Join(relative, entry.Name()))
		if entry.IsDir() {
			children, size, err := readProjectFileNodes(root, path)
			if err != nil {
				return nil, 0, err
			}
			nodes = append(nodes, projectFileNode{Name: entry.Name(), Path: path, Type: "folder", Size: size, Children: children})
			total += size
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, 0, err
		}
		nodes = append(nodes, projectFileNode{Name: entry.Name(), Path: path, Type: "file", Size: info.Size()})
		total += info.Size()
	}
	return nodes, total, nil
}

func (s *Server) projectFiles(w http.ResponseWriter, r *http.Request, u User) {
	projectID := r.PathValue("projectID")
	if _, err := s.store.Project(u.ID, projectID); !handleStoreError(w, err) {
		return
	}
	root := filepath.Clean(s.store.ProjectDir(projectID))
	nodes, _, err := readProjectFileNodes(root, "")
	if err != nil {
		writeError(w, 500, "could not list project files")
		return
	}
	writeJSON(w, 200, nodes)
}

func (s *Server) projectFileContent(w http.ResponseWriter, r *http.Request, u User) {
	path, err := s.projectFilePath(u.ID, r.PathValue("projectID"), r.URL.Query().Get("path"))
	if !handleStoreError(w, err) {
		return
	}
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(path)))
	http.ServeFile(w, r, path)
}

type chatInput struct {
	Message string `json:"message"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request, u User) {
	projectID := r.PathValue("projectID")
	p, err := s.store.Project(u.ID, projectID)
	if !handleStoreError(w, err) {
		return
	}
	var in chatInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" || len(in.Message) > 50000 {
		writeError(w, 400, "message is required and must be under 50,000 characters")
		return
	}
	if !s.setBusy(projectID, true) {
		writeError(w, 409, "project agent is already working")
		return
	}
	defer s.setBusy(projectID, false)
	if _, err = s.store.AddMessage(projectID, u.ID, "user", u.Name, in.Message); err != nil {
		writeError(w, 500, "could not save message")
		return
	}
	history, err := s.store.Messages(u.ID, projectID)
	if err != nil {
		writeError(w, 500, "could not load conversation")
		return
	}
	handle, err := s.store.SessionHandle(u.ID, projectID)
	if err != nil {
		writeError(w, 500, "could not continue this project")
		return
	}
	secretEnv, err := s.store.SecretEnv(u.ID)
	if err != nil {
		writeError(w, 500, "could not load secrets")
		return
	}
	// Refuse the turn here rather than letting the session fall back to whatever
	// Claude Code login exists on this machine. 428 tells the UI to send the
	// user to Settings instead of showing a generic failure.
	providerToken, err := s.store.Secret(u.ID, ClaudeCodeTokenSecret)
	if errors.Is(err, ErrNotFound) || (err == nil && strings.TrimSpace(providerToken) == "") {
		writeError(w, 428, "Add your Claude Code token in Settings before starting a session. Run `claude setup-token` to create one.")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load your Claude Code token")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	events := make(chan AgentEvent, 64)
	done := make(chan struct {
		result AgentResult
		err    error
	}, 1)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Minute)
	defer cancel()
	go func() {
		result, err := s.runner.Run(ctx, ProjectContext{Project: p, UserID: u.ID, WorkspacePath: s.store.ProjectDir(projectID), SessionHandle: handle, History: history, SecretEnv: secretEnv, ProviderToken: providerToken}, func(event AgentEvent) {
			s.recordAgentExecutionEvent(projectID, event)
			select {
			case events <- event:
			case <-ctx.Done():
			}
		})
		done <- struct {
			result AgentResult
			err    error
		}{result, err}
	}()
	sse(w, "status", map[string]string{"status": "working", "agent": "claude-code"})
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event := <-events:
			sse(w, event.Type, event)
			flusher.Flush()
		case outcome := <-done:
			// Run has returned, so no producer can add another event. Flush any
			// buffered final text/tool events before the terminal event.
			for {
				select {
				case event := <-events:
					sse(w, event.Type, event)
				default:
					goto eventsDrained
				}
			}
		eventsDrained:
			if outcome.err != nil {
				sse(w, "error", map[string]string{"error": friendlyAgentError(outcome.err)})
				flusher.Flush()
				return
			}
			if len(outcome.result.SessionHandle) > 0 {
				_ = s.store.SaveSessionHandle(projectID, outcome.result.SessionHandle)
			}
			message, err := s.store.AddMessage(projectID, "", "assistant", "Studio agent", outcome.result.Reply)
			if err != nil {
				sse(w, "error", map[string]string{"error": "reply finished but could not be saved"})
				flusher.Flush()
				return
			}
			videos, _ := s.projectVideos(u.ID, projectID)
			sse(w, "completed", map[string]any{"message": message, "videos": videos})
			flusher.Flush()
			return
		case <-heartbeat.C:
			sse(w, "heartbeat", map[string]string{"status": "working"})
			flusher.Flush()
		case <-ctx.Done():
			sse(w, "error", map[string]string{"error": "Claude turn cancelled"})
			flusher.Flush()
			return
		}
	}
}
func friendlyAgentError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "Claude turn cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Claude turn timed out"
	}
	return "The agent could not complete this request: " + err.Error()
}
func (s *Server) steer(w http.ResponseWriter, r *http.Request, u User) {
	projectID := r.PathValue("projectID")
	if _, err := s.store.Project(u.ID, projectID); !handleStoreError(w, err) {
		return
	}
	var in chatInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" {
		writeError(w, 400, "message is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.runner.Steer(ctx, projectID, in.Message); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	m, err := s.store.AddMessage(projectID, u.ID, "user", u.Name, in.Message)
	if err != nil {
		writeError(w, 500, "steering sent but message could not be saved")
		return
	}
	writeJSON(w, 200, m)
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request, u User) {
	projectID := r.PathValue("projectID")
	if _, err := s.store.Project(u.ID, projectID); !handleStoreError(w, err) {
		return
	}
	if !s.runner.Cancel(projectID) {
		writeError(w, 409, "project agent is not currently working")
		return
	}
	writeJSON(w, 200, map[string]bool{"cancelled": true})
}

func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request, u User) {
	names, err := s.store.SecretNames(u.ID)
	if err != nil {
		writeError(w, 500, "could not list secrets")
		return
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		// The provider token has its own card in Settings; showing it here too
		// would invite editing it through a path that skips validation.
		if name != ClaudeCodeTokenSecret {
			filtered = append(filtered, name)
		}
	}
	writeJSON(w, 200, map[string]any{"names": filtered})
}

// providerToken reports whether this user can start a session at all. The
// response deliberately carries no token value — only whether one is stored.
func (s *Server) getProviderToken(w http.ResponseWriter, r *http.Request, u User) {
	token, err := s.store.Secret(u.ID, ClaudeCodeTokenSecret)
	if err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, 500, "could not read your Claude Code token")
		return
	}
	writeJSON(w, 200, map[string]any{"configured": strings.TrimSpace(token) != ""})
}

func (s *Server) putProviderToken(w http.ResponseWriter, r *http.Request, u User) {
	var in secretInput
	if !decodeJSON(w, r, &in) {
		return
	}
	// Validate before storing. A token that Claude Code rejects would otherwise
	// be indistinguishable from a working one until the next turn failed.
	if err := claudeauth.ValidateOAuthToken(r.Context(), in.Value); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := s.store.PutSecret(u.ID, ClaudeCodeTokenSecret, strings.TrimSpace(in.Value)); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"configured": true})
}

func (s *Server) deleteProviderToken(w http.ResponseWriter, r *http.Request, u User) {
	if err := s.store.DeleteSecret(u.ID, ClaudeCodeTokenSecret); err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, 500, "could not remove your Claude Code token")
		return
	}
	w.WriteHeader(204)
}

type secretInput struct {
	Value string `json:"value"`
}

func (s *Server) putSecret(w http.ResponseWriter, r *http.Request, u User) {
	var in secretInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.ToUpper(strings.TrimSpace(r.PathValue("name"))) == ClaudeCodeTokenSecret {
		writeError(w, 400, "Set your Claude Code token from the Claude Code card in Settings, so it can be validated first.")
		return
	}
	if err := s.store.PutSecret(u.ID, r.PathValue("name"), in.Value); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"name": strings.ToUpper(r.PathValue("name"))})
}
func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request, u User) {
	if err := s.store.DeleteSecret(u.ID, r.PathValue("name")); !handleStoreError(w, err) {
		return
	}
	w.WriteHeader(204)
}
func (s *Server) listSkills(w http.ResponseWriter, r *http.Request, u User) {
	skills := make([]map[string]string, 0, len(builtinSkillDefinitions))
	for _, skill := range builtinSkillDefinitions {
		skills = append(skills, map[string]string{"name": skill.name, "description": skill.description})
	}
	writeJSON(w, 200, skills)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, "invalid JSON request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "request must contain one JSON object")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func handleStoreError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, 404, "not found")
	} else {
		writeError(w, 500, err.Error())
	}
	return false
}
func sse(w io.Writer, event string, value any) {
	data, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
