package step_based_workflow

import (
	"context"
	"database/sql"
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

const legacyPulseReviewCleanupLedgerSchema = `CREATE TABLE IF NOT EXISTS pulse_review_artifact_imports (
	legacy_path TEXT PRIMARY KEY,
	content_sha256 TEXT NOT NULL DEFAULT '',
	imported_at TEXT NOT NULL DEFAULT ''
)`

// PulseLegacyReviewMigrationResult describes the one-time removal of the old
// pulse/reviews Markdown transport. Recognized review files become compact
// receipts plus lifecycle findings; evidence packets are redundant transport
// data and are removed. Unknown files are never deleted automatically.
type PulseLegacyReviewMigrationResult struct {
	FilesFound          int      `json:"files_found"`
	ReviewReceipts      int      `json:"review_receipts"`
	AuxiliaryFiles      int      `json:"auxiliary_files"`
	ConcernOccurrences  int      `json:"concern_occurrences"`
	FilesRemoved        int      `json:"files_removed"`
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

func legacyPulseReviewIdentity(path string) (module, kind string, ok bool) {
	name := strings.ToLower(filepath.Base(path))
	kind = "review"
	switch {
	case strings.HasSuffix(name, ".packet.md"):
		module = strings.TrimSuffix(name, ".packet.md")
		kind = "packet"
	case strings.HasSuffix(name, ".md"):
		module = strings.TrimSuffix(name, ".md")
	default:
		return "", "", false
	}
	module = pulsemodules.Normalize(module)
	if !pulsemodules.IsValid(module) {
		return "", "", false
	}
	return module, kind, true
}

// MigrateLegacyPulseReviews removes the obsolete pulse/reviews/**/*.md
// transport. Each recognized review is transactionally converted to a compact
// receipt and lifecycle findings before its source file is removed. Packet
// files contain only duplicated transport evidence and are removed directly.
// A failed conversion leaves its source file intact, making retry safe.
func MigrateLegacyPulseReviews(ctx context.Context, workspacePath string) (PulseLegacyReviewMigrationResult, error) {
	result := PulseLegacyReviewMigrationResult{UnrecognizedSkipped: []string{}}
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
	if _, err := db.ExecContext(ctx, legacyPulseReviewCleanupLedgerSchema); err != nil {
		return result, err
	}

	for _, path := range files {
		relativePath, err := filepath.Rel(workflowRoot, path)
		if err != nil {
			return result, err
		}
		relativePath = filepath.ToSlash(relativePath)
		module, kind, ok := legacyPulseReviewIdentity(path)
		if !ok {
			result.UnrecognizedSkipped = append(result.UnrecognizedSkipped, relativePath)
			continue
		}
		if kind == "packet" {
			if err := os.Remove(path); err != nil {
				return result, fmt.Errorf("remove legacy Pulse packet %s: %w", relativePath, err)
			}
			result.AuxiliaryFiles++
			result.FilesRemoved++
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return result, fmt.Errorf("read legacy Pulse review %s: %w", relativePath, err)
		}
		markdown := string(content)
		reviewRunID := pulseReviewHeaderValue(markdown, "Review run")
		if reviewRunID == "" {
			reviewRunID = filepath.Base(filepath.Dir(path))
		}
		originalReviewRunID := reviewRunID
		// Several retired reviewers now normalize into one current lane. Keep
		// each historical receipt distinct instead of letting the compact
		// receipt upsert collapse them onto one (module, review_run_id) pair.
		legacyName := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".md")
		pulseRunID := pulseReviewHeaderValue(markdown, "Pulse run")
		status := pulseReviewHeaderValue(markdown, "Status")
		recordedAt := pulseReviewRecordedAt(markdown, path)
		verifications, _ := extractLegacyPulseReviewVerifications(markdown)
		concernLines := ParseConcernLines(markdown)

		alreadyConverted, err := legacyPulseReviewAlreadyConverted(ctx, db, relativePath, originalReviewRunID, module, legacyName)
		if err != nil {
			return result, err
		}
		if alreadyConverted {
			if err := os.Remove(path); err != nil {
				return result, fmt.Errorf("remove already-migrated Pulse review %s: %w", relativePath, err)
			}
			result.FilesRemoved++
			continue
		}
		reviewRunID += "_legacy_" + legacyName

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return result, err
		}
		if err := recordPulseReviewOnDB(
			ctx, tx, module, reviewRunID, pulseRunID, extractReviewVerdict(markdown),
			status, len(concernLines), verifications, recordedAt,
		); err != nil {
			tx.Rollback()
			return result, err
		}
		runIdentity := pulseRunID
		if runIdentity == "" {
			runIdentity = reviewRunID
		}
		recordedConcerns, err := recordRunConcernLinesAt(
			ctx, tx, runIdentity, "", module, ConcernPhaseReview, concernLines, recordedAt,
		)
		if err != nil {
			tx.Rollback()
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		if err := os.Remove(path); err != nil {
			return result, fmt.Errorf("remove migrated Pulse review %s: %w", relativePath, err)
		}
		result.ReviewReceipts++
		result.ConcernOccurrences += recordedConcerns
		result.FilesRemoved++
	}

	removeEmptyPulseReviewDirs(reviewsRoot)
	if len(result.UnrecognizedSkipped) == 0 {
		if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS pulse_review_artifact_imports`); err != nil {
			return result, err
		}
	}
	return result, nil
}

func legacyPulseReviewAlreadyConverted(
	ctx context.Context,
	db *sql.DB,
	relativePath, reviewRunID, normalizedModule, legacyName string,
) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM pulse_review_artifact_imports WHERE legacy_path=? LIMIT 1`,
		relativePath,
	).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	legacyModule := strings.TrimSuffix(legacyName, ".packet")
	err = db.QueryRowContext(ctx, `SELECT 1 FROM pulse_review_log
		WHERE review_run_id=? AND (module=? OR module=?) LIMIT 1`,
		reviewRunID, normalizedModule, legacyModule,
	).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func removeEmptyPulseReviewDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir) // succeeds only when empty
	}
}
