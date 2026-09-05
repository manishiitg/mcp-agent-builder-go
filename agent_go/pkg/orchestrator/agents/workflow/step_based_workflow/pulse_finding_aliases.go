package step_based_workflow

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Automatic identity consolidation may remove duplicate rows, but public IDs
// already returned to callers must remain resolvable within this workflow DB.
const pulseFindingIssueAliasesSchema = `CREATE TABLE IF NOT EXISTS pulse_finding_issue_aliases (
	issue_id TEXT PRIMARY KEY COLLATE NOCASE,
	fingerprint TEXT NOT NULL
)`

// Match the same identity boundary as migrateDuplicatePulseFindingIdentities.
// Workflow target keys alone are not identities: different reviewers can use
// the same location for unrelated findings.
func existingPulseFindingIdentity(ctx context.Context, db pulseFindingLifecycleDB, marker pulseFindingDetailMarker) (string, error) {
	var where, value string
	if marker.FindingID != "" {
		where, value = "lower(trim(d.finding_id))=lower(?)", marker.FindingID
	} else if marker.IssueKind == IssueKindHarness && marker.TargetKey != "" {
		where, value = "d.finding_id='' AND d.issue_kind='harness_issue' AND lower(trim(d.target_key))=lower(?)", marker.TargetKey
	} else {
		return "", nil
	}
	var fingerprint string
	err := db.QueryRowContext(ctx, `SELECT d.fingerprint FROM pulse_finding_details d
		JOIN run_concerns c USING(fingerprint) WHERE `+where+` ORDER BY d.fingerprint LIMIT 1`, value).Scan(&fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return fingerprint, err
}

// Record aliases before deleting an old row. Repoint existing aliases as well,
// so multiple consolidations don't leave chains pointing at removed rows.
func preserveMergedPulseIssueID(ctx context.Context, db pulseFindingLifecycleDB, old, target string) error {
	var issueID string
	err := db.QueryRowContext(ctx, `SELECT issue_id FROM run_concerns WHERE fingerprint=?`, old).Scan(&issueID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(issueID) != "" {
		if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_issue_aliases(issue_id,fingerprint)
			VALUES (?,?) ON CONFLICT(issue_id) DO UPDATE SET fingerprint=excluded.fingerprint`, strings.TrimSpace(issueID), target); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx, `UPDATE pulse_finding_issue_aliases SET fingerprint=? WHERE fingerprint=?`, target, old)
	return err
}
