package server

import (
	"context"
	"encoding/json"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	migratedEvaluationScoreWorkspaces sync.Map
	evaluationScoreMigrationMu        sync.Mutex
)

type evaluationScoreDailyFile struct {
	Date        string                             `json:"date"`
	GroupFolder string                             `json:"group_folder"`
	UpdatedAt   time.Time                          `json:"updated_at"`
	Evaluations map[string]*storedEvaluationReport `json:"evaluations,omitempty"`
	RunFolders  map[string]*EvaluationReport       `json:"run_folders,omitempty"`
}

type storedEvaluationReport struct {
	RunFolder         string            `json:"run_folder"`
	ArchivedRunFolder string            `json:"archived_run_folder,omitempty"`
	Report            *EvaluationReport `json:"report"`
}

func (r *storedEvaluationReport) effectiveRunFolder() string {
	if r == nil {
		return ""
	}
	if strings.TrimSpace(r.ArchivedRunFolder) != "" {
		return r.ArchivedRunFolder
	}
	return r.RunFolder
}

func ensureWorkspaceEvaluationScoreMigration(ctx context.Context, workspacePath string) error {
	normalized := normalizeWorkspacePath(workspacePath)
	if normalized == "" {
		return nil
	}
	if _, loaded := migratedEvaluationScoreWorkspaces.Load(normalized); loaded {
		return nil
	}

	evaluationScoreMigrationMu.Lock()
	defer evaluationScoreMigrationMu.Unlock()

	if _, loaded := migratedEvaluationScoreWorkspaces.Load(normalized); loaded {
		return nil
	}

	if err := migrateLegacyEvaluationReports(ctx, normalized); err != nil {
		return err
	}

	migratedEvaluationScoreWorkspaces.Store(normalized, struct{}{})
	return nil
}

func evaluationScoreGroupFolder(runFolder string) string {
	cleaned := strings.Trim(pathpkg.Clean(strings.TrimSpace(runFolder)), "/")
	if cleaned == "" || cleaned == "." {
		return "__ungrouped__"
	}

	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}

	return "__ungrouped__"
}

func evaluationScoreDateKey(ts time.Time) string {
	return ts.UTC().Format("2006-01-02")
}

func evaluationScoreGeneratedAt(report *EvaluationReport) time.Time {
	if report != nil {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(report.GeneratedAt)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func resolveEvaluationScorePath(workspacePath, runFolder string, ts time.Time) string {
	return workspaceCostPath(
		workspacePath,
		"scores",
		"evaluation",
		evaluationScoreGroupFolder(runFolder),
		evaluationScoreDateKey(ts)+".json",
	)
}

func migrateLegacyEvaluationReports(ctx context.Context, workspacePath string) error {
	root := workspaceCostPath(workspacePath, "evaluation", "runs")
	filePaths, err := listWorkspaceFilesRecursive(ctx, root)
	if err != nil {
		return err
	}

	for _, filePath := range filePaths {
		if pathpkg.Base(filePath) != "evaluation_report.json" {
			continue
		}

		content, exists, err := readFileFromWorkspace(ctx, filePath)
		if err != nil {
			return err
		}
		if !exists || strings.TrimSpace(content) == "" {
			continue
		}

		var report EvaluationReport
		if err := json.Unmarshal([]byte(content), &report); err != nil {
			continue
		}

		runFolder := strings.TrimPrefix(pathpkg.Dir(filePath), root+"/")
		if runFolder == pathpkg.Dir(filePath) || runFolder == "." || strings.TrimSpace(runFolder) == "" {
			continue
		}

		if err := persistEvaluationReportToScores(ctx, workspacePath, runFolder, &report); err != nil {
			return err
		}
	}

	return nil
}

func persistEvaluationReportToScores(ctx context.Context, workspacePath, runFolder string, report *EvaluationReport) error {
	if report == nil {
		return nil
	}

	persistAt := evaluationScoreGeneratedAt(report)
	targetPath := resolveEvaluationScorePath(workspacePath, runFolder, persistAt)
	dailyFile := &evaluationScoreDailyFile{
		Date:        evaluationScoreDateKey(persistAt),
		GroupFolder: evaluationScoreGroupFolder(runFolder),
		UpdatedAt:   time.Now().UTC(),
		Evaluations: make(map[string]*storedEvaluationReport),
		RunFolders:  make(map[string]*EvaluationReport),
	}

	if existingContent, exists, readErr := readFileFromWorkspace(ctx, targetPath); readErr == nil && exists && strings.TrimSpace(existingContent) != "" {
		var existing evaluationScoreDailyFile
		if err := json.Unmarshal([]byte(existingContent), &existing); err == nil {
			if existing.RunFolders == nil {
				existing.RunFolders = make(map[string]*EvaluationReport)
			}
			if existing.Evaluations == nil {
				existing.Evaluations = make(map[string]*storedEvaluationReport)
			}
			dailyFile = &existing
		}
	}

	dailyFile.Date = evaluationScoreDateKey(persistAt)
	dailyFile.GroupFolder = evaluationScoreGroupFolder(runFolder)
	dailyFile.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(report.EvaluationID) != "" {
		dailyFile.Evaluations[report.EvaluationID] = &storedEvaluationReport{RunFolder: runFolder, Report: report}
	} else {
		dailyFile.RunFolders[runFolder] = report
	}

	jsonData, err := json.MarshalIndent(dailyFile, "", "  ")
	if err != nil {
		return err
	}
	if err := writeRawFileToWorkspace(ctx, targetPath, string(jsonData)); err != nil {
		return err
	}

	return nil
}

func readAllEvaluationReportsFromScores(ctx context.Context, workspacePath string) (map[string]*storedEvaluationReport, error) {
	if err := ensureWorkspaceEvaluationScoreMigration(ctx, workspacePath); err != nil {
		return nil, err
	}

	root := workspaceCostPath(workspacePath, "scores", "evaluation")
	if !evaluationScoresRootExists(ctx, workspacePath) {
		return map[string]*storedEvaluationReport{}, nil
	}

	filePaths, err := listWorkspaceFilesRecursive(ctx, root)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*storedEvaluationReport)
	for _, filePath := range filePaths {
		if !strings.HasSuffix(filePath, ".json") {
			continue
		}

		content, exists, err := readFileFromWorkspace(ctx, filePath)
		if err != nil {
			return nil, err
		}
		if !exists || strings.TrimSpace(content) == "" {
			continue
		}

		var dailyFile evaluationScoreDailyFile
		if err := json.Unmarshal([]byte(content), &dailyFile); err != nil {
			continue
		}

		for evaluationID, stored := range dailyFile.Evaluations {
			if stored == nil || stored.Report == nil || strings.TrimSpace(evaluationID) == "" {
				continue
			}
			existing := result[evaluationID]
			if existing == nil || evaluationScoreGeneratedAt(stored.Report).After(evaluationScoreGeneratedAt(existing.Report)) {
				result[evaluationID] = stored
			}
		}

		for runFolder, report := range dailyFile.RunFolders {
			if report == nil {
				continue
			}
			legacyID := "legacy:" + runFolder + ":" + strings.TrimSpace(report.GeneratedAt)
			existing := result[legacyID]
			if existing == nil || evaluationScoreGeneratedAt(report).After(evaluationScoreGeneratedAt(existing.Report)) {
				result[legacyID] = &storedEvaluationReport{RunFolder: runFolder, Report: report}
			}
		}
	}

	return result, nil
}

func evaluationScoresRootExists(ctx context.Context, workspacePath string) bool {
	return workspacePathExists(ctx, filepath.ToSlash(workspaceCostPath(workspacePath, "scores", "evaluation")))
}
