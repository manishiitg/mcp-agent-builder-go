package videoproduct

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db      *sql.DB
	dataDir string
	aead    cipher.AEAD
}

func OpenStore(dataDir string) (*Store, error) {
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "projects"), 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	key, err := loadOrCreateKey(filepath.Join(dataDir, ".secrets.key"))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize secret encryption: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "video-studio.db"))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, dataDir: dataDir, aead: aead}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("secret key has invalid length")
		}
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("write secret key: %w", err)
	}
	return key, nil
}

func (s *Store) migrate() error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE COLLATE NOCASE, name TEXT NOT NULL,
 password_hash BLOB NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
 owner_id TEXT NOT NULL REFERENCES users(id), status TEXT NOT NULL DEFAULT 'active',
 session_handle BLOB, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS project_members (
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, role TEXT NOT NULL,
 created_at TEXT NOT NULL, PRIMARY KEY(project_id,user_id)
);
CREATE TABLE IF NOT EXISTS messages (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 user_id TEXT REFERENCES users(id), role TEXT NOT NULL, author TEXT NOT NULL,
 body TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_project ON messages(project_id, created_at);
CREATE TABLE IF NOT EXISTS assets (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 name TEXT NOT NULL, kind TEXT NOT NULL, path TEXT NOT NULL, size INTEGER NOT NULL,
 created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS secrets (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, name TEXT NOT NULL,
 nonce BLOB NOT NULL, ciphertext BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 PRIMARY KEY(user_id,name)
);
CREATE TABLE IF NOT EXISTS workflow_runs (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 name TEXT NOT NULL, group_name TEXT NOT NULL, status TEXT NOT NULL,
 current_step TEXT NOT NULL DEFAULT '', execution_id TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_project ON workflow_runs(project_id, updated_at DESC);
CREATE TABLE IF NOT EXISTS presented_videos (
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 path TEXT NOT NULL, title TEXT NOT NULL, note TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, PRIMARY KEY(project_id,path)
);
CREATE TABLE IF NOT EXISTS workflow_step_runs (
 workflow_run_id TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
 step_id TEXT NOT NULL, title TEXT NOT NULL, position INTEGER NOT NULL, status TEXT NOT NULL,
 updated_at TEXT NOT NULL, PRIMARY KEY(workflow_run_id,step_id)
);
DELETE FROM messages
 WHERE role='assistant'
   AND author='Studio agent'
   AND body LIKE 'Your % workspace is ready. Tell me about the first video you want to create, or upload any references you already have.';`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func (s *Store) StartWorkflowRun(projectID, name, groupName string, steps []WorkflowStep) (WorkflowRun, error) {
	now := time.Now().UTC()
	run := WorkflowRun{ID: uuid.NewString(), ProjectID: projectID, Name: name, GroupName: groupName, Status: "running", Steps: steps, CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.Begin()
	if err != nil {
		return WorkflowRun{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO workflow_runs(id,project_id,name,group_name,status,current_step,execution_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, projectID, name, groupName, run.Status, "", "", dbTime(now), dbTime(now)); err != nil {
		return WorkflowRun{}, err
	}
	for _, step := range steps {
		status := step.Status
		if status == "" {
			status = "pending"
		}
		if _, err = tx.Exec(`INSERT INTO workflow_step_runs(workflow_run_id,step_id,title,position,status,updated_at) VALUES(?,?,?,?,?,?)`, run.ID, step.ID, step.Title, step.Position, status, dbTime(now)); err != nil {
			return WorkflowRun{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return WorkflowRun{}, err
	}
	return run, nil
}

func (s *Store) BeginWorkflowRun(projectID, name, groupName string, steps []WorkflowStep) (WorkflowRun, error) {
	var run WorkflowRun
	var created, updated string
	err := s.db.QueryRow(`SELECT id,project_id,name,group_name,status,current_step,execution_id,created_at,updated_at FROM workflow_runs WHERE project_id=? AND group_name=? AND status='ready' ORDER BY updated_at DESC LIMIT 1`, projectID, groupName).Scan(
		&run.ID, &run.ProjectID, &run.Name, &run.GroupName, &run.Status, &run.CurrentStep, &run.ExecutionID, &created, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return s.StartWorkflowRun(projectID, name, groupName, steps)
	}
	if err != nil {
		return WorkflowRun{}, err
	}
	run.Status, run.CurrentStep, run.ExecutionID = "running", "", ""
	run.CreatedAt, run.UpdatedAt = parseTime(created), time.Now().UTC()
	if _, err := s.db.Exec(`UPDATE workflow_runs SET status='running',current_step='',execution_id='',updated_at=? WHERE id=?`, dbTime(run.UpdatedAt), run.ID); err != nil {
		return WorkflowRun{}, err
	}
	return run, nil
}

func (s *Store) SetWorkflowExecution(runID, executionID string) error {
	_, err := s.db.Exec(`UPDATE workflow_runs SET execution_id=?,updated_at=? WHERE id=?`, executionID, dbTime(time.Now()), runID)
	return err
}

func (s *Store) SetWorkflowStep(runID, stepID, status string) error {
	now := dbTime(time.Now())
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE workflow_step_runs SET status=?,updated_at=? WHERE workflow_run_id=? AND step_id=?`, status, now, runID, stepID); err != nil {
		return err
	}
	current := stepID
	if status == "completed" || status == "failed" || status == "cancelled" {
		current = ""
	}
	if _, err = tx.Exec(`UPDATE workflow_runs SET current_step=?,updated_at=? WHERE id=?`, current, now, runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FinishWorkflowRun(runID, status string) error {
	_, err := s.db.Exec(`UPDATE workflow_runs SET status=?,current_step='',updated_at=? WHERE id=?`, status, dbTime(time.Now()), runID)
	return err
}

func (s *Store) WorkflowRuns(userID, projectID string) ([]WorkflowRun, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,project_id,name,group_name,status,current_step,execution_id,created_at,updated_at FROM workflow_runs WHERE project_id=? ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	var runs []WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		var created, updated string
		if err := rows.Scan(&run.ID, &run.ProjectID, &run.Name, &run.GroupName, &run.Status, &run.CurrentStep, &run.ExecutionID, &created, &updated); err != nil {
			return nil, err
		}
		run.CreatedAt, run.UpdatedAt = parseTime(created), parseTime(updated)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range runs {
		stepRows, err := s.db.Query(`SELECT step_id,title,position,status FROM workflow_step_runs WHERE workflow_run_id=? ORDER BY position`, runs[index].ID)
		if err != nil {
			return nil, err
		}
		for stepRows.Next() {
			var step WorkflowStep
			if err := stepRows.Scan(&step.ID, &step.Title, &step.Position, &step.Status); err != nil {
				stepRows.Close()
				return nil, err
			}
			runs[index].Steps = append(runs[index].Steps, step)
		}
		if err := stepRows.Close(); err != nil {
			return nil, err
		}
		if runs[index].Steps == nil {
			runs[index].Steps = []WorkflowStep{}
		}
	}
	return runs, nil
}

func (s *Store) Close() error    { return s.db.Close() }
func (s *Store) DataDir() string { return s.dataDir }

func dbTime(t time.Time) string    { return t.UTC().Format(time.RFC3339Nano) }
func parseTime(v string) time.Time { t, _ := time.Parse(time.RFC3339Nano, v); return t }

func (s *Store) CreateUser(email, name string, passwordHash []byte) (User, error) {
	now := time.Now().UTC()
	u := User{ID: uuid.NewString(), Email: strings.ToLower(strings.TrimSpace(email)), Name: strings.TrimSpace(name), CreatedAt: now}
	if u.Name == "" {
		u.Name = strings.Split(u.Email, "@")[0]
	}
	_, err := s.db.Exec(`INSERT INTO users(id,email,name,password_hash,created_at) VALUES(?,?,?,?,?)`, u.ID, u.Email, u.Name, passwordHash, dbTime(now))
	if err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *Store) UserByEmail(email string) (User, []byte, error) {
	var u User
	var created string
	var hash []byte
	err := s.db.QueryRow(`SELECT id,email,name,password_hash,created_at FROM users WHERE email=?`, strings.ToLower(strings.TrimSpace(email))).Scan(&u.ID, &u.Email, &u.Name, &hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, nil, ErrNotFound
	}
	u.CreatedAt = parseTime(created)
	return u, hash, err
}

func (s *Store) CreateSession(userID, rawToken string, expires time.Time) error {
	h := sha256.Sum256([]byte(rawToken))
	_, err := s.db.Exec(`INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)`, hex.EncodeToString(h[:]), userID, dbTime(expires))
	return err
}

func (s *Store) UserBySession(rawToken string) (User, error) {
	h := sha256.Sum256([]byte(rawToken))
	var u User
	var created, expires string
	err := s.db.QueryRow(`SELECT u.id,u.email,u.name,u.created_at,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, hex.EncodeToString(h[:])).Scan(&u.ID, &u.Email, &u.Name, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if time.Now().After(parseTime(expires)) {
		_ = s.DeleteSession(rawToken)
		return User{}, ErrNotFound
	}
	u.CreatedAt = parseTime(created)
	return u, nil
}

func (s *Store) DeleteSession(rawToken string) error {
	h := sha256.Sum256([]byte(rawToken))
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, hex.EncodeToString(h[:]))
	return err
}

func (s *Store) ProjectDir(projectID string) string {
	return filepath.Join(s.dataDir, "projects", projectID)
}

func (s *Store) CreateProject(user User, title, description string) (Project, error) {
	now := time.Now().UTC()
	p := Project{ID: uuid.NewString(), Title: strings.TrimSpace(title), Description: strings.TrimSpace(description), OwnerID: user.ID, Role: "owner", Status: "active", SessionStatus: "ready", CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.Begin()
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO projects(id,title,description,owner_id,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, p.ID, p.Title, p.Description, p.OwnerID, p.Status, dbTime(now), dbTime(now)); err != nil {
		return Project{}, err
	}
	if _, err = tx.Exec(`INSERT INTO project_members(project_id,user_id,role,created_at) VALUES(?,?,?,?)`, p.ID, user.ID, "owner", dbTime(now)); err != nil {
		return Project{}, err
	}
	if err = tx.Commit(); err != nil {
		return Project{}, err
	}
	for _, dir := range []string{"uploads", "outputs", "work"} {
		if err := os.MkdirAll(filepath.Join(s.ProjectDir(p.ID), dir), 0700); err != nil {
			return Project{}, fmt.Errorf("create project workspace: %w", err)
		}
	}
	return p, nil
}

func scanProject(scanner interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var created, updated string
	err := scanner.Scan(&p.ID, &p.Title, &p.Description, &p.OwnerID, &p.Role, &p.Status, &created, &updated)
	p.CreatedAt, p.UpdatedAt, p.SessionStatus = parseTime(created), parseTime(updated), "ready"
	return p, err
}

const projectSelect = `SELECT p.id,p.title,p.description,p.owner_id,pm.role,p.status,p.created_at,p.updated_at FROM projects p JOIN project_members pm ON pm.project_id=p.id`

func (s *Store) Projects(userID string) ([]Project, error) {
	rows, err := s.db.Query(projectSelect+` WHERE pm.user_id=? AND p.status='active' ORDER BY p.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Project(userID, projectID string) (Project, error) {
	p, err := scanProject(s.db.QueryRow(projectSelect+` WHERE pm.user_id=? AND p.id=?`, userID, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (s *Store) UpdateProject(userID, projectID, title, description string) (Project, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return Project{}, err
	}
	_, err := s.db.Exec(`UPDATE projects SET title=?,description=?,updated_at=? WHERE id=?`, strings.TrimSpace(title), strings.TrimSpace(description), dbTime(time.Now()), projectID)
	if err != nil {
		return Project{}, err
	}
	return s.Project(userID, projectID)
}

// PresentVideo records a video the agent has chosen to show the user. Presenting
// is deliberate rather than discovered: a run leaves many video files behind —
// raw shots, silent intermediates, byte-identical delivery copies — and only the
// agent knows which one is the deliverable, because the delivery report names it.
func (s *Store) PresentVideo(projectID, path, title, note string) error {
	_, err := s.db.Exec(
		`INSERT INTO presented_videos(project_id,path,title,note,created_at) VALUES(?,?,?,?,?)
		 ON CONFLICT(project_id,path) DO UPDATE SET title=excluded.title,note=excluded.note`,
		projectID, path, title, note, dbTime(time.Now()))
	return err
}

// PresentedVideos returns the videos an agent explicitly surfaced, newest first.
func (s *Store) PresentedVideos(userID, projectID string) ([]PresentedVideo, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT path,title,note,created_at FROM presented_videos WHERE project_id=? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PresentedVideo
	for rows.Next() {
		var v PresentedVideo
		var created string
		if err := rows.Scan(&v.Path, &v.Title, &v.Note, &created); err != nil {
			return nil, err
		}
		v.CreatedAt = parseTime(created)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ArchiveProject(userID, projectID string) error {
	p, err := s.Project(userID, projectID)
	if err != nil {
		return err
	}
	if p.Role != "owner" {
		return errors.New("only the owner can archive a project")
	}
	_, err = s.db.Exec(`UPDATE projects SET status='archived',updated_at=? WHERE id=?`, dbTime(time.Now()), projectID)
	return err
}

func (s *Store) Messages(userID, projectID string) ([]Message, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,project_id,COALESCE(user_id,''),role,author,body,created_at FROM messages WHERE project_id=? ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &m.Author, &m.Body, &created); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMessage(projectID, userID, role, author, body string) (Message, error) {
	now := time.Now().UTC()
	m := Message{ID: uuid.NewString(), ProjectID: projectID, UserID: userID, Role: role, Author: author, Body: body, CreatedAt: now}
	var uid any = userID
	if userID == "" {
		uid = nil
	}
	_, err := s.db.Exec(`INSERT INTO messages(id,project_id,user_id,role,author,body,created_at) VALUES(?,?,?,?,?,?,?)`, m.ID, projectID, uid, role, author, body, dbTime(now))
	if err == nil {
		_, err = s.db.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, dbTime(now), projectID)
	}
	return m, err
}

func (s *Store) SessionHandle(userID, projectID string) ([]byte, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return nil, err
	}
	var data []byte
	err := s.db.QueryRow(`SELECT session_handle FROM projects WHERE id=?`, projectID).Scan(&data)
	return data, err
}
func (s *Store) SaveSessionHandle(projectID string, data []byte) error {
	_, err := s.db.Exec(`UPDATE projects SET session_handle=? WHERE id=?`, data, projectID)
	return err
}

func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." {
		return "asset"
	}
	return name
}

func assetKind(contentType, name string) string {
	v := strings.ToLower(contentType + " " + filepath.Ext(name))
	switch {
	case strings.Contains(v, "image/") || strings.Contains(v, ".png") || strings.Contains(v, ".jpg") || strings.Contains(v, ".jpeg") || strings.Contains(v, ".webp"):
		return "image"
	case strings.Contains(v, "video/") || strings.Contains(v, ".mp4") || strings.Contains(v, ".mov") || strings.Contains(v, ".webm"):
		return "video"
	case strings.Contains(v, "audio/") || strings.Contains(v, ".mp3") || strings.Contains(v, ".wav"):
		return "audio"
	default:
		return "document"
	}
}

func (s *Store) SaveAsset(userID, projectID, name, contentType string, src io.Reader) (Asset, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return Asset{}, err
	}
	now := time.Now().UTC()
	a := Asset{ID: uuid.NewString(), ProjectID: projectID, Name: safeFilename(name), Kind: assetKind(contentType, name), CreatedAt: now}
	dir := filepath.Join(s.ProjectDir(projectID), "uploads")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Asset{}, err
	}
	rel := filepath.Join("uploads", a.ID+"-"+a.Name)
	dst, err := os.OpenFile(filepath.Join(s.ProjectDir(projectID), rel), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Asset{}, err
	}
	a.Size, err = io.Copy(dst, io.LimitReader(src, 2<<30))
	closeErr := dst.Close()
	if err != nil {
		return Asset{}, err
	}
	if closeErr != nil {
		return Asset{}, closeErr
	}
	_, err = s.db.Exec(`INSERT INTO assets(id,project_id,name,kind,path,size,created_at) VALUES(?,?,?,?,?,?,?)`, a.ID, a.ProjectID, a.Name, a.Kind, rel, a.Size, dbTime(now))
	return a, err
}

func (s *Store) Assets(userID, projectID string) ([]Asset, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,project_id,name,kind,size,created_at FROM assets WHERE project_id=? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		var created string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Kind, &a.Size, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AssetPath(userID, projectID, assetID string) (string, string, error) {
	if _, err := s.Project(userID, projectID); err != nil {
		return "", "", err
	}
	var rel, name string
	err := s.db.QueryRow(`SELECT path,name FROM assets WHERE project_id=? AND id=?`, projectID, assetID).Scan(&rel, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(s.ProjectDir(projectID), rel)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.ProjectDir(projectID))+string(os.PathSeparator)) {
		return "", "", errors.New("invalid asset path")
	}
	return path, name, nil
}

func (s *Store) DeleteAsset(userID, projectID, assetID string) error {
	path, _, err := s.AssetPath(userID, projectID, assetID)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec(`DELETE FROM assets WHERE project_id=? AND id=?`, projectID, assetID); err != nil {
		return err
	}
	return os.Remove(path)
}
