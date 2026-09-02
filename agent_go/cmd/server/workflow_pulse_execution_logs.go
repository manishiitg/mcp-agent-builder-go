package server

import (
	"context"
	"sort"
	"strings"
	"time"

	orchEvents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

const maxPulseReviewRunsInExecutionLogs = 20

// PulseReviewRunLog is one Pulse pass associated with the selected retained
// run. A retained folder can be reviewed repeatedly, so session/run identity
// is preserved instead of flattening every reviewer into one timeless list.
type PulseReviewRunLog struct {
	RunID         string                          `json:"run_id"`
	SessionID     string                          `json:"session_id"`
	ScheduleID    string                          `json:"schedule_id,omitempty"`
	TriggerSource string                          `json:"trigger_source,omitempty"`
	Status        string                          `json:"status,omitempty"`
	StartedAt     time.Time                       `json:"started_at"`
	CompletedAt   *time.Time                      `json:"completed_at,omitempty"`
	Agents        []PulseBackgroundAgentExecution `json:"agents"`
}

type PulseBackgroundAgentExecution struct {
	AgentID           string                                      `json:"agent_id"`
	Name              string                                      `json:"name"`
	Kind              string                                      `json:"kind,omitempty"`
	ParentExecutionID string                                      `json:"parent_execution_id,omitempty"`
	Status            string                                      `json:"status"`
	Result            string                                      `json:"result,omitempty"`
	Error             string                                      `json:"error,omitempty"`
	Duration          string                                      `json:"duration,omitempty"`
	StartedAt         string                                      `json:"started_at,omitempty"`
	CompletedAt       string                                      `json:"completed_at,omitempty"`
	TranscriptPath    string                                      `json:"transcript_path,omitempty"`
	TranscriptStatus  string                                      `json:"transcript_status,omitempty"`
	Provider          string                                      `json:"provider,omitempty"`
	ModelID           string                                      `json:"model_id,omitempty"`
	Events            []orchEvents.BackgroundAgentTranscriptEvent `json:"events,omitempty"`
}

func loadPulseReviewRunsForExecutionLogs(ctx context.Context, workspacePath, runFolder string) ([]PulseReviewRunLog, error) {
	runs, err := ReadScheduleRuns(ctx, workspacePath)
	if err != nil {
		return nil, err
	}
	out := make([]PulseReviewRunLog, 0)
	for _, run := range runs {
		if strings.TrimSpace(run.SessionID) == "" || !scheduleRunMatchesExecutionFolder(run, runFolder) {
			continue
		}
		entries, err := backgroundAgentLogForSession(ctx, workspacePath, run.SessionID)
		if err != nil {
			return nil, err
		}
		pulseEntries := selectPulseBackgroundReviewEntries(run, entries)
		if len(pulseEntries) == 0 {
			continue
		}
		item := PulseReviewRunLog{
			RunID: run.ID, SessionID: run.SessionID, ScheduleID: run.ScheduleID,
			TriggerSource: run.TriggerSource, Status: run.Status,
			StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
			Agents: make([]PulseBackgroundAgentExecution, 0, len(pulseEntries)),
		}
		for _, entry := range pulseEntries {
			agent := PulseBackgroundAgentExecution{
				AgentID: entry.AgentID, Name: entry.Name, Kind: entry.Kind,
				ParentExecutionID: entry.ParentExecutionID, Status: entry.Status,
				Result: entry.Result, Error: entry.Error, Duration: entry.Duration,
				StartedAt: entry.StartedAt, CompletedAt: entry.CompletedAt,
				TranscriptPath: entry.TranscriptPath, TranscriptStatus: entry.TranscriptStatus,
			}
			if entry.TranscriptPath != "" && entry.TranscriptStatus == "ok" {
				content, exists, readErr := readFileFromWorkspace(ctx, entry.TranscriptPath)
				switch {
				case readErr != nil:
					agent.TranscriptStatus = "error: " + readErr.Error()
				case !exists:
					agent.TranscriptStatus = "missing"
				default:
					transcript, parseErr := orchEvents.ParseBackgroundAgentTranscript(content)
					if parseErr != nil {
						agent.TranscriptStatus = "error: " + parseErr.Error()
					} else if transcript == nil {
						agent.TranscriptStatus = "missing"
					} else {
						agent.Provider = transcript.Provider
						agent.ModelID = transcript.ModelID
						agent.Events = transcript.Events
					}
				}
			}
			item.Agents = append(item.Agents, agent)
		}
		out = append(out, item)
		if len(out) >= maxPulseReviewRunsInExecutionLogs {
			break
		}
	}
	return out, nil
}

func scheduleRunMatchesExecutionFolder(run ScheduleRunEntry, selected string) bool {
	selected = normalizeRunFolderForPulseLogs(selected)
	runFolder := normalizeRunFolderForPulseLogs(run.RunFolder)
	if selected == "" || runFolder == "" {
		return false
	}
	if selected == runFolder {
		return true
	}
	selectedParts := strings.Split(selected, "/")
	runParts := strings.Split(runFolder, "/")
	if selectedParts[0] != runParts[0] {
		return false
	}
	// When both sides identify a group, require the same group. When the
	// schedule retained only the iteration name, use its explicit group list.
	if len(selectedParts) > 1 && len(runParts) > 1 {
		return selectedParts[1] == runParts[1]
	}
	if len(selectedParts) > 1 && len(run.GroupNames) > 0 {
		for _, group := range run.GroupNames {
			if strings.TrimSpace(group) == selectedParts[1] {
				return true
			}
		}
		return false
	}
	return true
}

func normalizeRunFolderForPulseLogs(value string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
}

func selectPulseBackgroundReviewEntries(run ScheduleRunEntry, entries []BackgroundAgentLogEntry) []BackgroundAgentLogEntry {
	manualPulse := run.ScheduleID == manualWorkflowPulseScheduleID
	var workflowCompletedAt time.Time
	for _, entry := range entries {
		if entry.Kind != string(orchEvents.ExecutionKindFullRun) {
			continue
		}
		if completed, err := time.Parse(time.RFC3339, entry.CompletedAt); err == nil && completed.After(workflowCompletedAt) {
			workflowCompletedAt = completed
		}
	}

	selected := make([]BackgroundAgentLogEntry, 0)
	for _, entry := range entries {
		if entry.Kind != string(orchEvents.ExecutionKindSubAgent) {
			continue
		}
		started, _ := time.Parse(time.RFC3339, entry.StartedAt)
		afterWorkflow := !workflowCompletedAt.IsZero() && !started.Before(workflowCompletedAt)
		if manualPulse || afterWorkflow || (workflowCompletedAt.IsZero() && looksLikeLegacyPulseBackgroundReview(entry)) {
			selected = append(selected, entry)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].StartedAt < selected[j].StartedAt })
	return selected
}

// Older/manual records do not always have a full-run lifecycle marker. This
// fallback is deliberately narrow and only applies when the deterministic
// post-workflow boundary is unavailable.
func looksLikeLegacyPulseBackgroundReview(entry BackgroundAgentLogEntry) bool {
	value := strings.ToLower(strings.Join([]string{entry.AgentID, entry.Name}, " "))
	for _, marker := range []string{"pulse", "plan drift", "plan-drift", "technical review", "technical-review", "technical maintenance", "technical-maintenance", "strategic review", "strategy auditor", "goal advisor"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
