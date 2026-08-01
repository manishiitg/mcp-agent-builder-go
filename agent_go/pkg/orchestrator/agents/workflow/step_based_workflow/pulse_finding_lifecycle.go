package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const pulseFixAttemptsSchema = `CREATE TABLE IF NOT EXISTS pulse_fix_attempts (
	attempt_id TEXT PRIMARY KEY,
	module TEXT NOT NULL,
	pulse_run_id TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'fixing',
	intended_files_json TEXT NOT NULL DEFAULT '[]',
	changed_files_json TEXT NOT NULL DEFAULT '[]',
	before_refs_json TEXT NOT NULL DEFAULT '[]',
	after_refs_json TEXT NOT NULL DEFAULT '[]',
	started_at TEXT NOT NULL,
	completed_at TEXT NOT NULL DEFAULT ''
)`

const pulseFixAttemptFindingsSchema = `CREATE TABLE IF NOT EXISTS pulse_fix_attempt_findings (
	attempt_id TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	finding_id TEXT NOT NULL DEFAULT '',
	disposition TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (attempt_id, fingerprint)
)`

const pulseFixVerificationsSchema = `CREATE TABLE IF NOT EXISTS pulse_fix_verifications (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	attempt_id TEXT NOT NULL DEFAULT '',
	fingerprint TEXT NOT NULL,
	check_text TEXT NOT NULL,
	verdict TEXT NOT NULL,
	expected TEXT NOT NULL DEFAULT '',
	observed TEXT NOT NULL DEFAULT '',
	evidence_json TEXT NOT NULL DEFAULT '[]',
	verified_at TEXT NOT NULL,
	UNIQUE(attempt_id, fingerprint, check_text)
)`

const pulseFindingEventsSchema = `CREATE TABLE IF NOT EXISTS pulse_finding_events (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	fingerprint TEXT NOT NULL,
	finding_id TEXT NOT NULL DEFAULT '',
	pulse_run_id TEXT NOT NULL DEFAULT '',
	attempt_id TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	metadata_json TEXT NOT NULL DEFAULT '{}',
	recorded_at TEXT NOT NULL,
	UNIQUE(fingerprint, pulse_run_id, attempt_id, event_type)
)`

const (
	ConcernStatusFixing               = "fixing"
	ConcernStatusAwaitingVerification = "awaiting_verification"
	ConcernStatusAwaitingRun          = "awaiting_run"
)

const (
	FindingDispositionFixedVerified     = "fixed_verified"
	FindingDispositionVerifiedNoChange  = "verified_no_change"
	FindingDispositionChangedUnverified = "changed_unverified"
	FindingDispositionProposalOnly      = "proposal_only"
	FindingDispositionAwaitingUser      = "awaiting_user"
	FindingDispositionBlocked           = "blocked"
	FindingDispositionExternalAction    = "external_action_required"
	// FindingDispositionAwaitingRun is a real finding that no one is stuck on:
	// the evidence to resolve it simply has not been produced yet, and the next
	// scheduled run will produce it.
	//
	// blocked used to absorb these because changed_unverified requires a fix
	// attempt with changed files, and nothing was fixed — the data was never
	// collected. So rtslatency reported 9 blocked when 4 were only waiting: the
	// security and latency rows were missing because those steps had not run,
	// and the approved experiment could not ship because the digest step had not
	// executed since 2026-07-29. Reading those as blockers points the operator at
	// decisions that do not exist, and hides the ones that do.
	FindingDispositionAwaitingRun = "awaiting_run"
	FindingDispositionFailed      = "failed"
	FindingDispositionRejected    = "rejected"
)

const (
	VerificationPassed       = "passed"
	VerificationFailed       = "failed"
	VerificationInconclusive = "inconclusive"
)

type PulseFixFindingRef struct {
	Fingerprint string `json:"fingerprint"`
	FindingID   string `json:"finding_id"`
	Disposition string `json:"disposition,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

type PulseFixAttempt struct {
	AttemptID     string               `json:"attempt_id"`
	Module        string               `json:"module"`
	PulseRunID    string               `json:"pulse_run_id"`
	Summary       string               `json:"summary"`
	Status        string               `json:"status"`
	IntendedFiles []string             `json:"intended_files"`
	ChangedFiles  []string             `json:"changed_files"`
	BeforeRefs    []string             `json:"before_refs"`
	AfterRefs     []string             `json:"after_refs"`
	StartedAt     string               `json:"started_at"`
	CompletedAt   string               `json:"completed_at,omitempty"`
	Findings      []PulseFixFindingRef `json:"findings,omitempty"`
}

type PulseFindingVerification struct {
	Check    string   `json:"check"`
	Verdict  string   `json:"verdict"`
	Expected string   `json:"expected,omitempty"`
	Observed string   `json:"observed,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	At       string   `json:"verified_at,omitempty"`
}

type PulseFindingDisposition struct {
	Fingerprint     string   `json:"fingerprint"`
	FindingID       string   `json:"finding_id"`
	AttemptID       string   `json:"attempt_id,omitempty"`
	Disposition     string   `json:"disposition"`
	Summary         string   `json:"summary"`
	ChangedFiles    []string `json:"changed_files,omitempty"`
	BeforeRefs      []string `json:"before_refs,omitempty"`
	AfterRefs       []string `json:"after_refs,omitempty"`
	NextCheck       string   `json:"next_check,omitempty"`
	ExternalOwner   string   `json:"external_owner,omitempty"`
	ReasonCode      string   `json:"reason_code,omitempty"`
	ReopenCondition string   `json:"reopen_condition,omitempty"`
	// HumanInputID links an awaiting_user finding to the question actually put
	// to the operator. Without it "waiting on the user" is recordable while the
	// user is never asked, which is exactly what happened: rtslatency held five
	// findings marked awaiting_user and zero pending questions, so the operator
	// had nothing to answer and no way to discover that.
	HumanInputID string                     `json:"human_input_id,omitempty"`
	Verification []PulseFindingVerification `json:"verification,omitempty"`
}

type PulseFindingEvent struct {
	FindingID  string                 `json:"finding_id,omitempty"`
	EventType  string                 `json:"event_type"`
	Summary    string                 `json:"summary"`
	PulseRunID string                 `json:"pulse_run_id,omitempty"`
	AttemptID  string                 `json:"attempt_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	RecordedAt string                 `json:"recorded_at"`
}

type PulseFindingLifecycle struct {
	Issue           PulseIssue                 `json:"issue"`
	Fingerprint     string                     `json:"fingerprint"`
	FindingID       string                     `json:"finding_id,omitempty"`
	Module          string                     `json:"module,omitempty"`
	StepID          string                     `json:"step_id"`
	Phase           string                     `json:"phase"`
	GroupName       string                     `json:"group_name,omitempty"`
	Text            string                     `json:"text"`
	Status          string                     `json:"status"`
	FirstSeenRun    string                     `json:"first_seen_run,omitempty"`
	FirstSeenAt     string                     `json:"first_seen_at,omitempty"`
	LastSeenRun     string                     `json:"last_seen_run,omitempty"`
	LastSeenAt      string                     `json:"last_seen_at,omitempty"`
	SeenCount       int                        `json:"seen_count"`
	ResolutionNote  string                     `json:"resolution_note,omitempty"`
	ExternalOwner   string                     `json:"external_owner,omitempty"`
	ReasonCode      string                     `json:"reason_code,omitempty"`
	ReopenCondition string                     `json:"reopen_condition,omitempty"`
	Details         *PulseFindingDetails       `json:"details,omitempty"`
	Attempts        []PulseFixAttempt          `json:"fix_attempts"`
	Verification    []PulseFindingVerification `json:"verifications"`
	Events          []PulseFindingEvent        `json:"events"`
}

type pulseFindingLifecycleDB interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func ensurePulseFindingLifecycleSchema(ctx context.Context, db pulseFindingLifecycleDB) error {
	for _, ddl := range []string{
		runConcernsSchema,
		pulseFixAttemptsSchema,
		pulseFixAttemptFindingsSchema,
		pulseFixVerificationsSchema,
		pulseFindingEventsSchema,
		pulseFindingDetailsSchema,
		`CREATE INDEX IF NOT EXISTS idx_pulse_fix_attempts_module_run ON pulse_fix_attempts(module, pulse_run_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_fix_findings_fingerprint ON pulse_fix_attempt_findings(fingerprint, attempt_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_fix_verifications_fingerprint ON pulse_fix_verifications(fingerprint, verified_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_finding_events_fingerprint ON pulse_finding_events(fingerprint, recorded_at DESC)`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func normalizedLifecycleStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// NormalizePulseFindingDisposition canonicalizes tool-provided lifecycle data
// before either validation or persistence. Keeping this at the write boundary
// prevents a value accepted after trimming from being routed by its original,
// untrimmed spelling.
func NormalizePulseFindingDisposition(disposition PulseFindingDisposition) PulseFindingDisposition {
	disposition.Fingerprint = strings.TrimSpace(disposition.Fingerprint)
	disposition.FindingID = strings.TrimSpace(disposition.FindingID)
	disposition.AttemptID = strings.TrimSpace(disposition.AttemptID)
	disposition.Disposition = strings.TrimSpace(disposition.Disposition)
	disposition.Summary = strings.TrimSpace(disposition.Summary)
	disposition.ChangedFiles = normalizedLifecycleStrings(disposition.ChangedFiles)
	disposition.BeforeRefs = normalizedLifecycleStrings(disposition.BeforeRefs)
	disposition.AfterRefs = normalizedLifecycleStrings(disposition.AfterRefs)
	disposition.NextCheck = strings.TrimSpace(disposition.NextCheck)
	disposition.ExternalOwner = strings.TrimSpace(disposition.ExternalOwner)
	disposition.ReasonCode = strings.TrimSpace(disposition.ReasonCode)
	disposition.ReopenCondition = strings.TrimSpace(disposition.ReopenCondition)
	disposition.HumanInputID = strings.TrimSpace(disposition.HumanInputID)
	for index := range disposition.Verification {
		verification := &disposition.Verification[index]
		verification.Check = strings.TrimSpace(verification.Check)
		verification.Verdict = strings.TrimSpace(verification.Verdict)
		verification.Expected = strings.TrimSpace(verification.Expected)
		verification.Observed = strings.TrimSpace(verification.Observed)
		verification.Evidence = normalizedLifecycleStrings(verification.Evidence)
	}
	return disposition
}

func lifecycleJSON(values []string) string {
	encoded, _ := json.Marshal(normalizedLifecycleStrings(values))
	return string(encoded)
}

func lifecycleAttemptID(pulseRunID, module string, findings []PulseFixFindingRef, intendedFiles, beforeRefs []string) string {
	keys := make([]string, 0, len(findings))
	for _, finding := range findings {
		keys = append(keys, strings.TrimSpace(finding.Fingerprint))
	}
	sort.Strings(keys)
	files := normalizedLifecycleStrings(intendedFiles)
	refs := normalizedLifecycleStrings(beforeRefs)
	sort.Strings(files)
	sort.Strings(refs)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(pulseRunID),
		strings.TrimSpace(module),
		strings.Join(keys, "\x00"),
		strings.Join(files, "\x00"),
		strings.Join(refs, "\x00"),
	}, "\x01")))
	return "fix-" + hex.EncodeToString(sum[:])[:16]
}

// StartPulseFixAttempt durably moves one or more filed concerns into fixing
// before any mutation occurs. Its deterministic ID makes agent/tool retries
// idempotent for the same Pulse run, module, and concern set.
func StartPulseFixAttempt(
	ctx context.Context,
	workspacePath, pulseRunID, module, summary string,
	findings []PulseFixFindingRef,
	intendedFiles, beforeRefs []string,
) (*PulseFixAttempt, error) {
	if strings.TrimSpace(pulseRunID) == "" || strings.TrimSpace(module) == "" {
		return nil, fmt.Errorf("pulse_run_id and module are required")
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("at least one finding is required")
	}
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("no workflow database at %s", runConcernsDBPath(workspacePath))
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return nil, err
	}

	attemptID := lifecycleAttemptID(pulseRunID, module, findings, intendedFiles, beforeRefs)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, finding := range findings {
		fingerprint := strings.TrimSpace(finding.Fingerprint)
		findingID := strings.TrimSpace(finding.FindingID)
		if fingerprint == "" || findingID == "" {
			return nil, fmt.Errorf("each finding requires fingerprint and finding_id")
		}
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM run_concerns WHERE fingerprint=?`, fingerprint).Scan(&status); err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("no concern with fingerprint %q", fingerprint)
			}
			return nil, err
		}
		if status == ConcernStatusRejected {
			return nil, fmt.Errorf("concern %q was rejected and cannot enter fixing without new adjudication", fingerprint)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_fix_attempts
		(attempt_id, module, pulse_run_id, summary, status, intended_files_json, before_refs_json, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(attempt_id) DO NOTHING`,
		attemptID, strings.TrimSpace(module), strings.TrimSpace(pulseRunID), strings.TrimSpace(summary),
		ConcernStatusFixing, lifecycleJSON(intendedFiles), lifecycleJSON(beforeRefs), now); err != nil {
		return nil, err
	}
	for _, finding := range findings {
		fingerprint := strings.TrimSpace(finding.Fingerprint)
		findingID := strings.TrimSpace(finding.FindingID)
		if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_fix_attempt_findings
			(attempt_id, fingerprint, finding_id) VALUES (?, ?, ?)
			ON CONFLICT(attempt_id, fingerprint) DO UPDATE SET finding_id=excluded.finding_id`,
			attemptID, fingerprint, findingID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE run_concerns SET
			status=?, resolved_at='', resolved_by='', resolution_note=''
			WHERE fingerprint=?`, ConcernStatusFixing, fingerprint); err != nil {
			return nil, err
		}
		metadata, _ := json.Marshal(map[string]interface{}{
			"intended_files": normalizedLifecycleStrings(intendedFiles),
			"before_refs":    normalizedLifecycleStrings(beforeRefs),
		})
		if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_finding_events
			(fingerprint, finding_id, pulse_run_id, attempt_id, event_type, summary, metadata_json, recorded_at)
			VALUES (?, ?, ?, ?, 'fix_started', ?, ?, ?)
			ON CONFLICT(fingerprint, pulse_run_id, attempt_id, event_type) DO NOTHING`,
			fingerprint, findingID, pulseRunID, attemptID, strings.TrimSpace(summary), string(metadata), now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &PulseFixAttempt{
		AttemptID:     attemptID,
		Module:        strings.TrimSpace(module),
		PulseRunID:    strings.TrimSpace(pulseRunID),
		Summary:       strings.TrimSpace(summary),
		Status:        ConcernStatusFixing,
		IntendedFiles: normalizedLifecycleStrings(intendedFiles),
		BeforeRefs:    normalizedLifecycleStrings(beforeRefs),
		StartedAt:     now,
		Findings:      findings,
	}, nil
}

func validateFindingDisposition(disposition PulseFindingDisposition) error {
	disposition = NormalizePulseFindingDisposition(disposition)
	if disposition.Fingerprint == "" || disposition.FindingID == "" {
		return fmt.Errorf("finding disposition requires fingerprint and finding_id")
	}
	if disposition.Summary == "" {
		return fmt.Errorf("finding %q disposition requires summary", disposition.FindingID)
	}
	allowed := map[string]bool{
		FindingDispositionFixedVerified: true, FindingDispositionVerifiedNoChange: true,
		FindingDispositionChangedUnverified: true, FindingDispositionProposalOnly: true,
		FindingDispositionAwaitingUser: true, FindingDispositionBlocked: true,
		FindingDispositionExternalAction: true, FindingDispositionFailed: true,
		FindingDispositionRejected: true, FindingDispositionAwaitingRun: true,
	}
	if !allowed[disposition.Disposition] {
		return fmt.Errorf("finding %q has invalid disposition %q", disposition.FindingID, disposition.Disposition)
	}

	passed, failed, inconclusive := 0, 0, 0
	for _, verification := range disposition.Verification {
		if strings.TrimSpace(verification.Check) == "" {
			return fmt.Errorf("finding %q verification requires check", disposition.FindingID)
		}
		switch strings.TrimSpace(verification.Verdict) {
		case VerificationPassed:
			passed++
		case VerificationFailed:
			failed++
		case VerificationInconclusive:
			inconclusive++
		default:
			return fmt.Errorf("finding %q verification verdict must be passed, failed, or inconclusive", disposition.FindingID)
		}
	}
	switch disposition.Disposition {
	case FindingDispositionFixedVerified:
		if strings.TrimSpace(disposition.AttemptID) == "" || len(disposition.ChangedFiles) == 0 {
			return fmt.Errorf("fixed_verified finding %q requires attempt_id and changed_files", disposition.FindingID)
		}
		if len(disposition.BeforeRefs) != len(disposition.AfterRefs) {
			return fmt.Errorf("fixed_verified finding %q requires paired before_refs and after_refs", disposition.FindingID)
		}
		if passed == 0 || failed > 0 || inconclusive > 0 {
			return fmt.Errorf("fixed_verified finding %q requires one or more passed verifications and no failed/inconclusive checks", disposition.FindingID)
		}
	case FindingDispositionChangedUnverified:
		if strings.TrimSpace(disposition.AttemptID) == "" || len(disposition.ChangedFiles) == 0 {
			return fmt.Errorf("changed_unverified finding %q requires attempt_id and changed_files", disposition.FindingID)
		}
		if len(disposition.BeforeRefs) != len(disposition.AfterRefs) {
			return fmt.Errorf("changed_unverified finding %q requires paired before_refs and after_refs", disposition.FindingID)
		}
		if inconclusive == 0 || failed > 0 {
			return fmt.Errorf("changed_unverified finding %q requires an inconclusive verification and no failed check", disposition.FindingID)
		}
	case FindingDispositionVerifiedNoChange:
		if passed == 0 || failed > 0 || inconclusive > 0 {
			return fmt.Errorf("verified_no_change finding %q requires passed verification", disposition.FindingID)
		}
	case FindingDispositionFailed:
		if len(disposition.Verification) > 0 && failed == 0 {
			return fmt.Errorf("failed finding %q with verification evidence requires a failed check", disposition.FindingID)
		}
	case FindingDispositionAwaitingRun:
		// Naming the evidence boundary is what separates waiting from stalling:
		// without it nobody can tell whether the run that would resolve this has
		// already happened.
		if disposition.NextCheck == "" {
			return fmt.Errorf("awaiting_run finding %q requires next_check naming the run or evidence that will resolve it", disposition.FindingID)
		}
		if len(disposition.ChangedFiles) > 0 {
			return fmt.Errorf("awaiting_run finding %q changed files; a finding with a fix applied is changed_unverified, not awaiting_run", disposition.FindingID)
		}
	case FindingDispositionAwaitingUser:
		// A finding cannot wait on a decision nobody was asked for. Requiring
		// the question id here is what turns "awaiting_user" from a label into
		// something the operator can actually act on.
		if disposition.HumanInputID == "" {
			return fmt.Errorf("awaiting_user finding %q requires human_input_id: create the decision with create_human_input_request first, or use blocked/proposal_only if no question is being asked", disposition.FindingID)
		}
	case FindingDispositionExternalAction:
		if disposition.ExternalOwner == "" || disposition.ReasonCode == "" || disposition.ReopenCondition == "" {
			return fmt.Errorf("external_action_required finding %q requires external_owner, reason_code, and reopen_condition", disposition.FindingID)
		}
		switch disposition.ExternalOwner {
		case "platform", "user", "vendor", "workflow_owner":
		default:
			return fmt.Errorf("external_action_required finding %q has invalid external_owner %q", disposition.FindingID, disposition.ExternalOwner)
		}
	}
	return nil
}

func lifecycleStatusForDisposition(disposition string) (status, eventType, resolvedBy string) {
	switch strings.TrimSpace(disposition) {
	case FindingDispositionFixedVerified, FindingDispositionVerifiedNoChange:
		return ConcernStatusResolved, "closed", "pulse_fixer"
	case FindingDispositionChangedUnverified:
		return ConcernStatusAwaitingVerification, "verification_inconclusive", ""
	case FindingDispositionRejected:
		return ConcernStatusRejected, "rejected", "pulse_fixer"
	case FindingDispositionFailed:
		return ConcernStatusOpen, "verification_failed", ""
	case FindingDispositionProposalOnly:
		return ConcernStatusAcknowledged, "proposal_recorded", ""
	case FindingDispositionAwaitingUser:
		return ConcernStatusAcknowledged, "awaiting_user", ""
	case FindingDispositionBlocked:
		return ConcernStatusAcknowledged, "blocked", ""
	case FindingDispositionAwaitingRun:
		return ConcernStatusAwaitingRun, "awaiting_run", ""
	case FindingDispositionExternalAction:
		return ConcernStatusExternalActionRequired, "external_action_required", "pulse"
	default:
		return ConcernStatusOpen, "updated", ""
	}
}

// RecordPulseFindingDispositionsTx writes per-finding outcomes in the same
// transaction as the module audit. This prevents "module changed" from being
// committed while its issue/test lifecycle silently remains open.
func RecordPulseFindingDispositionsTx(
	ctx context.Context,
	db pulseFindingLifecycleDB,
	module, pulseRunID string,
	dispositions []PulseFindingDisposition,
	recordedAt string,
) error {
	if len(dispositions) == 0 {
		return nil
	}
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return err
	}
	if strings.TrimSpace(recordedAt) == "" {
		recordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	type attemptAggregate struct {
		changedFiles []string
		beforeRefs   []string
		afterRefs    []string
		status       string
	}
	attempts := map[string]*attemptAggregate{}

	for _, disposition := range dispositions {
		disposition = NormalizePulseFindingDisposition(disposition)
		if err := validateFindingDisposition(disposition); err != nil {
			return err
		}
		fingerprint := disposition.Fingerprint
		findingID := disposition.FindingID
		attemptID := disposition.AttemptID
		var concernExists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_concerns WHERE fingerprint=?`, fingerprint).Scan(&concernExists); err != nil {
			return err
		}
		if concernExists != 1 {
			return fmt.Errorf("no concern with fingerprint %q", fingerprint)
		}
		// Prove the decision exists and is still open. A claimed id is not
		// evidence: an already-answered or invented question would leave the
		// finding parked on a decision the operator can never make.
		if disposition.Disposition == FindingDispositionAwaitingUser {
			var inputStatus string
			err := db.QueryRowContext(ctx,
				`SELECT status FROM report_human_inputs WHERE id=?`, disposition.HumanInputID,
			).Scan(&inputStatus)
			if err != nil {
				if err == sql.ErrNoRows {
					return fmt.Errorf("awaiting_user finding %q references human input %q, which does not exist", findingID, disposition.HumanInputID)
				}
				return err
			}
			if inputStatus != "pending" {
				return fmt.Errorf("awaiting_user finding %q references human input %q with status %q; a finding can only wait on a pending decision", findingID, disposition.HumanInputID, inputStatus)
			}
		}
		if attemptID != "" {
			var attemptModule, attemptRun string
			if err := db.QueryRowContext(ctx, `SELECT module, pulse_run_id FROM pulse_fix_attempts WHERE attempt_id=?`, attemptID).Scan(&attemptModule, &attemptRun); err != nil {
				if err == sql.ErrNoRows {
					return fmt.Errorf("no fix attempt %q", attemptID)
				}
				return err
			}
			if attemptModule != strings.TrimSpace(module) || attemptRun != strings.TrimSpace(pulseRunID) {
				return fmt.Errorf("fix attempt %q belongs to module=%q pulse_run_id=%q", attemptID, attemptModule, attemptRun)
			}
			var linked int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_fix_attempt_findings
				WHERE attempt_id=? AND fingerprint=?`, attemptID, fingerprint).Scan(&linked); err != nil {
				return err
			}
			if linked != 1 {
				return fmt.Errorf("fix attempt %q is not linked to concern %q", attemptID, fingerprint)
			}
		}

		verificationAttemptID := attemptID
		if verificationAttemptID == "" {
			// The historical uniqueness key starts with attempt_id. Give
			// non-attempt dispositions a run-scoped identity so the same check
			// accumulates across Pulse runs instead of overwriting one row.
			verificationAttemptID = "disposition:" + strings.TrimSpace(pulseRunID)
		}
		for _, verification := range disposition.Verification {
			if _, err := db.ExecContext(ctx, `INSERT INTO pulse_fix_verifications
				(attempt_id, fingerprint, check_text, verdict, expected, observed, evidence_json, verified_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(attempt_id, fingerprint, check_text) DO UPDATE SET
					verdict=excluded.verdict, expected=excluded.expected, observed=excluded.observed,
					evidence_json=excluded.evidence_json, verified_at=excluded.verified_at`,
				verificationAttemptID, fingerprint, verification.Check, verification.Verdict,
				verification.Expected, verification.Observed,
				lifecycleJSON(verification.Evidence), recordedAt); err != nil {
				return err
			}
		}

		status, eventType, resolvedBy := lifecycleStatusForDisposition(disposition.Disposition)
		resolvedAt := ""
		if status == ConcernStatusResolved || status == ConcernStatusRejected || status == ConcernStatusExternalActionRequired {
			resolvedAt = recordedAt
		}
		if _, err := db.ExecContext(ctx, `UPDATE run_concerns SET
			status=?, resolved_at=?, resolved_by=?, resolution_note=?
			WHERE fingerprint=?`,
			status, resolvedAt, resolvedBy, strings.TrimSpace(disposition.Summary), fingerprint); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]interface{}{
			"disposition":      disposition.Disposition,
			"changed_files":    normalizedLifecycleStrings(disposition.ChangedFiles),
			"before_refs":      normalizedLifecycleStrings(disposition.BeforeRefs),
			"after_refs":       normalizedLifecycleStrings(disposition.AfterRefs),
			"next_check":       strings.TrimSpace(disposition.NextCheck),
			"external_owner":   disposition.ExternalOwner,
			"reason_code":      disposition.ReasonCode,
			"reopen_condition": disposition.ReopenCondition,
		})
		if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_events
			(fingerprint, finding_id, pulse_run_id, attempt_id, event_type, summary, metadata_json, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(fingerprint, pulse_run_id, attempt_id, event_type) DO UPDATE SET
				summary=excluded.summary, metadata_json=excluded.metadata_json, recorded_at=excluded.recorded_at`,
			fingerprint, findingID, pulseRunID, attemptID, eventType,
			strings.TrimSpace(disposition.Summary), string(metadata), recordedAt); err != nil {
			return err
		}
		if attemptID != "" {
			if _, err := db.ExecContext(ctx, `UPDATE pulse_fix_attempt_findings SET
				finding_id=?, disposition=?, summary=?
				WHERE attempt_id=? AND fingerprint=?`,
				findingID, disposition.Disposition, strings.TrimSpace(disposition.Summary), attemptID, fingerprint); err != nil {
				return err
			}
			aggregate := attempts[attemptID]
			if aggregate == nil {
				aggregate = &attemptAggregate{status: "verified"}
				attempts[attemptID] = aggregate
			}
			aggregate.changedFiles = append(aggregate.changedFiles, disposition.ChangedFiles...)
			aggregate.beforeRefs = append(aggregate.beforeRefs, disposition.BeforeRefs...)
			aggregate.afterRefs = append(aggregate.afterRefs, disposition.AfterRefs...)
			if disposition.Disposition == FindingDispositionFailed {
				aggregate.status = "failed"
			} else if disposition.Disposition == FindingDispositionChangedUnverified && aggregate.status != "failed" {
				aggregate.status = ConcernStatusAwaitingVerification
			} else if aggregate.status != "failed" && aggregate.status != ConcernStatusAwaitingVerification &&
				disposition.Disposition != FindingDispositionFixedVerified {
				aggregate.status = "applied"
			}
		}
	}
	for attemptID, aggregate := range attempts {
		if _, err := db.ExecContext(ctx, `UPDATE pulse_fix_attempts SET
			status=?, changed_files_json=?, before_refs_json=?, after_refs_json=?, completed_at=?
			WHERE attempt_id=?`,
			aggregate.status, lifecycleJSON(aggregate.changedFiles), lifecycleJSON(aggregate.beforeRefs),
			lifecycleJSON(aggregate.afterRefs), recordedAt, attemptID); err != nil {
			return err
		}
	}
	return nil
}

func decodeLifecycleStrings(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	if values == nil {
		return []string{}
	}
	return values
}

// LoadPulseFindingLifecycles returns current issue state plus its complete fix
// and verification history. A module filter includes reviewer concerns filed by
// that module and any step concern linked to one of that module's fix attempts.
// A negative limit returns every matching finding; zero uses the bounded default
// for callers that explicitly want a preview.
func LoadPulseFindingLifecycles(ctx context.Context, workspacePath, module string, limit int) ([]PulseFindingLifecycle, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return []PulseFindingLifecycle{}, err
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 100
	}
	module = strings.TrimSpace(module)
	// Lead with the step carrying the most unresolved work, and keep its rows
	// together.
	//
	// Recency put whatever was filed last on top, which is arbitrary when a
	// whole cluster is filed in one run. Fingerprints hash the finding text, so
	// one schema mismatch files once per field it names: social-media held 39
	// concerns from execute-find-opportunities, all opportunities.json failing
	// its validation_schema. Interleaved by timestamp they read as unrelated
	// one-offs, and successive Bug Review passes worked the top of the list and
	// left all 39 untouched.
	//
	// Clustering only helped where it was applied. LoadOpenRunConcerns was
	// reordered first, but that backs get_pulse_module_state while the Fixer
	// reads this query through get_pulse_finding_backlog — so the fix landed on
	// a path the Fixer never reads and the backlog did not move.
	query := `SELECT c.fingerprint, c.step_id, c.phase, c.group_name, c.text,
			c.first_seen_run, c.first_seen_at, c.last_seen_run, c.last_seen_at, c.seen_count,
			c.status, c.resolution_note, COALESCE(d.detail_json, '')
		FROM run_concerns c
		LEFT JOIN pulse_finding_details d ON d.fingerprint=c.fingerprint
		LEFT JOIN (
			SELECT step_id, COUNT(*) AS active_count, MAX(seen_count) AS peak_seen
			FROM run_concerns
			WHERE status NOT IN ('resolved', 'rejected', 'external_action_required')
			GROUP BY step_id
		) cluster ON cluster.step_id = c.step_id
		WHERE ?='' OR c.step_id=? OR EXISTS (
			SELECT 1 FROM pulse_fix_attempt_findings af
			JOIN pulse_fix_attempts a ON a.attempt_id=af.attempt_id
			WHERE af.fingerprint=c.fingerprint AND a.module=?
		)
		ORDER BY COALESCE(cluster.active_count, 0) DESC, COALESCE(cluster.peak_seen, 0) DESC,
			c.step_id ASC, c.last_seen_at DESC, c.seen_count DESC`
	args := []interface{}{module, module, module}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PulseFindingLifecycle{}
	for rows.Next() {
		var finding PulseFindingLifecycle
		var detailJSON string
		if err := rows.Scan(&finding.Fingerprint, &finding.StepID, &finding.Phase, &finding.GroupName,
			&finding.Text, &finding.FirstSeenRun, &finding.FirstSeenAt, &finding.LastSeenRun,
			&finding.LastSeenAt, &finding.SeenCount, &finding.Status, &finding.ResolutionNote,
			&detailJSON); err != nil {
			return nil, err
		}
		if strings.TrimSpace(detailJSON) != "" {
			var details PulseFindingDetails
			if err := json.Unmarshal([]byte(detailJSON), &details); err == nil {
				details = normalizePulseFindingDetails(details)
				if details.IssueKind == "harness_issue" {
					platform, platformErr := loadPulseHarnessPlatformIssue(ctx, details.TargetKey)
					if platformErr != nil {
						return nil, platformErr
					}
					details.Platform = platform
				}
				finding.Details = &details
				finding.FindingID = details.FindingID
			}
		}
		if finding.Phase == ConcernPhaseReview {
			finding.Module = finding.StepID
		}
		finding.Attempts = []PulseFixAttempt{}
		finding.Verification = []PulseFindingVerification{}
		finding.Events = []PulseFindingEvent{}
		out = append(out, finding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for index := range out {
		fingerprint := out[index].Fingerprint
		attemptRows, err := db.QueryContext(ctx, `SELECT a.attempt_id, a.module, a.pulse_run_id, a.summary,
				a.status, a.intended_files_json, a.changed_files_json, a.before_refs_json,
				a.after_refs_json, a.started_at, a.completed_at, af.finding_id,
				af.disposition, af.summary
			FROM pulse_fix_attempt_findings af
			JOIN pulse_fix_attempts a ON a.attempt_id=af.attempt_id
			WHERE af.fingerprint=?
			ORDER BY a.started_at DESC`, fingerprint)
		if err != nil {
			return nil, err
		}
		for attemptRows.Next() {
			var attempt PulseFixAttempt
			var intendedJSON, changedJSON, beforeJSON, afterJSON, findingID, disposition, findingSummary string
			if err := attemptRows.Scan(&attempt.AttemptID, &attempt.Module, &attempt.PulseRunID,
				&attempt.Summary, &attempt.Status, &intendedJSON, &changedJSON, &beforeJSON,
				&afterJSON, &attempt.StartedAt, &attempt.CompletedAt, &findingID,
				&disposition, &findingSummary); err != nil {
				attemptRows.Close()
				return nil, err
			}
			attempt.IntendedFiles = decodeLifecycleStrings(intendedJSON)
			attempt.ChangedFiles = decodeLifecycleStrings(changedJSON)
			attempt.BeforeRefs = decodeLifecycleStrings(beforeJSON)
			attempt.AfterRefs = decodeLifecycleStrings(afterJSON)
			attempt.Findings = []PulseFixFindingRef{{
				Fingerprint: fingerprint,
				FindingID:   findingID,
				Disposition: disposition,
				Summary:     findingSummary,
			}}
			if out[index].FindingID == "" {
				out[index].FindingID = findingID
			}
			if out[index].Module == "" {
				out[index].Module = attempt.Module
			}
			out[index].Attempts = append(out[index].Attempts, attempt)
		}
		attemptRows.Close()

		verificationRows, err := db.QueryContext(ctx, `SELECT check_text, verdict, expected, observed,
				evidence_json, verified_at FROM pulse_fix_verifications
			WHERE fingerprint=? ORDER BY verified_at DESC`, fingerprint)
		if err != nil {
			return nil, err
		}
		for verificationRows.Next() {
			var verification PulseFindingVerification
			var evidenceJSON string
			if err := verificationRows.Scan(&verification.Check, &verification.Verdict,
				&verification.Expected, &verification.Observed, &evidenceJSON, &verification.At); err != nil {
				verificationRows.Close()
				return nil, err
			}
			verification.Evidence = decodeLifecycleStrings(evidenceJSON)
			out[index].Verification = append(out[index].Verification, verification)
		}
		verificationRows.Close()

		eventRows, err := db.QueryContext(ctx, `SELECT finding_id, event_type, summary, pulse_run_id, attempt_id,
				metadata_json, recorded_at FROM pulse_finding_events
			WHERE fingerprint=? ORDER BY recorded_at DESC`, fingerprint)
		if err != nil {
			return nil, err
		}
		for eventRows.Next() {
			var event PulseFindingEvent
			var metadataJSON string
			if err := eventRows.Scan(&event.FindingID, &event.EventType, &event.Summary, &event.PulseRunID,
				&event.AttemptID, &metadataJSON, &event.RecordedAt); err != nil {
				eventRows.Close()
				return nil, err
			}
			_ = json.Unmarshal([]byte(metadataJSON), &event.Metadata)
			if out[index].Status == ConcernStatusExternalActionRequired && event.EventType == "external_action_required" {
				if owner, ok := event.Metadata["external_owner"].(string); ok {
					out[index].ExternalOwner = owner
				}
				if reason, ok := event.Metadata["reason_code"].(string); ok {
					out[index].ReasonCode = reason
				}
				if condition, ok := event.Metadata["reopen_condition"].(string); ok {
					out[index].ReopenCondition = condition
				}
			}
			if out[index].FindingID == "" && event.FindingID != "" {
				out[index].FindingID = event.FindingID
			}
			out[index].Events = append(out[index].Events, event)
		}
		eventRows.Close()
		out[index].Issue = NewPulseIssue(out[index])
		// Ordinary and migrated concerns predate reviewer-assigned IDs. Expose
		// the deterministic compact issue ID through the legacy lifecycle field
		// too, because fixer write tools use finding_id as the durable human
		// address while fingerprint remains the internal matching key.
		if strings.TrimSpace(out[index].FindingID) == "" {
			out[index].FindingID = out[index].Issue.ID
		}
	}
	return out, nil
}
