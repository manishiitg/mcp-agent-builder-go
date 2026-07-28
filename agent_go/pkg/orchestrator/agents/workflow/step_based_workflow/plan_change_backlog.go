package step_based_workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
)

// The backlog of plan changes whose knock-on effects nobody has checked yet.
//
// A workflow is a web of implicit dependencies: a step's output feeds downstream
// steps, evals score it, the report dashboard queries its data, db stores it, and
// learnings/KB describe its behavior. Change a step and those silently rot.
//
// The framework already accounts for this. plan-change-impact names the six
// dimensions to reconcile, every plan-mod call is logged with its reason and
// old/new values, artifact_review is the module that sweeps them, and
// mark_changelog_artifact_reviewed stamps an entry once its blast radius has been
// traced.
//
// The one missing piece was arithmetic. Nothing counted the unstamped entries, so
// Pulse Gate had to open the changelog files and work it out on every run — the
// same shape of blind spot as learning_health, whose trigger has always been
// "plan or prompt changes affected step behavior" but which had no fact to fire
// on.
//
// So this counts them, and makes no judgement about which dependents actually
// broke: only tracing a change's surface against the real artifacts can tell a
// cosmetic edit from one that orphans a report query. An earlier version of this
// file tried to decide staleness in Go and was wrong for exactly that reason —
// in a live workflow, 78 of the plan-mod calls were update_step_config, which
// records no field diff at all, so a review_notes touch was indistinguishable
// from a rewritten step description.
//
// Computed on read. It needs no state, and an entry stays listed until it is
// actually stamped, so the backlog cannot be lost by anything failing to record.

// UnreviewedPlanChange is one plan-mod call whose blast radius has not been traced.
type UnreviewedPlanChange struct {
	At            string   `json:"at"`
	Tool          string   `json:"tool"`
	Reason        string   `json:"reason,omitempty"`
	StepIDs       []string `json:"step_ids,omitempty"`
	FieldsChanged []string `json:"fields_changed,omitempty"`
	SourceFile    string   `json:"source_file"`
}

// PlanChangeBacklog is the summary handed to Pulse Gate.
type PlanChangeBacklog struct {
	UnreviewedCount int                    `json:"unreviewed_count"`
	OldestAt        string                 `json:"oldest_at,omitempty"`
	NewestAt        string                 `json:"newest_at,omitempty"`
	Changes         []UnreviewedPlanChange `json:"changes,omitempty"`
	Note            string                 `json:"note,omitempty"`
}

// maxListedPlanChanges caps the listed entries. The count is always exact; the
// list is a prompt for a decision, and planning/changelog/ holds the full record.
const maxListedPlanChanges = 12

func workflowAbsPath(workspacePath string, parts ...string) string {
	all := append([]string{fsutil.WorkspaceDocsRoot(), filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/"))}, parts...)
	return filepath.Join(all...)
}

// CollectPlanChangeBacklog scans planning/changelog/ for entries that have not
// been stamped artifact_review.done, newest first.
//
// Returns nil when there is no changelog or nothing is outstanding — a workflow
// whose changes have all been reconciled must add nothing to Pulse's context.
func CollectPlanChangeBacklog(workspacePath string) *PlanChangeBacklog {
	workspacePath = strings.Trim(strings.TrimSpace(workspacePath), "/")
	if workspacePath == "" {
		return nil
	}
	dir := workflowAbsPath(workspacePath, PlanningFolderName, "changelog")
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	type dated struct {
		at     time.Time
		change UnreviewedPlanChange
	}
	var pending []dated
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var file PlanChangelog
		if json.Unmarshal(raw, &file) != nil {
			// A malformed changelog file is not evidence that its changes were
			// reviewed, but guessing its contents would be worse. Skip it; the
			// other files still produce a real backlog.
			continue
		}
		for _, e := range file.Entries {
			if e.ArtifactReview != nil && e.ArtifactReview.Done {
				continue
			}
			ts, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(e.Timestamp))
			if parseErr != nil {
				// Undateable but still unreviewed: keep it, sorted last, rather than
				// dropping an outstanding change because its timestamp is malformed.
				ts = time.Time{}
			}
			pending = append(pending, dated{at: ts.UTC(), change: toUnreviewedPlanChange(e, f.Name())})
		}
	}
	if len(pending) == 0 {
		return nil
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].at.After(pending[j].at) })

	backlog := &PlanChangeBacklog{
		UnreviewedCount: len(pending),
		NewestAt:        pending[0].change.At,
		OldestAt:        pending[len(pending)-1].change.At,
	}
	for i, p := range pending {
		if i >= maxListedPlanChanges {
			break
		}
		backlog.Changes = append(backlog.Changes, p.change)
	}
	backlog.Note = fmt.Sprintf(
		"%d plan-mod change(s) have not been stamped artifact_review.done, so their knock-on effects across downstream steps, evals, the report dashboard, db contracts, KB notes and learnings may not have been reconciled. Evidence, not a verdict: many changes have no blast radius at all. Consider marking artifact_review due so its reviewer can trace each surface against the real artifacts — get_reference_doc(kind=\"plan-change-impact\") is the procedure. Entries stay listed until mark_changelog_artifact_reviewed stamps them, so deferring loses nothing.",
		len(pending))
	if len(pending) > len(backlog.Changes) {
		backlog.Note += fmt.Sprintf(" Showing the %d most recent; the rest are in planning/changelog/.", len(backlog.Changes))
	}
	return backlog
}

func toUnreviewedPlanChange(e PlanChangelogEntry, sourceFile string) UnreviewedPlanChange {
	out := UnreviewedPlanChange{
		At:         strings.TrimSpace(e.Timestamp),
		Tool:       strings.TrimSpace(e.Tool),
		Reason:     strings.TrimSpace(e.Reason),
		StepIDs:    e.StepIDs,
		SourceFile: sourceFile,
	}
	// Field names are what make an entry triageable: they are the surface the
	// reviewer traces. update_step_config records none today, which is exactly
	// why the reviewer must open the artifacts rather than filter on a name.
	for _, c := range e.Changes {
		if f := strings.TrimSpace(c.Field); f != "" {
			out.FieldsChanged = append(out.FieldsChanged, f)
		}
	}
	return out
}
