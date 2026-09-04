package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	_ "modernc.org/sqlite"
)

const orgDashboardHistoryLimitPerKind = 50

// OrgDashboardNotification is the durable, channel-neutral record rendered by
// the Organization page. It is written by notify_user through the
// org_dashboard connector, never by parsing an email or Slack payload.
type OrgDashboardNotification struct {
	ID            string                       `json:"id"`
	WorkspacePath string                       `json:"workspace_path"`
	Kind          string                       `json:"kind"`
	Title         string                       `json:"title,omitempty"`
	Status        string                       `json:"status"`
	Route         string                       `json:"route,omitempty"`
	Message       string                       `json:"message"`
	Fields        []NotificationSummaryField   `json:"fields,omitempty"`
	Sections      []NotificationSummarySection `json:"sections,omitempty"`
	CreatedAt     string                       `json:"created_at"`
}

type OrgDashboardWorkflowNotifications struct {
	WorkspacePath string                     `json:"workspace_path"`
	RunSummary    *OrgDashboardNotification  `json:"run_summary,omitempty"`
	PulseSummary  *OrgDashboardNotification  `json:"pulse_summary,omitempty"`
	Recent        []OrgDashboardNotification `json:"recent,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

// OrgDashboardConnector is an always-on internal notification connector. It
// persists run_summary and pulse_summary calls even when every external
// channel is disabled or fails.
type OrgDashboardConnector struct {
	mu sync.Mutex
}

func NewOrgDashboardConnector() *OrgDashboardConnector { return &OrgDashboardConnector{} }

func (c *OrgDashboardConnector) Name() string    { return "org_dashboard" }
func (c *OrgDashboardConnector) IsEnabled() bool { return true }

// Blocking feedback requests are not Org Dashboard events.
func (c *OrgDashboardConnector) SendNotification(context.Context, string, string, string, *ButtonOptions, *NotificationDestination) (string, error) {
	return "", nil
}

func (c *OrgDashboardConnector) SendUserNotification(ctx context.Context, message, _ string, dest *NotificationDestination) (string, error) {
	if dest == nil || dest.Content == nil || dest.Content.Summary == nil {
		return "", nil
	}
	summary := dest.Content.Summary
	kind := strings.ToLower(strings.TrimSpace(summary.Kind))
	if kind != "run_summary" && kind != "pulse_summary" {
		return "", nil
	}
	workspacePath, dbPath, err := orgDashboardDBPath(dest.WorkspacePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return "", fmt.Errorf("create workflow database folder: %w", err)
	}

	fields := notificationFieldsWithRoute(summary.Fields, summary.Route)
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	sectionsJSON, err := json.Marshal(summary.Sections)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	c.mu.Lock()
	defer c.mu.Unlock()
	db, err := openOrgDashboardDB(ctx, dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := ensureOrgDashboardSchema(ctx, db); err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO org_dashboard_notifications
		(id, notification_kind, title, status, message, fields_json, sections_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, kind, strings.TrimSpace(summary.Title), normalizedSummaryStatus(summary.Status), strings.TrimSpace(message), string(fieldsJSON), string(sectionsJSON), now,
	); err != nil {
		return "", fmt.Errorf("persist org dashboard notification for %s: %w", workspacePath, err)
	}
	// Retain each stream independently so frequent workflow-run notifications
	// cannot evict the latest (usually less frequent) Pulse summary, or vice versa.
	for _, retainedKind := range []string{"run_summary", "pulse_summary"} {
		_, _ = db.ExecContext(ctx, `
			DELETE FROM org_dashboard_notifications
			WHERE notification_kind = ? AND id NOT IN (
				SELECT id FROM org_dashboard_notifications
				WHERE notification_kind = ? ORDER BY created_at DESC LIMIT ?
			)`, retainedKind, retainedKind, orgDashboardHistoryLimitPerKind)
	}
	return id, nil
}

func normalizedSummaryStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "blocked", "waiting_for_user", "waiting_for_platform", "monitoring", "informational", "no_run":
		return strings.ToLower(strings.TrimSpace(status))
	// Older notifications used visual severity. Preserve their meaning as a
	// truthful workflow status when they are read or rewritten.
	case "success":
		return "completed"
	case "warning":
		return "blocked"
	case "danger":
		return "failed"
	default:
		return "informational"
	}
}

func orgDashboardDBPath(rawWorkspacePath string) (string, string, error) {
	workspacePath := filepath.ToSlash(filepath.Clean(strings.Trim(strings.TrimSpace(rawWorkspacePath), "/")))
	if workspacePath == "." || workspacePath == "" || filepath.IsAbs(workspacePath) {
		return "", "", fmt.Errorf("org_dashboard requires a workflow workspace path")
	}
	for _, part := range strings.Split(workspacePath, "/") {
		if part == ".." {
			return "", "", fmt.Errorf("workspace path escapes workspace root")
		}
	}
	root, err := filepath.Abs(fsutil.WorkspaceDocsRoot())
	if err != nil {
		return "", "", err
	}
	dbPath, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(workspacePath), "db", "db.sqlite"))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, dbPath)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("workspace path escapes workspace root")
	}
	return workspacePath, dbPath, nil
}

func openOrgDashboardDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureOrgDashboardSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS org_dashboard_notifications (
			id TEXT PRIMARY KEY,
			notification_kind TEXT NOT NULL CHECK (notification_kind IN ('run_summary', 'pulse_summary')),
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'informational',
			message TEXT NOT NULL,
			fields_json TEXT NOT NULL DEFAULT '[]',
			sections_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_org_dashboard_notifications_kind_created
		ON org_dashboard_notifications(notification_kind, created_at DESC);
	`)
	if err != nil {
		return fmt.Errorf("initialize org dashboard notification store: %w", err)
	}
	return nil
}

// ListOrgDashboardNotifications returns the newest durable notification of
// each supported kind plus a bounded recent history for one workflow.
func ListOrgDashboardNotifications(ctx context.Context, rawWorkspacePath string, recentLimit int) (OrgDashboardWorkflowNotifications, error) {
	workspacePath, dbPath, err := orgDashboardDBPath(rawWorkspacePath)
	if err != nil {
		return OrgDashboardWorkflowNotifications{}, err
	}
	result := OrgDashboardWorkflowNotifications{WorkspacePath: workspacePath}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	db, err := openOrgDashboardDB(ctx, dbPath)
	if err != nil {
		return result, err
	}
	defer db.Close()
	if err := ensureOrgDashboardSchema(ctx, db); err != nil {
		return result, err
	}
	if recentLimit <= 0 || recentLimit > 50 {
		recentLimit = 10
	}
	result.RunSummary, err = latestOrgDashboardNotification(ctx, db, workspacePath, "run_summary")
	if err != nil {
		return result, err
	}
	result.PulseSummary, err = latestOrgDashboardNotification(ctx, db, workspacePath, "pulse_summary")
	if err != nil {
		return result, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, notification_kind, title, status, message, fields_json, sections_json, created_at
		FROM org_dashboard_notifications
		ORDER BY created_at DESC LIMIT ?`, recentLimit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OrgDashboardNotification
		var fieldsJSON, sectionsJSON string
		item.WorkspacePath = workspacePath
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Status, &item.Message, &fieldsJSON, &sectionsJSON, &item.CreatedAt); err != nil {
			return result, err
		}
		_ = json.Unmarshal([]byte(fieldsJSON), &item.Fields)
		item.Route = notificationRouteFromFields(item.Fields)
		item.Fields = notificationFieldsWithoutRoute(item.Fields)
		_ = json.Unmarshal([]byte(sectionsJSON), &item.Sections)
		item.Status = normalizedSummaryStatus(item.Status)
		result.Recent = append(result.Recent, item)
	}
	return result, rows.Err()
}

func latestOrgDashboardNotification(ctx context.Context, db *sql.DB, workspacePath, kind string) (*OrgDashboardNotification, error) {
	var item OrgDashboardNotification
	var fieldsJSON, sectionsJSON string
	item.WorkspacePath = workspacePath
	err := db.QueryRowContext(ctx, `
		SELECT id, notification_kind, title, status, message, fields_json, sections_json, created_at
		FROM org_dashboard_notifications
		WHERE notification_kind = ?
		ORDER BY created_at DESC LIMIT 1`, kind,
	).Scan(&item.ID, &item.Kind, &item.Title, &item.Status, &item.Message, &fieldsJSON, &sectionsJSON, &item.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(fieldsJSON), &item.Fields)
	item.Route = notificationRouteFromFields(item.Fields)
	item.Fields = notificationFieldsWithoutRoute(item.Fields)
	_ = json.Unmarshal([]byte(sectionsJSON), &item.Sections)
	item.Status = normalizedSummaryStatus(item.Status)
	return &item, nil
}

func notificationFieldsWithRoute(fields []NotificationSummaryField, route string) []NotificationSummaryField {
	route = strings.TrimSpace(route)
	result := append([]NotificationSummaryField(nil), fields...)
	for i := range result {
		if strings.EqualFold(strings.TrimSpace(result[i].Label), "route") {
			if route != "" {
				result[i].Value = route
			}
			return result
		}
	}
	if route == "" {
		return result
	}
	return append([]NotificationSummaryField{{Label: "Route", Value: route}}, result...)
}

func notificationRouteFromFields(fields []NotificationSummaryField) string {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.Label), "route") {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func notificationFieldsWithoutRoute(fields []NotificationSummaryField) []NotificationSummaryField {
	result := make([]NotificationSummaryField, 0, len(fields))
	for _, field := range fields {
		if !strings.EqualFold(strings.TrimSpace(field.Label), "route") {
			result = append(result, field)
		}
	}
	return result
}
