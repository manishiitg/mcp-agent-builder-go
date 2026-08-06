package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"

	_ "modernc.org/sqlite"
)

// A durable record of which Pulse reviewers actually ran and what they concluded.
//
// Pulse Gate chooses which modules to review each cycle. It had no way to tell a
// module that keeps finding real problems from one that never finds anything,
// because nothing recorded the outcome — so the choice drifted to habit in one
// workflow (the same four modules every run) and to everything-at-once in another.
//
// The evidence that this was costing real findings: confida ran seven reviewers
// in one cycle. All seven wrote substantive artifacts to disk — the knowledgebase
// reviewer caught a half-finished migration where two live consumers were reading
// a deleted path and silently getting nothing. Every one of those seven left
// `last_result` empty in pulse_module_state, because recording the outcome depends
// on the Pulse Fixer calling record_pulse_result, and it did not. A cycle
// that found a live breakage left no trace for the next cycle to learn from.
//
// So this is written by Go at the moment the backend persists a reviewer artifact
// — the same choke point that files reviewer concerns. There is no call for an
// agent to skip.
//
// Kept separate from pulse_module_audit deliberately. That table records the
// FIXER's outcome for a module (done/changed/blocked). This records that a
// REVIEWER ran and what it concluded, which is a different fact: a review can
// find something real and still be followed by no fix at all.

const pulseReviewLogSchema = `CREATE TABLE IF NOT EXISTS pulse_review_log (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	module TEXT NOT NULL,
	review_run_id TEXT NOT NULL DEFAULT '',
	pulse_run_id TEXT NOT NULL DEFAULT '',
	verdict TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	artifact_kind TEXT NOT NULL DEFAULT 'review',
	artifact_path TEXT NOT NULL DEFAULT '',
	artifact_bytes INTEGER NOT NULL DEFAULT 0,
	artifact_markdown TEXT NOT NULL DEFAULT '',
	content_sha256 TEXT NOT NULL DEFAULT '',
	recorded_at TEXT NOT NULL
)`

const pulseReviewLogIndex = `CREATE INDEX IF NOT EXISTS idx_pulse_review_log_module ON pulse_review_log(module, recorded_at)`

// maxVerdictChars keeps one recorded verdict to a scannable length. The full text
// stays in the reviewer artifact; this is the index into it.
const maxVerdictChars = 400

// ModuleReviewHistory summarizes what one reviewer has been finding.
type ModuleReviewHistory struct {
	Module        string   `json:"module"`
	RunCount      int      `json:"run_count"`
	LastRanAt     string   `json:"last_ran_at,omitempty"`
	RecentVerdict []string `json:"recent_verdicts,omitempty"`
}

type PulseReviewArtifactRecord struct {
	ID               int64                           `json:"id"`
	Module           string                          `json:"module"`
	ReviewRunID      string                          `json:"review_run_id"`
	PulseRunID       string                          `json:"pulse_run_id,omitempty"`
	Verdict          string                          `json:"verdict,omitempty"`
	Status           string                          `json:"status,omitempty"`
	ArtifactKind     string                          `json:"artifact_kind"`
	LegacySourcePath string                          `json:"legacy_source_path,omitempty"`
	ArtifactBytes    int                             `json:"artifact_bytes"`
	ContentSHA256    string                          `json:"content_sha256,omitempty"`
	RecordedAt       string                          `json:"recorded_at"`
	Markdown         string                          `json:"markdown,omitempty"`
	Verifications    []PulseReviewVerificationResult `json:"verifications"`
	Metrics          *PulseAgentMetricRecord         `json:"metrics,omitempty"`
}

// PulseReviewVerificationResult is the reviewer's structured judgment about a
// prior changed_unverified attempt. The reviewer remains read-only; the single
// Fixer consumes this record and performs the lifecycle mutation.
type PulseReviewVerificationResult struct {
	FindingID   string   `json:"finding_id"`
	Fingerprint string   `json:"fingerprint"`
	AttemptID   string   `json:"attempt_id"`
	Verdict     string   `json:"verdict"`
	Expected    string   `json:"expected"`
	Observed    string   `json:"observed"`
	Evidence    []string `json:"evidence"`
	NextCheck   string   `json:"next_check,omitempty"`
}

const pulseVerificationMarker = "PULSE_VERIFICATION_JSON:"
const pulseReviewStatusContractFailed = "contract_failed"

func ExtractPulseReviewVerifications(artifact string) ([]PulseReviewVerificationResult, error) {
	results := []PulseReviewVerificationResult{}
	seen := map[string]bool{}
	for lineNumber, raw := range strings.Split(artifact, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, pulseVerificationMarker) {
			continue
		}
		var result PulseReviewVerificationResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, pulseVerificationMarker))), &result); err != nil {
			return nil, fmt.Errorf("verification marker line %d is invalid JSON: %w", lineNumber+1, err)
		}
		result.FindingID = strings.TrimSpace(result.FindingID)
		result.Fingerprint = strings.TrimSpace(result.Fingerprint)
		result.AttemptID = strings.TrimSpace(result.AttemptID)
		result.Verdict = strings.TrimSpace(result.Verdict)
		result.Expected = strings.TrimSpace(result.Expected)
		result.Observed = strings.TrimSpace(result.Observed)
		result.NextCheck = strings.TrimSpace(result.NextCheck)
		result.Evidence = normalizedLifecycleStrings(result.Evidence)
		if result.FindingID == "" || result.Fingerprint == "" || result.AttemptID == "" || result.Expected == "" || result.Observed == "" {
			return nil, fmt.Errorf("verification marker line %d requires finding_id, fingerprint, attempt_id, expected, and observed", lineNumber+1)
		}
		switch result.Verdict {
		case VerificationPassed, VerificationFailed:
		case VerificationInconclusive:
			if result.NextCheck == "" {
				return nil, fmt.Errorf("inconclusive verification marker line %d requires next_check", lineNumber+1)
			}
		default:
			return nil, fmt.Errorf("verification marker line %d verdict must be passed, failed, or inconclusive", lineNumber+1)
		}
		key := result.Fingerprint + "\x00" + result.AttemptID
		if seen[key] {
			return nil, fmt.Errorf("duplicate verification marker for finding %q attempt %q", result.FindingID, result.AttemptID)
		}
		seen[key] = true
		results = append(results, result)
	}
	return results, nil
}

func ensurePulseReviewLogSchema(ctx context.Context, db pulseFindingLifecycleDB) error {
	if _, err := db.ExecContext(ctx, pulseReviewLogSchema); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(pulse_review_log)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for name, definition := range map[string]string{
		"status":            "TEXT NOT NULL DEFAULT ''",
		"artifact_kind":     "TEXT NOT NULL DEFAULT 'review'",
		"artifact_markdown": "TEXT NOT NULL DEFAULT ''",
		"content_sha256":    "TEXT NOT NULL DEFAULT ''",
	} {
		if columns[name] {
			continue
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE pulse_review_log ADD COLUMN `+name+` `+definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, pulseReviewLogIndex); err != nil {
		return err
	}
	return nil
}

// extractReviewVerdict pulls the reviewer's conclusion out of its artifact.
//
// Reviewers write markdown, and the verdict shows up as `## Verdict`, `Verdict:`,
// or `**Verdict:**` depending on the module. Handles the heading form (where the
// text is on the following lines) as well as the inline form.
//
// Returns "" when no verdict is found rather than guessing: an empty verdict with
// a recorded run is still useful — it says the reviewer ran — whereas inventing
// one would poison the history the Gate is meant to trust.
func extractReviewVerdict(artifact string) string {
	lines := strings.Split(artifact, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(strings.TrimLeft(line, "#* "))
		if !strings.HasPrefix(lower, "verdict") {
			continue
		}
		// Inline form: "Verdict: <text>" / "**Verdict:** <text>".
		if idx := strings.Index(line, ":"); idx >= 0 {
			if rest := strings.TrimSpace(strings.Trim(line[idx+1:], "* ")); rest != "" {
				return truncateVerdict(rest)
			}
		}
		// Heading form: text begins on the next non-empty line.
		for _, next := range lines[i+1:] {
			if candidate := strings.TrimSpace(next); candidate != "" {
				return truncateVerdict(candidate)
			}
		}
		return ""
	}
	return ""
}

func truncateVerdict(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxVerdictChars {
		return s
	}
	return s[:maxVerdictChars] + "…"
}

// RecordPulseReview logs that a reviewer ran and what it concluded.
//
// Best-effort: a reviewer whose artifact was persisted must not fail because the
// bookkeeping write did. Callers log and continue.
func RecordPulseReview(ctx context.Context, workspacePath, module, reviewRunID, pulseRunID, artifactPath, artifact string) error {
	return RecordPulseReviewForModules(ctx, workspacePath, []string{module}, reviewRunID, pulseRunID, artifactPath, artifact)
}

// RecordPulseReviewForModules stores one shared reviewer artifact under every
// Gate-selected lane. The Markdown is one report, but each lane must retain an
// independent review-history and lookup identity for future Gate decisions.
func RecordPulseReviewForModules(ctx context.Context, workspacePath string, modules []string, reviewRunID, pulseRunID, artifactPath, artifact string) error {
	canonicalModules := make([]string, 0, len(modules))
	seen := map[string]bool{}
	for _, module := range modules {
		module = pulsemodules.Normalize(module)
		if module == "" || seen[module] {
			continue
		}
		seen[module] = true
		canonicalModules = append(canonicalModules, module)
	}
	if len(canonicalModules) == 0 {
		return nil
	}
	verifications, contractErr := ExtractPulseReviewVerifications(artifact)
	if contractErr == nil {
		contractErr = validatePulseReviewVerificationAllowlistForModules(ctx, workspacePath, canonicalModules, verifications)
	}
	if contractErr == nil {
		for _, module := range canonicalModules {
			if err := validatePulseAdvisorFindingRoutes(module, artifact); err != nil {
				contractErr = err
				break
			}
		}
	}
	status := pulseReviewStatus(artifact)
	persistedArtifact := artifact
	if contractErr != nil {
		// Keep the complete reviewer artifact even when one structured marker is
		// malformed. Previously the validation error returned before persistence,
		// so a long, otherwise useful review vanished and the consolidated Fixer
		// could neither inspect the evidence nor terminalize the module honestly.
		// Invalid markers remain quarantined: every structured-verification read
		// path below skips contract_failed records.
		status = pulseReviewStatusContractFailed
		persistedArtifact = strings.TrimRight(artifact, "\n") +
			"\n\n---\n\n**Pulse review contract failure:** " + contractErr.Error() + "\n"
	}
	recordedAt := time.Now().UTC().Format(time.RFC3339Nano)
	var recordErrors []string
	for _, module := range canonicalModules {
		if err := recordPulseReviewAt(
			ctx, workspacePath, module, reviewRunID, pulseRunID, "review",
			artifactPath, status, persistedArtifact, recordedAt,
		); err != nil {
			recordErrors = append(recordErrors, module+": "+err.Error())
		}
	}
	if len(recordErrors) > 0 {
		recordErr := fmt.Errorf("%s", strings.Join(recordErrors, "; "))
		if contractErr != nil {
			return fmt.Errorf("review contract failed (%w) and the artifact could not be retained: %w", contractErr, recordErr)
		}
		return recordErr
	}
	if contractErr != nil {
		return fmt.Errorf("review contract failed; artifact retained with status %s: %w", pulseReviewStatusContractFailed, contractErr)
	}
	return nil
}

func validatePulseReviewVerificationAllowlistForModules(
	ctx context.Context,
	workspacePath string,
	modules []string,
	verifications []PulseReviewVerificationResult,
) error {
	if len(verifications) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	canonicalModules := make([]string, 0, len(modules))
	for _, module := range modules {
		module = pulsemodules.Normalize(module)
		if module == "" {
			continue
		}
		canonicalModules = append(canonicalModules, module)
		candidates, err := LoadPulseReviewVerificationCandidates(ctx, workspacePath, module)
		if err != nil {
			return fmt.Errorf("load verification allowlist for %s: %w", module, err)
		}
		for _, candidate := range candidates {
			allowed[candidate.FindingID+"\x00"+candidate.Fingerprint+"\x00"+candidate.AttemptID] = true
		}
	}
	for _, verification := range verifications {
		key := verification.FindingID + "\x00" + verification.Fingerprint + "\x00" + verification.AttemptID
		if !allowed[key] {
			return fmt.Errorf(
				"verification marker for finding %q attempt %q is outside the backend allowlist for selected modules %q",
				verification.FindingID, verification.AttemptID, strings.Join(canonicalModules, ","),
			)
		}
	}
	return nil
}

func recordPulseReviewAt(
	ctx context.Context,
	workspacePath, module, reviewRunID, pulseRunID, artifactKind, artifactPath, status, artifact, recordedAt string,
) error {
	module = pulsemodules.Normalize(module)
	if module == "" {
		return nil
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return err
	}
	defer db.Close()
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return err
	}
	return recordPulseReviewOnDB(
		ctx, db, module, reviewRunID, pulseRunID, artifactKind,
		artifactPath, status, artifact, recordedAt,
	)
}

func recordPulseReviewOnDB(
	ctx context.Context,
	db pulseFindingLifecycleDB,
	module, reviewRunID, pulseRunID, artifactKind, artifactPath, status, artifact, recordedAt string,
) error {
	if strings.TrimSpace(recordedAt) == "" {
		recordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	artifactKind = strings.TrimSpace(artifactKind)
	if artifactKind == "" {
		artifactKind = "review"
	}
	sum := sha256.Sum256([]byte(artifact))
	contentSHA := hex.EncodeToString(sum[:])
	result, err := db.ExecContext(ctx, `UPDATE pulse_review_log SET
			pulse_run_id=?, verdict=?, status=?, artifact_path=?, artifact_bytes=?,
			artifact_markdown=?, content_sha256=?, recorded_at=?
		WHERE module=? AND review_run_id=? AND artifact_kind=? AND artifact_path=?`,
		strings.TrimSpace(pulseRunID), extractReviewVerdict(artifact), strings.TrimSpace(status),
		strings.TrimSpace(artifactPath), len(artifact), artifact, contentSHA, recordedAt,
		module, strings.TrimSpace(reviewRunID), artifactKind, strings.TrimSpace(artifactPath))
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `INSERT INTO pulse_review_log
		(module, review_run_id, pulse_run_id, verdict, status, artifact_kind,
		 artifact_path, artifact_bytes, artifact_markdown, content_sha256, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		module, strings.TrimSpace(reviewRunID), strings.TrimSpace(pulseRunID),
		extractReviewVerdict(artifact), strings.TrimSpace(status), artifactKind,
		strings.TrimSpace(artifactPath), len(artifact), artifact, contentSHA, recordedAt)
	return err
}

func pulseReviewStatus(artifact string) string {
	for _, line := range strings.Split(artifact, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "- status:") {
			return strings.Trim(strings.TrimSpace(line[len("- status:"):]), "`* ")
		}
	}
	return ""
}

// LoadPulseReviewArtifacts returns every matching artifact when limit is
// negative. Zero keeps the bounded default for preview/history callers.
func LoadPulseReviewArtifacts(ctx context.Context, workspacePath, module string, includeMarkdown bool, limit int) ([]PulseReviewArtifactRecord, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return []PulseReviewArtifactRecord{}, err
	}
	defer db.Close()
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 200
	}
	module = pulsemodules.Normalize(module)
	markdownColumn := "''"
	if includeMarkdown {
		markdownColumn = "artifact_markdown"
	}
	query := `SELECT _id, module, review_run_id, pulse_run_id,
			verdict, status, artifact_kind, artifact_path, artifact_bytes,
			content_sha256, recorded_at, ` + markdownColumn + `
		FROM pulse_review_log
		WHERE artifact_kind='review'`
	args := []interface{}{}
	if module != "" {
		aliases := []string{}
		for _, accepted := range pulsemodules.AcceptedForReviewArtifacts() {
			if pulsemodules.Normalize(accepted) == module {
				aliases = append(aliases, accepted)
			}
		}
		if len(aliases) == 0 {
			return []PulseReviewArtifactRecord{}, nil
		}
		query += ` AND module IN (` + strings.TrimRight(strings.Repeat("?,", len(aliases)), ",") + `)`
		for _, alias := range aliases {
			args = append(args, alias)
		}
	}
	query += ` ORDER BY recorded_at DESC, _id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PulseReviewArtifactRecord{}
	for rows.Next() {
		var artifact PulseReviewArtifactRecord
		if err := rows.Scan(
			&artifact.ID, &artifact.Module, &artifact.ReviewRunID, &artifact.PulseRunID,
			&artifact.Verdict, &artifact.Status, &artifact.ArtifactKind,
			&artifact.LegacySourcePath, &artifact.ArtifactBytes, &artifact.ContentSHA256,
			&artifact.RecordedAt, &artifact.Markdown,
		); err != nil {
			return nil, err
		}
		artifact.Module = pulsemodules.Normalize(artifact.Module)
		if includeMarkdown && artifact.Status != pulseReviewStatusContractFailed {
			artifact.Verifications, err = ExtractPulseReviewVerifications(artifact.Markdown)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	metrics, err := LoadPulseAgentMetrics(ctx, workspacePath, "", "", "reviewer", -1)
	if err != nil {
		return nil, err
	}
	attachPulseReviewMetrics(out, metrics)
	return out, nil
}

func LoadPulseReviewArtifact(ctx context.Context, workspacePath string, id int64) (*PulseReviewArtifactRecord, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, sql.ErrNoRows
	}
	defer db.Close()
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return nil, err
	}
	var artifact PulseReviewArtifactRecord
	err = db.QueryRowContext(ctx, `SELECT _id, module, review_run_id, pulse_run_id,
			verdict, status, artifact_kind, artifact_path, artifact_bytes,
			content_sha256, recorded_at, artifact_markdown
		FROM pulse_review_log WHERE _id=?`, id).Scan(
		&artifact.ID, &artifact.Module, &artifact.ReviewRunID, &artifact.PulseRunID,
		&artifact.Verdict, &artifact.Status, &artifact.ArtifactKind,
		&artifact.LegacySourcePath, &artifact.ArtifactBytes, &artifact.ContentSHA256,
		&artifact.RecordedAt, &artifact.Markdown,
	)
	if err != nil {
		return nil, err
	}
	artifact.Module = pulsemodules.Normalize(artifact.Module)
	if artifact.Status != pulseReviewStatusContractFailed {
		artifact.Verifications, err = ExtractPulseReviewVerifications(artifact.Markdown)
		if err != nil {
			return nil, err
		}
	}
	metrics, err := LoadPulseAgentMetrics(ctx, workspacePath, artifact.PulseRunID, artifact.Module, "reviewer", -1)
	if err != nil {
		return nil, err
	}
	withMetrics := []PulseReviewArtifactRecord{artifact}
	attachPulseReviewMetrics(withMetrics, metrics)
	artifact = withMetrics[0]
	return &artifact, nil
}

func LoadPulseReviewArtifactForRun(ctx context.Context, workspacePath, reviewRunID, module string) (*PulseReviewArtifactRecord, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, sql.ErrNoRows
	}
	defer db.Close()
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return nil, err
	}
	module = pulsemodules.Normalize(module)
	var artifact PulseReviewArtifactRecord
	err = db.QueryRowContext(ctx, `SELECT _id, module, review_run_id, pulse_run_id,
			verdict, status, artifact_kind, artifact_path, artifact_bytes,
			content_sha256, recorded_at, artifact_markdown
		FROM pulse_review_log
		WHERE review_run_id=? AND module=? AND artifact_kind='review'
		ORDER BY _id DESC LIMIT 1`, strings.TrimSpace(reviewRunID), module).Scan(
		&artifact.ID, &artifact.Module, &artifact.ReviewRunID, &artifact.PulseRunID,
		&artifact.Verdict, &artifact.Status, &artifact.ArtifactKind,
		&artifact.LegacySourcePath, &artifact.ArtifactBytes, &artifact.ContentSHA256,
		&artifact.RecordedAt, &artifact.Markdown,
	)
	if err != nil {
		return nil, err
	}
	artifact.Module = pulsemodules.Normalize(artifact.Module)
	if artifact.Status != pulseReviewStatusContractFailed {
		artifact.Verifications, err = ExtractPulseReviewVerifications(artifact.Markdown)
		if err != nil {
			return nil, err
		}
	}
	metrics, err := LoadPulseAgentMetrics(ctx, workspacePath, artifact.PulseRunID, artifact.Module, "reviewer", -1)
	if err != nil {
		return nil, err
	}
	withMetrics := []PulseReviewArtifactRecord{artifact}
	attachPulseReviewMetrics(withMetrics, metrics)
	artifact = withMetrics[0]
	return &artifact, nil
}

func attachPulseReviewMetrics(reviews []PulseReviewArtifactRecord, metrics []PulseAgentMetricRecord) {
	byReview := make(map[string]*PulseAgentMetricRecord, len(metrics))
	for i := range metrics {
		metric := &metrics[i]
		key := strings.TrimSpace(metric.ReviewRunID) + "\x00" + pulsemodules.Normalize(metric.Module)
		if _, exists := byReview[key]; !exists {
			byReview[key] = metric
		}
	}
	for i := range reviews {
		key := strings.TrimSpace(reviews[i].ReviewRunID) + "\x00" + pulsemodules.Normalize(reviews[i].Module)
		reviews[i].Metrics = byReview[key]
	}
}

func LoadPulseReviewVerificationsForPulseRun(ctx context.Context, workspacePath, pulseRunID, module string) ([]PulseReviewVerificationResult, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return nil, err
	}
	module = pulsemodules.Normalize(module)
	rows, err := db.QueryContext(ctx, `SELECT artifact_markdown FROM pulse_review_log
		WHERE pulse_run_id=? AND module=? AND artifact_kind='review'
			AND status NOT IN ('failed', 'contract_failed')
		ORDER BY recorded_at ASC, _id ASC`, strings.TrimSpace(pulseRunID), module)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []PulseReviewVerificationResult{}
	for rows.Next() {
		var markdown string
		if err := rows.Scan(&markdown); err != nil {
			return nil, err
		}
		parsed, err := ExtractPulseReviewVerifications(markdown)
		if err != nil {
			return nil, err
		}
		results = append(results, parsed...)
	}
	return results, rows.Err()
}

// LoadModuleReviewHistory summarizes recent reviews per module, most recently run
// first, so Gate can weigh "this one keeps finding things" against "this one has
// come back clean five times".
func LoadModuleReviewHistory(ctx context.Context, workspacePath string, perModule int) ([]ModuleReviewHistory, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if perModule <= 0 {
		perModule = 3
	}

	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT module, recorded_at, verdict
		FROM pulse_review_log WHERE artifact_kind='review'
		ORDER BY recorded_at DESC, _id DESC`)
	if err != nil {
		// No table means no reviewer has run yet — that is "nothing to report",
		// not an error the Gate has to handle.
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	var out []ModuleReviewHistory
	indexByModule := map[string]int{}
	for rows.Next() {
		var module, recordedAt, verdict string
		if err := rows.Scan(&module, &recordedAt, &verdict); err != nil {
			rows.Close()
			return nil, err
		}
		module = pulsemodules.Normalize(module)
		index, exists := indexByModule[module]
		if !exists {
			index = len(out)
			indexByModule[module] = index
			out = append(out, ModuleReviewHistory{
				Module:        module,
				LastRanAt:     recordedAt,
				RecentVerdict: []string{},
			})
		}
		out[index].RunCount++
		if len(out[index].RecentVerdict) < perModule {
			if strings.TrimSpace(verdict) == "" {
				verdict = "(ran; no verdict line found in the artifact)"
			}
			out[index].RecentVerdict = append(
				out[index].RecentVerdict,
				fmt.Sprintf("%s — %s", recordedAt, verdict),
			)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	return out, nil
}
