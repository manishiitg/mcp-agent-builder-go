package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
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

// The closed sets below are the single source of truth for both the accept
// check and the rejection message. A rejection that does not name its members
// cannot be converged on: the Fixer wrote external_owner "shared workflow
// runtime" and then "RTS dev voice..." across two live runs, both meaning
// platform, because nothing in the error said the set was closed or what was
// in it.
var (
	pulseFindingDispositionValues = []string{
		FindingDispositionFixedVerified,
		FindingDispositionVerifiedNoChange,
		FindingDispositionChangedUnverified,
		FindingDispositionProposalOnly,
		FindingDispositionAwaitingUser,
		FindingDispositionAwaitingRun,
		FindingDispositionBlocked,
		FindingDispositionExternalAction,
		FindingDispositionFailed,
		FindingDispositionRejected,
	}
	pulseVerificationVerdictValues = []string{
		VerificationPassed,
		VerificationFailed,
		VerificationInconclusive,
	}
	pulseExternalOwnerValues = []string{"platform", "user", "vendor", "workflow_owner"}
)

func pulseAllowed(values []string) string {
	return strings.Join(values, ", ")
}

func pulseValueAllowed(value string, values []string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

// pulseFieldArrival records what actually reached the validator for one field.
// Naming only the first missing field forces the caller to rediscover the
// contract one rejected write at a time, which is exactly how a completed fix
// ends up unrecorded.
type pulseFieldArrival struct {
	Name  string
	State string
}

func pulseStringArrival(name, value string) pulseFieldArrival {
	if strings.TrimSpace(value) == "" {
		return pulseFieldArrival{Name: name, State: "missing"}
	}
	return pulseFieldArrival{Name: name, State: "set"}
}

func pulseListArrival(name string, values []string) pulseFieldArrival {
	if len(values) == 0 {
		return pulseFieldArrival{Name: name, State: "missing"}
	}
	return pulseFieldArrival{Name: name, State: fmt.Sprintf("%d items", len(values))}
}

func pulseArrivalReport(arrivals ...pulseFieldArrival) string {
	parts := make([]string, 0, len(arrivals))
	for _, arrival := range arrivals {
		parts = append(parts, arrival.Name+"="+arrival.State)
	}
	return strings.Join(parts, ", ")
}

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

// PulseReviewVerificationCandidate is the backend-owned allowlist entry for a
// reviewer checking a prior fix. A review may emit PULSE_VERIFICATION_JSON only
// for one of these exact tuples; ordinary open, blocked, rejected, and already
// resolved findings have no attempt to verify.
type PulseReviewVerificationCandidate struct {
	FindingID   string `json:"finding_id"`
	Fingerprint string `json:"fingerprint"`
	AttemptID   string `json:"attempt_id"`
	NextCheck   string `json:"next_check"`
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
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	if err := migratePreValidationConcernGranularity(ctx, db); err != nil {
		return err
	}
	if err := migrateDuplicatePulseFindingIdentities(ctx, db); err != nil {
		return err
	}
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_pulse_fix_attempts_module_run ON pulse_fix_attempts(module, pulse_run_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_fix_findings_fingerprint ON pulse_fix_attempt_findings(fingerprint, attempt_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_fix_verifications_fingerprint ON pulse_fix_verifications(fingerprint, verified_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_finding_events_fingerprint ON pulse_finding_events(fingerprint, recorded_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_pulse_finding_details_finding_id ON pulse_finding_details(lower(finding_id)) WHERE finding_id<>''`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_finding_details_target_key ON pulse_finding_details(target_key) WHERE target_key<>''`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

// migratePreValidationConcernGranularity folds the legacy one-row-per-failed-
// field shape into one lifecycle finding per step. Individual field failures
// remain in pulse_finding_events and the retained per-run pre_validation.json
// logs; the active issue tracker represents the one output-contract repair.
func migratePreValidationConcernGranularity(ctx context.Context, db pulseFindingLifecycleDB) error {
	type legacyRow struct {
		Fingerprint  string
		StepID       string
		FirstSeenRun string
		LastSeenRun  string
	}
	rows, err := db.QueryContext(ctx, `SELECT fingerprint, step_id, first_seen_run, last_seen_run
		FROM run_concerns WHERE phase=? ORDER BY step_id, first_seen_at, fingerprint`, ConcernPhasePreValidation)
	if err != nil {
		return err
	}
	groups := map[string][]legacyRow{}
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.Fingerprint, &row.StepID, &row.FirstSeenRun, &row.LastSeenRun); err != nil {
			rows.Close()
			return err
		}
		groups[row.StepID] = append(groups[row.StepID], row)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for stepID, legacy := range groups {
		target := preValidationConcernFingerprint(stepID)
		if len(legacy) == 1 && legacy[0].Fingerprint == target {
			continue
		}
		identityRows := make([]pulseIdentityRow, 0, len(legacy))
		runs := map[string]bool{}
		fingerprints := make([]string, 0, len(legacy))
		for _, row := range legacy {
			identityRows = append(identityRows, pulseIdentityRow{Fingerprint: row.Fingerprint, StepID: stepID})
			fingerprints = append(fingerprints, row.Fingerprint)
			if strings.TrimSpace(row.FirstSeenRun) != "" {
				runs[row.FirstSeenRun] = true
			}
			if strings.TrimSpace(row.LastSeenRun) != "" {
				runs[row.LastSeenRun] = true
			}
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(fingerprints)), ",")
		args := make([]interface{}, 0, len(fingerprints))
		for _, fingerprint := range fingerprints {
			args = append(args, fingerprint)
		}
		eventRows, err := db.QueryContext(ctx, `SELECT DISTINCT pulse_run_id FROM pulse_finding_events
			WHERE fingerprint IN (`+placeholders+`) AND pulse_run_id<>''`, args...)
		if err != nil {
			return err
		}
		for eventRows.Next() {
			var runID string
			if err := eventRows.Scan(&runID); err != nil {
				eventRows.Close()
				return err
			}
			runs[runID] = true
		}
		if err := eventRows.Close(); err != nil {
			return err
		}
		if err := mergePulseIdentityGroup(ctx, db, target, identityRows); err != nil {
			return err
		}
		seenCount := len(runs)
		if seenCount == 0 {
			seenCount = 1
		}
		text := fmt.Sprintf("prevalidation gate failed for the step output contract; %d legacy field-level finding(s) were consolidated. Inspect the linked event history and per-run pre_validation.json for every failed check", len(legacy))
		if _, err := db.ExecContext(ctx, `UPDATE run_concerns SET text=?, phase=?, seen_count=? WHERE fingerprint=?`,
			text, ConcernPhasePreValidation, seenCount, target); err != nil {
			return err
		}
	}
	return nil
}

type pulseIdentityRow struct {
	Fingerprint string
	FindingID   string
	TargetKey   string
	StepID      string
}

func migrateDuplicatePulseFindingIdentities(ctx context.Context, db pulseFindingLifecycleDB) error {
	rows, err := db.QueryContext(ctx, `SELECT d.fingerprint, d.finding_id, d.target_key, COALESCE(c.step_id, '')
		FROM pulse_finding_details d LEFT JOIN run_concerns c USING (fingerprint)
		ORDER BY d.fingerprint`)
	if err != nil {
		return err
	}
	var identities []pulseIdentityRow
	for rows.Next() {
		var row pulseIdentityRow
		if err := rows.Scan(&row.Fingerprint, &row.FindingID, &row.TargetKey, &row.StepID); err != nil {
			rows.Close()
			return err
		}
		identities = append(identities, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, field := range []string{"finding_id"} {
		groups := map[string][]pulseIdentityRow{}
		for _, row := range identities {
			value := row.FindingID
			if field == "target_key" {
				value = row.TargetKey
			}
			value = strings.TrimSpace(value)
			if value != "" {
				groups[strings.ToLower(value)] = append(groups[strings.ToLower(value)], row)
			}
		}
		for _, group := range groups {
			marker := pulseFindingDetailMarker{PulseFindingDetails: PulseFindingDetails{FindingID: group[0].FindingID, TargetKey: group[0].TargetKey}}
			target := pulseFindingCanonicalFingerprint(group[0].StepID, marker)
			if len(group) == 1 && group[0].Fingerprint == target {
				continue
			}
			if err := mergePulseIdentityGroup(ctx, db, target, group); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergePulseIdentityGroup(ctx context.Context, db pulseFindingLifecycleDB, target string, group []pulseIdentityRow) error {
	initialized := false
	for _, row := range group {
		if row.Fingerprint == target {
			initialized = true
			break
		}
	}
	for _, row := range group {
		old := row.Fingerprint
		if old == target {
			continue
		}
		if !initialized {
			if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO run_concerns
				SELECT ?, step_id, phase, group_name, text, first_seen_run, first_seen_at, last_seen_run, last_seen_at, seen_count, status, resolved_at, resolved_by, resolution_note
				FROM run_concerns WHERE fingerprint=?`, target, old); err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO pulse_finding_details
				SELECT ?, finding_id, issue_kind, target_key, detail_json, source_run_id, updated_at
				FROM pulse_finding_details WHERE fingerprint=?`, target, old); err != nil {
				return err
			}
			initialized = true
		} else {
			if _, err := db.ExecContext(ctx, `UPDATE run_concerns SET
				seen_count=seen_count+COALESCE((SELECT seen_count FROM run_concerns WHERE fingerprint=?),0),
				first_seen_at=MIN(first_seen_at, COALESCE((SELECT first_seen_at FROM run_concerns WHERE fingerprint=?),first_seen_at)),
				last_seen_at=MAX(last_seen_at, COALESCE((SELECT last_seen_at FROM run_concerns WHERE fingerprint=?),last_seen_at)),
				last_seen_run=COALESCE((SELECT last_seen_run FROM run_concerns WHERE fingerprint=?),last_seen_run),
				text=COALESCE((SELECT text FROM run_concerns WHERE fingerprint=?),text),
				status=CASE WHEN (SELECT status FROM run_concerns WHERE fingerprint=?) IN ('open','fixing','awaiting_verification','awaiting_run','external_action_required') THEN (SELECT status FROM run_concerns WHERE fingerprint=?) ELSE status END
				WHERE fingerprint=?`, old, old, old, old, old, old, old, target); err != nil {
				return err
			}
		}
		for _, stmt := range []string{
			`INSERT OR IGNORE INTO pulse_fix_attempt_findings SELECT attempt_id, ?, finding_id, disposition, summary FROM pulse_fix_attempt_findings WHERE fingerprint=?`,
			`INSERT OR IGNORE INTO pulse_fix_verifications (attempt_id,fingerprint,check_text,verdict,expected,observed,evidence_json,verified_at) SELECT attempt_id,?,check_text,verdict,expected,observed,evidence_json,verified_at FROM pulse_fix_verifications WHERE fingerprint=?`,
		} {
			if _, err := db.ExecContext(ctx, stmt, target, old); err != nil {
				return err
			}
		}
		// Preserve every historical event. If the canonical row already has the
		// same unique event tuple, retain the twin as an explicit migration event
		// instead of silently dropping it through INSERT OR IGNORE.
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_finding_events
			(fingerprint,finding_id,pulse_run_id,attempt_id,event_type,summary,metadata_json,recorded_at)
			SELECT ?,e.finding_id,e.pulse_run_id,e.attempt_id,e.event_type||':identity_merge:'||substr(e.fingerprint,1,8),e.summary,e.metadata_json,e.recorded_at
			FROM pulse_finding_events e WHERE e.fingerprint=? AND EXISTS (
				SELECT 1 FROM pulse_finding_events t WHERE t.fingerprint=? AND t.pulse_run_id=e.pulse_run_id AND t.attempt_id=e.attempt_id AND t.event_type=e.event_type
			)`, target, old, target); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_finding_events
			(fingerprint,finding_id,pulse_run_id,attempt_id,event_type,summary,metadata_json,recorded_at)
			SELECT ?,finding_id,pulse_run_id,attempt_id,event_type,summary,metadata_json,recorded_at FROM pulse_finding_events WHERE fingerprint=?`, target, old); err != nil {
			return err
		}
		for _, stmt := range []string{
			`DELETE FROM pulse_fix_attempt_findings WHERE fingerprint=?`,
			`DELETE FROM pulse_fix_verifications WHERE fingerprint=?`,
			`DELETE FROM pulse_finding_events WHERE fingerprint=?`,
			`DELETE FROM pulse_finding_details WHERE fingerprint=?`,
			`DELETE FROM run_concerns WHERE fingerprint=?`,
		} {
			if _, err := db.ExecContext(ctx, stmt, old); err != nil {
				return err
			}
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

// pulseDispositionOpensAttempt reports whether a disposition asserts that files
// were changed, which is the only case that needs a fix-attempt record created
// for it when none already exists.
func pulseDispositionOpensAttempt(disposition string) bool {
	switch strings.TrimSpace(disposition) {
	case FindingDispositionFixedVerified, FindingDispositionChangedUnverified:
		return true
	default:
		return false
	}
}

// pulseDispositionSettlesAttempt reports whether a disposition is a verdict on
// an attempt already in flight. These reuse the module's open attempt instead of
// opening a second one, so a later run verifying an earlier changed_unverified
// fix closes the attempt that made the change rather than inventing a fresh
// record for work already done.
func pulseDispositionSettlesAttempt(disposition string) bool {
	switch strings.TrimSpace(disposition) {
	case FindingDispositionFixedVerified, FindingDispositionChangedUnverified,
		FindingDispositionVerifiedNoChange, FindingDispositionFailed:
		return true
	default:
		return false
	}
}

// resolvePulseFixAttemptTx returns the attempt a disposition is recorded
// against, creating one when the disposition claims changed files and the module
// has no open attempt for this finding.
//
// The agent used to declare this itself through a start_pulse_fix_attempt tool
// call before mutating. That write-ahead bought one narrow property — the
// declaration preceded the mutation — and cost a whole error class: an attempt
// id the agent had to carry across turns, and two rejections ("requires
// attempt_id and changed_files", "no fix attempt %q for finding %q") that fired
// after the repair had already been made and could not then be recorded. Nothing
// reads an orphaned attempt: there is no reaper, no incomplete-attempt query, and
// no crash-recovery path, so the ordering guarantee was never load-bearing. The
// record is now created here, at the same moment the disposition that describes
// it is written.
func resolvePulseFixAttemptTx(
	ctx context.Context,
	db pulseFindingLifecycleDB,
	module, pulseRunID, concernStatus string,
	disposition PulseFindingDisposition,
	recordedAt string,
) (string, error) {
	if !pulseDispositionSettlesAttempt(disposition.Disposition) {
		return "", nil
	}
	rows, err := db.QueryContext(ctx, `SELECT a.attempt_id, a.module
		FROM pulse_fix_attempt_findings af
		JOIN pulse_fix_attempts a ON a.attempt_id=af.attempt_id
		WHERE af.fingerprint=? AND a.status IN (?, ?)
		ORDER BY a.started_at DESC`,
		disposition.Fingerprint, ConcernStatusFixing, ConcernStatusAwaitingVerification)
	if err != nil {
		return "", err
	}
	existing := ""
	for rows.Next() {
		var attemptID, attemptModule string
		if err := rows.Scan(&attemptID, &attemptModule); err != nil {
			rows.Close()
			return "", err
		}
		// An attempt belongs to the module that made it; letting another module
		// settle it would let one module take credit for work it did not do.
		if existing == "" && pulsemodules.Normalize(attemptModule) == module {
			existing = attemptID
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	if !pulseDispositionOpensAttempt(disposition.Disposition) {
		return "", nil
	}
	if concernStatus == ConcernStatusRejected {
		return "", fmt.Errorf("concern %q was rejected and cannot enter fixing without new adjudication; re-file it with new evidence before recording a fix for finding %q",
			disposition.Fingerprint, disposition.FindingID)
	}

	findings := []PulseFixFindingRef{{Fingerprint: disposition.Fingerprint, FindingID: disposition.FindingID}}
	attemptID := lifecycleAttemptID(pulseRunID, module, findings, disposition.ChangedFiles, disposition.BeforeRefs)
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_fix_attempts
		(attempt_id, module, pulse_run_id, summary, status, intended_files_json, before_refs_json, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(attempt_id) DO NOTHING`,
		attemptID, module, strings.TrimSpace(pulseRunID), disposition.Summary,
		ConcernStatusFixing, lifecycleJSON(disposition.ChangedFiles),
		lifecycleJSON(disposition.BeforeRefs), recordedAt); err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_fix_attempt_findings
		(attempt_id, fingerprint, finding_id) VALUES (?, ?, ?)
		ON CONFLICT(attempt_id, fingerprint) DO UPDATE SET finding_id=excluded.finding_id`,
		attemptID, disposition.Fingerprint, disposition.FindingID); err != nil {
		return "", err
	}
	metadata, _ := json.Marshal(map[string]interface{}{
		"intended_files": normalizedLifecycleStrings(disposition.ChangedFiles),
		"before_refs":    normalizedLifecycleStrings(disposition.BeforeRefs),
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_events
		(fingerprint, finding_id, pulse_run_id, attempt_id, event_type, summary, metadata_json, recorded_at)
		VALUES (?, ?, ?, ?, 'fix_started', ?, ?, ?)
		ON CONFLICT(fingerprint, pulse_run_id, attempt_id, event_type) DO NOTHING`,
		disposition.Fingerprint, disposition.FindingID, pulseRunID, attemptID,
		disposition.Summary, string(metadata), recordedAt); err != nil {
		return "", err
	}
	return attemptID, nil
}

// validateFindingDisposition reports every structural problem with one
// disposition, not just the first.
//
// It used to return on the first violation. On 2026-08-04 one finding
// (PUL-70B1057E) took four sequential record_pulse_result rejections over 24
// minutes to satisfy: wrong disposition value, then a verification proof that
// didn't match the reviewer's evidence, then a next_check that didn't match the
// reviewer's boundary text (all from validateReviewerVerificationDispositions,
// which has the same fix below), and only after all three passed did this
// function's own missing-changed_files check surface — a structural problem
// that had been true the entire time but was never reached. Four round trips to
// learn four facts a single rejection could have stated together, on ONE
// finding.
//
// That anti-pattern already had one fix in this codebase: the result=changed
// check three functions away in pulse_worklist.go merged three separate
// "X is required" rejections into one message naming the whole required set.
// This applies the same treatment here.
func validateFindingDisposition(disposition PulseFindingDisposition) error {
	disposition = NormalizePulseFindingDisposition(disposition)
	var problems []string
	add := func(format string, args ...interface{}) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if disposition.Fingerprint == "" || disposition.FindingID == "" {
		add("requires both fingerprint and finding_id (got %s); copy both from the same get_pulse_state(view=\"backlog\") item, where finding_id is issue.id",
			pulseArrivalReport(pulseStringArrival("fingerprint", disposition.Fingerprint), pulseStringArrival("finding_id", disposition.FindingID)))
	}
	if disposition.Summary == "" {
		add("requires summary: one sentence stating what was done and what it means for this finding")
	}
	dispositionKnown := pulseValueAllowed(disposition.Disposition, pulseFindingDispositionValues)
	if !dispositionKnown {
		add("has invalid disposition %q. Must be one of: %s", disposition.Disposition, pulseAllowed(pulseFindingDispositionValues))
	}

	passed, failed, inconclusive := 0, 0, 0
	for index, verification := range disposition.Verification {
		if strings.TrimSpace(verification.Check) == "" {
			add("verification[%d] requires check naming what was run or inspected; each verification entry is {\"check\", \"verdict\", \"expected\", \"observed\", \"evidence\"} where check and verdict are required and verdict is one of: %s",
				index, pulseAllowed(pulseVerificationVerdictValues))
			continue
		}
		switch strings.TrimSpace(verification.Verdict) {
		case VerificationPassed:
			passed++
		case VerificationFailed:
			failed++
		case VerificationInconclusive:
			inconclusive++
		default:
			add("verification[%d] has invalid verdict %q. Must be one of: %s", index, verification.Verdict, pulseAllowed(pulseVerificationVerdictValues))
		}
	}
	verdictCounts := fmt.Sprintf("passed=%d, failed=%d, inconclusive=%d", passed, failed, inconclusive)

	// Type-specific rules only apply once the disposition value itself is
	// known-good — checking them against an invalid value would report
	// requirements for a type the agent may not have meant.
	if dispositionKnown {
		switch disposition.Disposition {
		case FindingDispositionFixedVerified:
			if len(disposition.ChangedFiles) == 0 {
				add("fixed_verified requires changed_files (got %s): the exact workspace-relative files this fix changed. Use verified_no_change when a check proved the problem is absent without changing any file",
					pulseArrivalReport(pulseListArrival("changed_files", disposition.ChangedFiles)))
			}
			if len(disposition.BeforeRefs) != len(disposition.AfterRefs) {
				add("fixed_verified requires before_refs and after_refs as equal-length positional pairs (got before_refs=%d, after_refs=%d); supply the matching after_ref for each before_ref, or omit both arrays",
					len(disposition.BeforeRefs), len(disposition.AfterRefs))
			}
			if passed == 0 || failed > 0 || inconclusive > 0 {
				add("fixed_verified requires at least one passed verification and no failed or inconclusive check (got %s). Use changed_unverified when the proof has not arrived yet, or failed when a check failed", verdictCounts)
			}
		case FindingDispositionChangedUnverified:
			if len(disposition.ChangedFiles) == 0 {
				add("changed_unverified requires changed_files (got %s): the exact workspace-relative files this fix changed. Use awaiting_run when nothing was changed and only a scheduled run can produce the evidence",
					pulseArrivalReport(pulseListArrival("changed_files", disposition.ChangedFiles)))
			}
			if len(disposition.BeforeRefs) != len(disposition.AfterRefs) {
				add("changed_unverified requires before_refs and after_refs as equal-length positional pairs (got before_refs=%d, after_refs=%d); supply the matching after_ref for each before_ref, or omit both arrays",
					len(disposition.BeforeRefs), len(disposition.AfterRefs))
			}
			// next_check names the evidence that will settle this. Without it the
			// next reviewer cannot tell whether the producing run has happened, so
			// the finding is re-attempted instead of verified — rtslatency held one
			// at seen_count 4, still awaiting_verification, because each pass
			// re-fixed it rather than checking the run that had since occurred.
			if disposition.NextCheck == "" {
				add("changed_unverified requires next_check naming the run, table, or artifact whose arrival proves or disproves this fix")
			}
			if inconclusive == 0 || failed > 0 {
				add("changed_unverified requires at least one inconclusive verification and no failed check (got %s). A passed-only result is fixed_verified and any failed check makes this failed", verdictCounts)
			}
		case FindingDispositionVerifiedNoChange:
			if passed == 0 || failed > 0 || inconclusive > 0 {
				add("verified_no_change requires at least one passed verification and no failed or inconclusive check (got %s). verified_no_change means a check proved this is not (or is no longer) a problem without changing any file", verdictCounts)
			}
		case FindingDispositionFailed:
			if len(disposition.Verification) > 0 && failed == 0 {
				add("failed supplied %d verification entries but none with verdict %q (got %s); include the check that failed, or omit verification entirely when no check was run",
					len(disposition.Verification), VerificationFailed, verdictCounts)
			}
		case FindingDispositionAwaitingRun:
			// Naming the evidence boundary is what separates waiting from stalling:
			// without it nobody can tell whether the run that would resolve this has
			// already happened.
			if disposition.NextCheck == "" {
				add("awaiting_run requires next_check naming the run or evidence that will resolve it")
			}
			if len(disposition.ChangedFiles) > 0 {
				add("awaiting_run changed files; a finding with a fix applied is changed_unverified, not awaiting_run")
			}
		case FindingDispositionAwaitingUser:
			// A finding cannot wait on a decision nobody was asked for. Requiring
			// the question id here is what turns "awaiting_user" from a label into
			// something the operator can actually act on.
			if disposition.HumanInputID == "" {
				add("awaiting_user requires human_input_id: create the decision with create_human_input_request first, or use blocked/proposal_only if no question is being asked")
			}
		case FindingDispositionExternalAction:
			if disposition.ExternalOwner == "" || disposition.ReasonCode == "" || disposition.ReopenCondition == "" {
				add("external_action_required requires all of external_owner, reason_code, and reopen_condition (got %s). external_owner must be one of: %s. reason_code is a stable slug such as missing_platform_tool, permission_boundary, vendor_issue, policy, or accepted_risk, and reopen_condition names the evidence or capability that would make this actionable again",
					pulseArrivalReport(
						pulseStringArrival("external_owner", disposition.ExternalOwner),
						pulseStringArrival("reason_code", disposition.ReasonCode),
						pulseStringArrival("reopen_condition", disposition.ReopenCondition)),
					pulseAllowed(pulseExternalOwnerValues))
			} else if !pulseValueAllowed(disposition.ExternalOwner, pulseExternalOwnerValues) {
				add("external_action_required has invalid external_owner %q. Must be one of: %s. Use \"platform\" for shared runtime, harness, bridge, or product-side issues; \"user\" for a decision only the operator can make; \"vendor\" for a third-party service; \"workflow_owner\" for another workflow's own configuration",
					disposition.ExternalOwner, pulseAllowed(pulseExternalOwnerValues))
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return FormatPulseDispositionProblems(disposition.FindingID, problems)
}

// FormatPulseDispositionProblems joins every violation for one finding into a
// single numbered message, so one rejection can be fixed in one retry instead
// of one violation per round trip. Exported for pulse_worklist.go's reviewer
// cross-validation, which needs the identical combined-message shape.
func FormatPulseDispositionProblems(findingID string, problems []string) error {
	if len(problems) == 1 {
		return fmt.Errorf("finding %q %s", findingID, problems[0])
	}
	var b strings.Builder
	fmt.Fprintf(&b, "finding %q has %d problems — fix all of them before retrying:", findingID, len(problems))
	for i, problem := range problems {
		fmt.Fprintf(&b, "\n%d) %s", i+1, problem)
	}
	return errors.New(b.String())
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
	module = pulsemodules.Normalize(module)
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
		var concernStatus string
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(status), '') FROM run_concerns WHERE fingerprint=?`,
			fingerprint).Scan(&concernExists, &concernStatus); err != nil {
			return err
		}
		if concernExists != 1 {
			return fmt.Errorf("no concern with fingerprint %q for finding %q; fingerprint must be copied verbatim from a get_pulse_state(view=\"backlog\") item's fingerprint field, not from its issue.id", fingerprint, findingID)
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
					return fmt.Errorf("no fix attempt %q for finding %q; omit attempt_id and the backend opens or reuses the right attempt for this module and finding", attemptID, findingID)
				}
				return err
			}
			// The module must match: an attempt belongs to the module that made
			// it, and letting another module close it would let one module take
			// credit for work it did not do.
			//
			// The run deliberately need not match. changed_unverified exists
			// precisely so a fix whose proof needs a future run is recorded now
			// and verified later — fix-verification, post-run-monitor and the
			// Fixer contract all instruct exactly that, with reason
			// awaiting_next_valid_run. Requiring the attempt to belong to the
			// closing run made that impossible: the evidence arrives a run or
			// more after the attempt, and the disposition that would record it
			// was rejected as belonging to a previous Pulse run. social-media hit
			// this on 2026-08-01 and correctly preserved the unresolved state
			// rather than forcing a second lifecycle write, which would have
			// invented a fresh attempt for work already done.
			if pulsemodules.Normalize(attemptModule) != module {
				return fmt.Errorf("fix attempt %q belongs to module %q, not %q", attemptID, attemptModule, module)
			}
			var linked int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_fix_attempt_findings
				WHERE attempt_id=? AND fingerprint=?`, attemptID, fingerprint).Scan(&linked); err != nil {
				return err
			}
			if linked != 1 {
				return fmt.Errorf("fix attempt %q is not linked to concern %q (finding %q); an attempt can only settle the findings it was opened for, so omit attempt_id and let the backend open one for this finding", attemptID, fingerprint, findingID)
			}
		} else {
			resolved, resolveErr := resolvePulseFixAttemptTx(ctx, db, module, pulseRunID, concernStatus, disposition, recordedAt)
			if resolveErr != nil {
				return resolveErr
			}
			attemptID = resolved
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

// LoadPulseReviewVerificationCandidates returns the exact changed_unverified
// attempts a module is allowed to verify. Evidence arrival is still a reviewer
// judgment, but attempt eligibility is not: it is derived from the lifecycle
// tables and cannot be widened by caller-authored prose.
func LoadPulseReviewVerificationCandidates(
	ctx context.Context,
	workspacePath, module string,
) ([]PulseReviewVerificationCandidate, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return []PulseReviewVerificationCandidate{}, err
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return nil, err
	}

	module = pulsemodules.Normalize(module)
	acceptedModules := []string{}
	for _, candidate := range append(pulsemodules.IDs(), pulsemodules.RetiredIDs...) {
		if pulsemodules.Normalize(candidate) == module {
			acceptedModules = append(acceptedModules, candidate)
		}
	}
	if len(acceptedModules) == 0 {
		return []PulseReviewVerificationCandidate{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(acceptedModules)), ",")
	args := make([]interface{}, 0, len(acceptedModules))
	for _, accepted := range acceptedModules {
		args = append(args, accepted)
	}
	rows, err := db.QueryContext(ctx, `SELECT af.finding_id, af.fingerprint, a.attempt_id
		FROM pulse_fix_attempt_findings af
		JOIN pulse_fix_attempts a ON a.attempt_id=af.attempt_id
		JOIN run_concerns c ON c.fingerprint=af.fingerprint
		WHERE af.disposition=? AND a.status=? AND c.status=?
			AND a.module IN (`+placeholders+`)
		ORDER BY a.completed_at ASC, a.started_at ASC, af.finding_id ASC`,
		append([]interface{}{
			FindingDispositionChangedUnverified,
			ConcernStatusAwaitingVerification,
			ConcernStatusAwaitingVerification,
		}, args...)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PulseReviewVerificationCandidate{}
	for rows.Next() {
		var candidate PulseReviewVerificationCandidate
		if err := rows.Scan(&candidate.FindingID, &candidate.Fingerprint, &candidate.AttemptID); err != nil {
			return nil, err
		}
		var metadataJSON string
		if err := db.QueryRowContext(ctx, `SELECT metadata_json FROM pulse_finding_events
			WHERE fingerprint=? AND attempt_id=? AND event_type='verification_inconclusive'
			ORDER BY recorded_at DESC LIMIT 1`, candidate.Fingerprint, candidate.AttemptID).Scan(&metadataJSON); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		metadata := map[string]interface{}{}
		_ = json.Unmarshal([]byte(metadataJSON), &metadata)
		candidate.NextCheck, _ = metadata["next_check"].(string)
		candidate.NextCheck = strings.TrimSpace(candidate.NextCheck)
		out = append(out, candidate)
	}
	return out, rows.Err()
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
	// "pulse_fixer" is the consolidated Fixer's module sentinel, meaning every
	// due module rather than a module of that name. The write path has always
	// understood it; this read path did not, and `module` here is a plain
	// step_id filter. No concern carries step_id "pulse_fixer" and no attempt is
	// recorded under it, so the tool that hands the Fixer its queue returned an
	// empty list for the exact value its own contract tells it to pass — 0 rows
	// against 149 for an omitted module on social-media, 2026-08-01. The Fixer
	// only did any work at all by falling back to omitting the filter.
	if module == pulsemodules.PseudoPulseFixerID {
		module = ""
	}
	workflowReviewFilter := 0
	if module == pulsemodules.WorkflowReviewID {
		workflowReviewFilter = 1
	}
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
	// reordered first, but that backs get_pulse_state(view="module") while the
	// Fixer reads this query through view="backlog" — so the fix landed on
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
		WHERE ?='' OR c.step_id=? OR (?=1 AND c.phase='review' AND c.step_id IN (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)) OR EXISTS (
			SELECT 1 FROM pulse_fix_attempt_findings af
			JOIN pulse_fix_attempts a ON a.attempt_id=af.attempt_id
			WHERE af.fingerprint=c.fingerprint AND (a.module=? OR (?=1 AND a.module IN (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)))
		)
		ORDER BY COALESCE(cluster.active_count, 0) DESC, COALESCE(cluster.peak_seen, 0) DESC,
			c.step_id ASC, c.last_seen_at DESC, c.seen_count DESC`
	legacyWorkflowReviewModules := make([]interface{}, 0, len(pulsemodules.RetiredIDs))
	for _, retired := range pulsemodules.RetiredIDs {
		legacyWorkflowReviewModules = append(legacyWorkflowReviewModules, retired)
	}
	args := []interface{}{module, module, workflowReviewFilter}
	args = append(args, legacyWorkflowReviewModules...)
	args = append(args, module, workflowReviewFilter)
	args = append(args, legacyWorkflowReviewModules...)
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
			finding.Module = pulsemodules.Normalize(finding.StepID)
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
