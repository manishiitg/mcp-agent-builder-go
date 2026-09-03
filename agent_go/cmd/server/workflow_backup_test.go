package server

import (
	"slices"
	"testing"
)

func TestWorkflowBackupHashIncludesManagedDatabaseSnapshot(t *testing.T) {
	if !slices.Contains(backupHashFiles, "backup/database/db.sqlite.sha256") {
		t.Fatal("managed database checksum is absent from the backup freshness hash")
	}
	if slices.Contains(backupHashFiles, "db/db.sqlite") {
		t.Fatal("backup freshness hash must never read protected live db/db.sqlite")
	}
}

func TestWorkflowBackupEffectiveState(t *testing.T) {
	remoteConfig := func() *WorkflowBackupConfig {
		return &WorkflowBackupConfig{
			Enabled: true,
			Destinations: []WorkflowBackupDestination{{
				ID: "config-repo", Type: "git", Provider: "github", Repo: "owner/workflow",
			}},
		}
	}

	tests := []struct {
		name   string
		config *WorkflowBackupConfig
		status *WorkflowBackupStatus
		want   string
	}{
		{
			name:   "missing config",
			config: nil,
			status: nil,
			want:   workflowBackupStateNotConfigured,
		},
		{
			name:   "disabled config",
			config: &WorkflowBackupConfig{Enabled: false},
			status: &WorkflowBackupStatus{State: workflowBackupStateHealthy},
			want:   workflowBackupStateNotConfigured,
		},
		{
			name:   "enabled without off-device destination",
			config: &WorkflowBackupConfig{Enabled: true},
			status: nil,
			want:   workflowBackupStateLocalOnly,
		},
		{
			name: "local git bundle stays local only",
			config: &WorkflowBackupConfig{
				Enabled: true,
				Destinations: []WorkflowBackupDestination{{
					ID: "local-git", Type: "git-bundle", Provider: "local",
				}},
			},
			status: &WorkflowBackupStatus{State: workflowBackupStateHealthy},
			want:   workflowBackupStateLocalOnly,
		},
		{
			name:   "remote enabled without status",
			config: remoteConfig(),
			status: nil,
			want:   workflowBackupStateConfiguredNotVerified,
		},
		{
			name:   "healthy stays healthy even after files change",
			config: remoteConfig(),
			status: &WorkflowBackupStatus{State: workflowBackupStateHealthy, LastSourceHash: "old-hash"},
			want:   workflowBackupStateHealthy,
		},
		{
			name:   "failed stays failed",
			config: remoteConfig(),
			status: &WorkflowBackupStatus{State: workflowBackupStateFailed, LastSourceHash: "old-hash"},
			want:   workflowBackupStateFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workflowBackupEffectiveState(tt.config, tt.status)
			if got != tt.want {
				t.Fatalf("workflowBackupEffectiveState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkflowBackupHasRemoteDestination(t *testing.T) {
	tests := []struct {
		name        string
		destination WorkflowBackupDestination
		want        bool
	}{
		{name: "github", destination: WorkflowBackupDestination{Type: "git", Provider: "github", Repo: "owner/repo"}, want: true},
		{name: "s3", destination: WorkflowBackupDestination{Type: "object_store", Provider: "s3", Bucket: "backups"}, want: true},
		{name: "custom remote", destination: WorkflowBackupDestination{Type: "sync", Provider: "webdav"}, want: true},
		{name: "local bundle", destination: WorkflowBackupDestination{Type: "git-bundle", Provider: "local"}, want: false},
		{name: "local zip", destination: WorkflowBackupDestination{Type: "local_zip", Provider: "local"}, want: false},
		{name: "local copy", destination: WorkflowBackupDestination{Type: "local-copy"}, want: false},
		{name: "generic git without remote", destination: WorkflowBackupDestination{Type: "git", Provider: "git"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &WorkflowBackupConfig{Enabled: true, Destinations: []WorkflowBackupDestination{tt.destination}}
			if got := workflowBackupHasRemoteDestination(config); got != tt.want {
				t.Fatalf("workflowBackupHasRemoteDestination() = %v, want %v", got, tt.want)
			}
		})
	}
}
