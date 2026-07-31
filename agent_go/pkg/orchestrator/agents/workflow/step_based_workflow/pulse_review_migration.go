package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
)

const legacyPulseReviewImportsSchema = `CREATE TABLE IF NOT EXISTS pulse_review_artifact_imports (
	legacy_path TEXT PRIMARY KEY,
	content_sha256 TEXT NOT NULL,
	imported_at TEXT NOT NULL
)`

type PulseReviewArtifactMigrationResult struct {
	FilesFound          int      `json:"files_found"`
	ReviewArtifacts     int      `json:"review_artifacts"`
	AuxiliaryArtifacts  int      `json:"auxiliary_artifacts"`
	ConcernOccurrences  int      `json:"concern_occurrences"`
	AlreadyImported     int      `json:"already_imported"`
	FilesRetained       int      `json:"files_retained"`
	UnrecognizedSkipped []string `json:"unrecognized_skipped,omitempty"`
}

func pulseReviewHeaderValue(markdown, label string) string {
	prefix := "- " + strings.ToLower(strings.TrimSpace(label)) + ":"
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			continue
		}
		return strings.Trim(strings.TrimSpace(trimmed[len(prefix):]), "`* ")
	}
	return ""
}

func pulseReviewRecordedAt(markdown, path string) string {
	if value := pulseReviewHeaderValue(markdown, "Completed at"); value != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano)
		}
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func legacyPulseReviewIdentity(path string) (module, artifactKind string, ok bool) {
	name := strings.ToLower(filepath.Base(path))
	artifactKind = "review"
	switch {
	case strings.HasSuffix(name, ".packet.md"):
		module = strings.TrimSuffix(name, ".packet.md")
		artifactKind = "packet"
	case strings.HasSuffix(name, ".md"):
		module = strings.TrimSuffix(name, ".md")
	default:
		return "", "", false
	}
	module = pulsemodules.Normalize(module)
	if !pulsemodules.IsValid(module) {
		return "", "", false
	}
	return module, artifactKind, true
}

// ImportLegacyPulseReviewArtifacts migrates the old pulse/reviews/**/*.md
// transport into SQLite. Full Markdown is retained byte-for-byte in
// pulse_review_log; only explicit CONCERNS lines become open lifecycle rows.
// Source files are intentionally retained during the compatibility phase. A
// later version may remove them only after DB/UI read parity is proven.
func ImportLegacyPulseReviewArtifacts(ctx context.Context, workspacePath string) (PulseReviewArtifactMigrationResult, error) {
	result := PulseReviewArtifactMigrationResult{UnrecognizedSkipped: []string{}}
	workflowRoot := filepath.Dir(filepath.Dir(runConcernsDBPath(workspacePath)))
	reviewsRoot := filepath.Join(workflowRoot, "pulse", "reviews")
	if _, err := os.Stat(reviewsRoot); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}

	var files []string
	if err := filepath.WalkDir(reviewsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return result, err
	}
	sort.Strings(files)
	result.FilesFound = len(files)

	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil {
		return result, err
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return result, err
	}
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return result, err
	}
	if _, err := db.ExecContext(ctx, legacyPulseReviewImportsSchema); err != nil {
		return result, err
	}

	for _, path := range files {
		relativePath, err := filepath.Rel(workflowRoot, path)
		if err != nil {
			return result, err
		}
		relativePath = filepath.ToSlash(relativePath)
		module, artifactKind, ok := legacyPulseReviewIdentity(path)
		if !ok {
			result.UnrecognizedSkipped = append(result.UnrecognizedSkipped, relativePath)
			continue
		}
		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return result, fmt.Errorf("read legacy Pulse review %s: %w", relativePath, err)
		}
		markdown := string(contentBytes)
		sum := sha256.Sum256(contentBytes)
		contentSHA := hex.EncodeToString(sum[:])

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return result, err
		}
		var existingSHA string
		err = tx.QueryRowContext(ctx, `SELECT content_sha256 FROM pulse_review_artifact_imports WHERE legacy_path=?`, relativePath).Scan(&existingSHA)
		if err == nil {
			tx.Rollback()
			if existingSHA != contentSHA {
				return result, fmt.Errorf("legacy Pulse review %s changed after it was imported", relativePath)
			}
			result.AlreadyImported++
			result.FilesRetained++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			tx.Rollback()
			return result, err
		}

		reviewRunID := pulseReviewHeaderValue(markdown, "Review run")
		if reviewRunID == "" {
			reviewRunID = filepath.Base(filepath.Dir(path))
		}
		pulseRunID := pulseReviewHeaderValue(markdown, "Pulse run")
		status := pulseReviewHeaderValue(markdown, "Status")
		recordedAt := pulseReviewRecordedAt(markdown, path)
		if err := recordPulseReviewOnDB(
			ctx, tx, module, reviewRunID, pulseRunID, artifactKind,
			relativePath, status, markdown, recordedAt,
		); err != nil {
			tx.Rollback()
			return result, err
		}
		runIdentity := pulseRunID
		if runIdentity == "" {
			runIdentity = reviewRunID
		}
		concernLines := ParseConcernLines(markdown)
		recordedConcerns, err := recordRunConcernLinesAt(
			ctx, tx, runIdentity, "", module, ConcernPhaseReview, concernLines, recordedAt,
		)
		if err != nil {
			tx.Rollback()
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_review_artifact_imports
			(legacy_path, content_sha256, imported_at) VALUES (?, ?, ?)`,
			relativePath, contentSHA, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		if artifactKind == "review" {
			result.ReviewArtifacts++
		} else {
			result.AuxiliaryArtifacts++
		}
		result.ConcernOccurrences += recordedConcerns
		result.FilesRetained++
	}
	return result, nil
}
