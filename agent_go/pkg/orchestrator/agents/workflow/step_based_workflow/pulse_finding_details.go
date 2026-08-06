package step_based_workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
)

const pulseFindingDetailsSchema = `CREATE TABLE IF NOT EXISTS pulse_finding_details (
	fingerprint TEXT PRIMARY KEY,
	finding_id TEXT NOT NULL DEFAULT '',
	issue_kind TEXT NOT NULL DEFAULT '',
	target_key TEXT NOT NULL DEFAULT '',
	detail_json TEXT NOT NULL DEFAULT '{}',
	source_run_id TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
)`

const pulseFindingJSONPrefix = "PULSE_FINDING_JSON:"

const pulsePlatformHarnessIssuesSchema = `CREATE TABLE IF NOT EXISTS platform_harness_issues (
	issue_key TEXT PRIMARY KEY,
	finding_id TEXT NOT NULL DEFAULT '',
	severity TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	detail_json TEXT NOT NULL DEFAULT '{}',
	first_seen_at TEXT NOT NULL,
	last_seen_at TEXT NOT NULL,
	seen_count INTEGER NOT NULL DEFAULT 1
)`

const pulsePlatformHarnessOccurrencesSchema = `CREATE TABLE IF NOT EXISTS platform_harness_occurrences (
	issue_key TEXT NOT NULL,
	workspace_path TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	module TEXT NOT NULL DEFAULT '',
	source_run_id TEXT NOT NULL DEFAULT '',
	first_seen_at TEXT NOT NULL,
	last_seen_at TEXT NOT NULL,
	seen_count INTEGER NOT NULL DEFAULT 1,
	PRIMARY KEY (issue_key, workspace_path, fingerprint)
)`

// PulseFindingReproduction is inert evidence: the UI renders it for humans but
// never executes Action. Safe only means the reviewer proved the described
// reproduction is side-effect-free.
type PulseFindingReproduction struct {
	Safe        bool   `json:"safe"`
	Setup       string `json:"setup,omitempty"`
	Action      string `json:"action,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Observed    string `json:"observed,omitempty"`
	Limitations string `json:"limitations,omitempty"`
}

// PulseFindingDetails carries the structured part of a reviewer finding that
// would otherwise be trapped in forensic Markdown. issue_kind=harness_issue is
// rendered as a dedicated platform-owned card in Pulse.
type PulseFindingDetails struct {
	FindingID        string                     `json:"finding_id,omitempty"`
	TargetKey        string                     `json:"target_key,omitempty"`
	IssueKind        string                     `json:"issue_kind,omitempty"`
	RecommendedRoute string                     `json:"recommended_route,omitempty"`
	NextCheck        string                     `json:"next_check,omitempty"`
	Classification   string                     `json:"classification,omitempty"`
	Severity         string                     `json:"severity,omitempty"`
	Summary          string                     `json:"summary,omitempty"`
	Impact           string                     `json:"impact,omitempty"`
	Workaround       string                     `json:"workaround,omitempty"`
	Evidence         []string                   `json:"evidence,omitempty"`
	Reproduction     PulseFindingReproduction   `json:"reproduction"`
	Platform         *PulseHarnessPlatformIssue `json:"platform,omitempty"`
}

type PulseHarnessPlatformIssue struct {
	IssueKey          string   `json:"issue_key"`
	AffectedWorkflows []string `json:"affected_workflows"`
	SeenCount         int      `json:"seen_count"`
	FirstSeenAt       string   `json:"first_seen_at,omitempty"`
	LastSeenAt        string   `json:"last_seen_at,omitempty"`
}

type pulseFindingDetailMarker struct {
	Concern string `json:"concern"`
	Module  string `json:"module,omitempty"`
	PulseFindingDetails
}

func normalizePulseFindingDetails(details PulseFindingDetails) PulseFindingDetails {
	details.FindingID = strings.TrimSpace(details.FindingID)
	details.TargetKey = strings.TrimSpace(details.TargetKey)
	details.IssueKind = strings.TrimSpace(details.IssueKind)
	details.RecommendedRoute = strings.TrimSpace(details.RecommendedRoute)
	details.NextCheck = strings.TrimSpace(details.NextCheck)
	details.Classification = strings.TrimSpace(details.Classification)
	details.Severity = strings.TrimSpace(details.Severity)
	details.Summary = strings.TrimSpace(details.Summary)
	details.Impact = strings.TrimSpace(details.Impact)
	details.Workaround = strings.TrimSpace(details.Workaround)
	details.Evidence = normalizedLifecycleStrings(details.Evidence)
	details.Reproduction.Setup = strings.TrimSpace(details.Reproduction.Setup)
	details.Reproduction.Action = strings.TrimSpace(details.Reproduction.Action)
	details.Reproduction.Expected = strings.TrimSpace(details.Reproduction.Expected)
	details.Reproduction.Observed = strings.TrimSpace(details.Reproduction.Observed)
	details.Reproduction.Limitations = strings.TrimSpace(details.Reproduction.Limitations)
	return details
}

const (
	pulseFindingRouteDecisionRequired = "decision_required"
	pulseFindingRouteEvidenceWait     = "evidence_wait"
	pulseFindingRouteFixerHandoff     = "fixer_handoff"
	pulseFindingRouteNone             = "none"
)

func isPulseAdvisorModule(module string) bool {
	module = pulsemodules.Normalize(module)
	return module == pulsemodules.StrategyAuditorID || module == pulsemodules.GoalAdvisorID
}

// validatePulseAdvisorFindingRoutes makes the advisor-to-lifecycle handoff a
// stored contract rather than prose the next agent has to infer. An advisor
// concern is not an engineering repair by default: it must identify whether it
// needs a decision, future evidence, or an explicit Fixer handoff.
func validatePulseAdvisorFindingRoutes(module, summary string) error {
	module = pulsemodules.Normalize(module)
	if !isPulseAdvisorModule(module) {
		return nil
	}
	concerns := ParseConcernLines(summary)
	if len(concerns) == 0 {
		return nil
	}
	markers := map[string]pulseFindingDetailMarker{}
	for _, marker := range ParsePulseFindingDetailMarkers(summary) {
		key := strings.ToLower(strings.Join(strings.Fields(marker.Concern), " "))
		if _, duplicate := markers[key]; duplicate {
			return fmt.Errorf("%s concern %q has duplicate PULSE_FINDING_JSON routing markers", module, marker.Concern)
		}
		marker.Module = pulsemodules.Normalize(marker.Module)
		if marker.Module != module {
			return fmt.Errorf("%s concern %q must use module=%q in PULSE_FINDING_JSON; got %q", module, marker.Concern, module, marker.Module)
		}
		switch marker.RecommendedRoute {
		case pulseFindingRouteDecisionRequired, pulseFindingRouteFixerHandoff:
		case pulseFindingRouteEvidenceWait:
			if marker.NextCheck == "" {
				return fmt.Errorf("%s concern %q uses recommended_route=evidence_wait without an exact next_check", module, marker.Concern)
			}
		case pulseFindingRouteNone:
			return fmt.Errorf("%s concern %q uses recommended_route=none but is still emitted as CONCERNS; omit the CONCERNS line for a non-trackable conclusion", module, marker.Concern)
		default:
			return fmt.Errorf("%s concern %q must set recommended_route to decision_required, evidence_wait, fixer_handoff, or none", module, marker.Concern)
		}
		markers[key] = marker
	}
	for _, concern := range concerns {
		key := strings.ToLower(strings.Join(strings.Fields(concern), " "))
		if _, ok := markers[key]; !ok {
			return fmt.Errorf("%s concern %q is missing its PULSE_FINDING_JSON routing marker", module, concern)
		}
	}
	return nil
}

// ParsePulseFindingDetailMarkers extracts one-line JSON records emitted by a
// trusted Pulse reviewer. Invalid records are ignored here; the full Markdown
// remains available for forensic inspection and the CONCERNS line still files.
func ParsePulseFindingDetailMarkers(summary string) []pulseFindingDetailMarker {
	var markers []pulseFindingDetailMarker
	for _, line := range strings.Split(summary, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), pulseFindingJSONPrefix) {
			continue
		}
		raw := strings.TrimSpace(trimmed[len(pulseFindingJSONPrefix):])
		if raw == "" {
			continue
		}
		var marker pulseFindingDetailMarker
		if err := json.Unmarshal([]byte(raw), &marker); err != nil {
			continue
		}
		marker.Concern = strings.TrimSpace(marker.Concern)
		marker.Module = strings.TrimSpace(marker.Module)
		marker.PulseFindingDetails = normalizePulseFindingDetails(marker.PulseFindingDetails)
		if marker.Concern == "" || marker.IssueKind == "" {
			continue
		}
		markers = append(markers, marker)
	}
	return markers
}

func pulseFindingCanonicalFingerprint(stepID string, marker pulseFindingDetailMarker) string {
	identity := strings.TrimSpace(marker.FindingID)
	prefix := "finding_id:"
	if identity == "" {
		identity = strings.TrimSpace(marker.TargetKey)
		prefix = "target_key:"
	}
	if identity == "" {
		return concernFingerprint(stepID, marker.Concern)
	}
	// Structured IDs are workflow-global identities. Including the reporting
	// module here recreated the same issue once per reviewer.
	return concernFingerprint("__structured_finding__", prefix+strings.ToLower(identity))
}

func pulseFindingFingerprintsByConcern(summary, stepID string) map[string]string {
	out := map[string]string{}
	for _, marker := range ParsePulseFindingDetailMarkers(summary) {
		normalized := strings.ToLower(strings.Join(strings.Fields(marker.Concern), " "))
		out[normalized] = pulseFindingCanonicalFingerprint(stepID, marker)
	}
	return out
}

func recordPulseFindingDetailsAt(
	ctx context.Context,
	db pulseFindingLifecycleDB,
	workspacePath, runFolder, stepID, summary, observedAt string,
	concernLines []string,
	fingerprints map[string]string,
) error {
	markers := ParsePulseFindingDetailMarkers(summary)
	if len(markers) == 0 {
		return nil
	}
	knownConcerns := make(map[string]bool, len(concernLines))
	for _, concern := range concernLines {
		knownConcerns[strings.ToLower(strings.Join(strings.Fields(concern), " "))] = true
	}
	for _, marker := range markers {
		normalizedConcern := strings.ToLower(strings.Join(strings.Fields(marker.Concern), " "))
		// Structured details may only decorate a concern filed in the same
		// artifact. This prevents a malformed marker from creating a hidden issue.
		if !knownConcerns[normalizedConcern] {
			continue
		}
		fingerprint := fingerprints[normalizedConcern]
		if fingerprint == "" {
			fingerprint = pulseFindingCanonicalFingerprint(stepID, marker)
		}
		encoded, err := json.Marshal(marker.PulseFindingDetails)
		if err != nil {
			return fmt.Errorf("encode Pulse finding details: %w", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_details
			(fingerprint, finding_id, issue_kind, target_key, detail_json, source_run_id, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(fingerprint) DO UPDATE SET
				finding_id=excluded.finding_id,
				issue_kind=excluded.issue_kind,
				target_key=excluded.target_key,
				detail_json=excluded.detail_json,
				source_run_id=excluded.source_run_id,
				updated_at=excluded.updated_at`,
			fingerprint, marker.FindingID, marker.IssueKind, marker.TargetKey,
			string(encoded), runFolder, observedAt); err != nil {
			return err
		}
		if marker.IssueKind == "harness_issue" && marker.TargetKey != "" {
			if err := upsertPulseHarnessPlatformIssue(
				ctx, workspacePath, stepID, fingerprint, runFolder,
				marker.PulseFindingDetails, observedAt,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func pulsePlatformDBPath() string {
	return filepath.Join(fsutil.WorkspaceDocsRoot(), "_system", "pulse-platform.sqlite")
}

func openPulsePlatformDB(ctx context.Context, create bool) (*sql.DB, error) {
	dbPath := pulsePlatformDBPath()
	if create {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, err
	}
	for _, ddl := range []string{
		pulsePlatformHarnessIssuesSchema,
		pulsePlatformHarnessOccurrencesSchema,
		`CREATE INDEX IF NOT EXISTS idx_platform_harness_occurrences_issue ON platform_harness_occurrences(issue_key, last_seen_at DESC)`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func upsertPulseHarnessPlatformIssue(
	ctx context.Context,
	workspacePath, module, fingerprint, sourceRunID string,
	details PulseFindingDetails,
	observedAt string,
) error {
	db, err := openPulsePlatformDB(ctx, true)
	if err != nil || db == nil {
		return err
	}
	defer db.Close()
	encoded, err := json.Marshal(details)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_harness_issues
		(issue_key, finding_id, severity, summary, detail_json, first_seen_at, last_seen_at, seen_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(issue_key) DO UPDATE SET
			finding_id=CASE WHEN excluded.finding_id<>'' THEN excluded.finding_id ELSE platform_harness_issues.finding_id END,
			severity=CASE WHEN excluded.severity<>'' THEN excluded.severity ELSE platform_harness_issues.severity END,
			summary=CASE WHEN excluded.summary<>'' THEN excluded.summary ELSE platform_harness_issues.summary END,
			detail_json=excluded.detail_json,
			last_seen_at=excluded.last_seen_at,
			seen_count=platform_harness_issues.seen_count+1`,
		details.TargetKey, details.FindingID, details.Severity, details.Summary,
		string(encoded), observedAt, observedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_harness_occurrences
		(issue_key, workspace_path, fingerprint, module, source_run_id, first_seen_at, last_seen_at, seen_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(issue_key, workspace_path, fingerprint) DO UPDATE SET
			module=excluded.module,
			source_run_id=excluded.source_run_id,
			last_seen_at=excluded.last_seen_at,
			seen_count=platform_harness_occurrences.seen_count+1`,
		details.TargetKey, strings.TrimSpace(workspacePath), fingerprint, module,
		sourceRunID, observedAt, observedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func loadPulseHarnessPlatformIssue(ctx context.Context, issueKey string) (*PulseHarnessPlatformIssue, error) {
	issueKey = strings.TrimSpace(issueKey)
	if issueKey == "" {
		return nil, nil
	}
	db, err := openPulsePlatformDB(ctx, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	var issue PulseHarnessPlatformIssue
	issue.IssueKey = issueKey
	if err := db.QueryRowContext(ctx, `SELECT first_seen_at, last_seen_at, seen_count
		FROM platform_harness_issues WHERE issue_key=?`, issueKey).
		Scan(&issue.FirstSeenAt, &issue.LastSeenAt, &issue.SeenCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT workspace_path
		FROM platform_harness_occurrences WHERE issue_key=? ORDER BY workspace_path`, issueKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var workspacePath string
		if err := rows.Scan(&workspacePath); err != nil {
			return nil, err
		}
		issue.AffectedWorkflows = append(issue.AffectedWorkflows, workspacePath)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if issue.AffectedWorkflows == nil {
		issue.AffectedWorkflows = []string{}
	}
	return &issue, nil
}
