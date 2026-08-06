package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// EvaluationScoreDailyFile persists evaluation reports outside the rotating
// evaluation/runs tree so scores survive iteration pruning.
type EvaluationScoreDailyFile struct {
	Date        string    `json:"date"`
	GroupFolder string    `json:"group_folder"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Evaluations is the authoritative immutable ledger. RunFolders remains a
	// read-compatible v1 projection for existing workspaces.
	Evaluations map[string]*StoredEvaluationReport `json:"evaluations,omitempty"`
	RunFolders  map[string]*EvaluationReport       `json:"run_folders,omitempty"`
}

type StoredEvaluationReport struct {
	RunFolder         string            `json:"run_folder"`
	ArchivedRunFolder string            `json:"archived_run_folder,omitempty"`
	Report            *EvaluationReport `json:"report"`
}

func evaluationScoreDateKey(ts time.Time) string {
	return ts.UTC().Format("2006-01-02")
}

func evaluationScoreGroupFolder(runFolder string) string {
	cleaned := strings.Trim(filepath.ToSlash(filepath.Clean(strings.TrimSpace(runFolder))), "/")
	if cleaned == "" || cleaned == "." {
		return "__ungrouped__"
	}

	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}

	return "__ungrouped__"
}

func evaluationScoreGeneratedAt(report *EvaluationReport) time.Time {
	if report != nil {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(report.GeneratedAt)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func resolveEvaluationScoreDailyPath(workspacePath, runFolder string, ts time.Time) string {
	return filepath.Join(
		workspacePath,
		"scores",
		"evaluation",
		evaluationScoreGroupFolder(runFolder),
		evaluationScoreDateKey(ts)+".json",
	)
}

func (hcpo *StepBasedWorkflowOrchestrator) persistEvaluationScoreLedger(ctx context.Context, report *EvaluationReport, runFolder string) error {
	if report == nil {
		return nil
	}

	persistAt := evaluationScoreGeneratedAt(report)
	ledgerPath := resolveEvaluationScoreDailyPath(hcpo.GetWorkspacePath(), runFolder, persistAt)
	dailyFile := &EvaluationScoreDailyFile{
		Date:        evaluationScoreDateKey(persistAt),
		GroupFolder: evaluationScoreGroupFolder(runFolder),
		UpdatedAt:   time.Now().UTC(),
		Evaluations: make(map[string]*StoredEvaluationReport),
		RunFolders:  make(map[string]*EvaluationReport),
	}

	if existingContent, err := hcpo.ReadWorkspaceFile(ctx, ledgerPath); err == nil && strings.TrimSpace(existingContent) != "" {
		var existing EvaluationScoreDailyFile
		if err := json.Unmarshal([]byte(existingContent), &existing); err == nil {
			if strings.TrimSpace(existing.Date) == "" {
				existing.Date = dailyFile.Date
			}
			if strings.TrimSpace(existing.GroupFolder) == "" {
				existing.GroupFolder = dailyFile.GroupFolder
			}
			if existing.RunFolders == nil {
				existing.RunFolders = make(map[string]*EvaluationReport)
			}
			if existing.Evaluations == nil {
				existing.Evaluations = make(map[string]*StoredEvaluationReport)
			}
			dailyFile = &existing
		}
	}

	dailyFile.Date = evaluationScoreDateKey(persistAt)
	dailyFile.GroupFolder = evaluationScoreGroupFolder(runFolder)
	dailyFile.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(report.EvaluationID) != "" {
		dailyFile.Evaluations[report.EvaluationID] = &StoredEvaluationReport{RunFolder: runFolder, Report: report}
	} else {
		// Reports created before the immutable-id contract remain readable.
		dailyFile.RunFolders[runFolder] = report
	}

	jsonData, err := json.MarshalIndent(dailyFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal evaluation score ledger: %w", err)
	}
	if err := hcpo.WriteWorkspaceFile(ctx, ledgerPath, string(jsonData)); err != nil {
		return fmt.Errorf("failed to persist evaluation score ledger: %w", err)
	}
	return nil
}

// archiveEvaluationScoreRunFolder updates the display path of immutable
// evaluation records after iteration-0 rotates. The report itself and its
// evaluation ID remain unchanged.
func (hcpo *StepBasedWorkflowOrchestrator) archiveEvaluationScoreRunFolder(ctx context.Context, fromRunFolder, toRunFolder string) error {
	fromRunFolder = strings.Trim(filepath.ToSlash(filepath.Clean(strings.TrimSpace(fromRunFolder))), "/")
	toRunFolder = strings.Trim(filepath.ToSlash(filepath.Clean(strings.TrimSpace(toRunFolder))), "/")
	if fromRunFolder == "" || fromRunFolder == "." || toRunFolder == "" || toRunFolder == "." || fromRunFolder == toRunFolder {
		return nil
	}

	root := filepath.Join(hcpo.GetWorkspacePath(), "scores", "evaluation")
	groups, err := hcpo.ListWorkspaceDirectories(ctx, root)
	if err != nil {
		return nil // no score ledger exists yet
	}
	for _, group := range groups {
		dir := filepath.Join(root, filepath.Base(strings.TrimSpace(group)))
		files, err := hcpo.ListWorkspaceFiles(ctx, dir)
		if err != nil {
			continue
		}
		for _, name := range files {
			name = filepath.Base(strings.TrimSpace(name))
			if filepath.Ext(name) != ".json" {
				continue
			}
			path := filepath.Join(dir, name)
			content, err := hcpo.ReadWorkspaceFile(ctx, path)
			if err != nil || strings.TrimSpace(content) == "" {
				continue
			}
			var daily EvaluationScoreDailyFile
			if err := json.Unmarshal([]byte(content), &daily); err != nil {
				continue
			}
			changed := false
			for _, record := range daily.Evaluations {
				if record == nil || strings.TrimSpace(record.ArchivedRunFolder) != "" {
					continue
				}
				if archived := archivedEvaluationRunFolder(record.RunFolder, fromRunFolder, toRunFolder); archived != "" {
					record.ArchivedRunFolder = archived
					changed = true
				}
			}
			if !changed {
				continue
			}
			daily.UpdatedAt = time.Now().UTC()
			encoded, err := json.MarshalIndent(&daily, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal archived evaluation ledger %s: %w", path, err)
			}
			if err := hcpo.WriteWorkspaceFile(ctx, path, string(encoded)); err != nil {
				return fmt.Errorf("update archived evaluation ledger %s: %w", path, err)
			}
		}
	}
	return nil
}

func archivedEvaluationRunFolder(runFolder, fromRunFolder, toRunFolder string) string {
	runFolder = strings.Trim(filepath.ToSlash(filepath.Clean(strings.TrimSpace(runFolder))), "/")
	if runFolder == fromRunFolder {
		return toRunFolder
	}
	if strings.HasPrefix(runFolder, fromRunFolder+"/") {
		return toRunFolder + strings.TrimPrefix(runFolder, fromRunFolder)
	}
	return ""
}
