package workspace

import (
	"context"
	"encoding/json"
	"fmt"
)

type CreateWorkflowDatabaseBackupSnapshotParams struct {
	DBPath string `json:"db_path"`
}

type WorkflowDatabaseBackupSnapshotResult struct {
	SourceDBPath string `json:"source_db_path"`
	SnapshotPath string `json:"snapshot_path"`
	ChecksumPath string `json:"checksum_path"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"size_bytes"`
	CreatedAt    string `json:"created_at"`
	Integrity    string `json:"integrity"`
}

type workflowDatabaseBackupSnapshotAPIResponse struct {
	Success bool                                 `json:"success"`
	Message string                               `json:"message,omitempty"`
	Data    WorkflowDatabaseBackupSnapshotResult `json:"data,omitempty"`
	Error   string                               `json:"error,omitempty"`
}

// CreateWorkflowDatabaseBackupSnapshot calls the trusted workspace-service
// operation that materializes a WAL-aware, integrity-checked backup image at a
// platform-owned path. The caller cannot choose the destination.
func (c *Client) CreateWorkflowDatabaseBackupSnapshot(ctx context.Context, params CreateWorkflowDatabaseBackupSnapshotParams) (WorkflowDatabaseBackupSnapshotResult, error) {
	if params.DBPath == "" {
		return WorkflowDatabaseBackupSnapshotResult{}, fmt.Errorf("db_path is required")
	}
	body, err := c.request(ctx, "POST", "/api/db/backup-snapshot", params)
	if err != nil {
		return WorkflowDatabaseBackupSnapshotResult{}, err
	}
	var response workflowDatabaseBackupSnapshotAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return WorkflowDatabaseBackupSnapshotResult{}, fmt.Errorf("decode workflow database backup snapshot response: %w", err)
	}
	if !response.Success {
		if response.Error != "" {
			return WorkflowDatabaseBackupSnapshotResult{}, fmt.Errorf("workflow database backup snapshot failed: %s", response.Error)
		}
		return WorkflowDatabaseBackupSnapshotResult{}, fmt.Errorf("workflow database backup snapshot failed: %s", response.Message)
	}
	return response.Data, nil
}
