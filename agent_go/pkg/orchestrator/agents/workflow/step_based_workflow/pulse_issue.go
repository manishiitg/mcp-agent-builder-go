package step_based_workflow

import (
	"strings"
	"unicode"
)

// PulseIssue is the intentionally small, user-facing projection of a finding.
// The lifecycle tables remain the source of truth for evidence, attempts,
// verification, and audit history; those records are linked activity, not issue
// fields. Fingerprints are internal matching keys and are never used as the
// human-facing issue identity.
type PulseIssue struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Module      string `json:"module,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	SeenCount   int    `json:"seen_count"`
}

func pulseIssueID(finding PulseFindingLifecycle) string {
	fingerprint := strings.ToUpper(strings.TrimSpace(finding.Fingerprint))
	if len(fingerprint) > 8 {
		fingerprint = fingerprint[:8]
	}
	if fingerprint == "" {
		return "PUL-UNKNOWN"
	}
	return "PUL-" + fingerprint
}

func pulseIssueTitle(finding PulseFindingLifecycle) string {
	if finding.Details != nil {
		if summary := strings.TrimSpace(finding.Details.Summary); summary != "" {
			return summary
		}
	}
	text := strings.TrimSpace(finding.Text)
	if firstLine, _, ok := strings.Cut(text, "\n"); ok {
		text = strings.TrimSpace(firstLine)
	}
	const maxRunes = 140
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	cut := maxRunes - 1
	for cut > maxRunes-28 && cut > 0 && !unicode.IsSpace(runes[cut]) {
		cut--
	}
	if cut <= maxRunes-28 {
		cut = maxRunes - 1
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}

func pulseIssueStatus(finding PulseFindingLifecycle) string {
	switch strings.TrimSpace(finding.Status) {
	case ConcernStatusFixing:
		return "in_progress"
	case ConcernStatusAwaitingVerification:
		return "in_review"
	case ConcernStatusQueuedForEngineering:
		return "backlog"
	case ConcernStatusResolved:
		return "done"
	case ConcernStatusRejected:
		return "canceled"
	case ConcernStatusExternalActionRequired:
		return "external"
	case ConcernStatusAcknowledged:
		// Lifecycle events are loaded newest first. The latest triage event is
		// the current issue state.
		for index := 0; index < len(finding.Events); index++ {
			switch finding.Events[index].EventType {
			case FindingDispositionAwaitingUser:
				return "needs_input"
			case FindingDispositionBlocked:
				return "blocked"
			}
		}
		return "backlog"
	default:
		return "backlog"
	}
}

func pulseIssuePriority(finding PulseFindingLifecycle) string {
	if finding.Details == nil {
		return "none"
	}
	switch strings.ToLower(strings.TrimSpace(finding.Details.Severity)) {
	case "critical", "urgent":
		return "urgent"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	default:
		return "none"
	}
}

// NewPulseIssue projects the durable lifecycle into the compact issue shape
// used by Pulse's Linear-style workspace.
func NewPulseIssue(finding PulseFindingLifecycle) PulseIssue {
	title := pulseIssueTitle(finding)
	description := strings.TrimSpace(finding.Text)
	if description == title {
		description = ""
	}
	if finding.Details != nil && strings.TrimSpace(finding.Details.Impact) != "" {
		impact := strings.TrimSpace(finding.Details.Impact)
		if description == "" {
			description = "Impact: " + impact
		} else if !strings.Contains(strings.ToLower(description), strings.ToLower(impact)) {
			description += "\n\nImpact: " + impact
		}
	}
	return PulseIssue{
		ID:          pulseIssueID(finding),
		Title:       title,
		Description: description,
		Status:      pulseIssueStatus(finding),
		Priority:    pulseIssuePriority(finding),
		Module:      strings.TrimSpace(finding.Module),
		CreatedAt:   finding.FirstSeenAt,
		UpdatedAt:   finding.LastSeenAt,
		SeenCount:   finding.SeenCount,
	}
}
