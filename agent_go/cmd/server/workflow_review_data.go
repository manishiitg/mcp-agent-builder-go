package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/costledger"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
)

type workflowCostsResponse struct {
	Success         bool                              `json:"success"`
	ScopedCosts     *costledger.Summary               `json:"scoped_costs,omitempty"`
	PhaseTokenUsage *orchestrator.PhaseTokenUsageFile `json:"phase_token_usage,omitempty"`
	PhaseDailyCosts []workflowPhaseDailyCostEntry     `json:"phase_daily_costs"`
	RunDailyCosts   []workflowRunDailyCostEntry       `json:"run_daily_costs"`
	Runs            []workflowRunCostEntry            `json:"runs"`
	ActivityTiming  workflowActivityTimingSummary     `json:"activity_timing"`
}

// workflowActivityTimingAggregate deliberately calls this agent time rather
// than wall time. It is the sum of elapsed time for the agents in a category;
// agents may overlap when a workflow runs work in parallel.
type workflowActivityTimingAggregate struct {
	DurationMS     int64                                       `json:"duration_ms"`
	LLMDurationMS  int64                                       `json:"llm_duration_ms"`
	ToolDurationMS int64                                       `json:"tool_duration_ms"`
	ByExecution    map[string]*workflowActivityTimingAggregate `json:"by_execution,omitempty"`
}

type workflowActivityTimingDate struct {
	ByScope map[string]*workflowActivityTimingAggregate `json:"by_scope"`
}

type workflowActivityTimingSummary struct {
	ByScope map[string]*workflowActivityTimingAggregate `json:"by_scope"`
	ByDate  map[string]*workflowActivityTimingDate      `json:"by_date"`
}

type persistedWorkflowTiming struct {
	StepID string `json:"step_id"`
	Agent  struct {
		StartedAt  string `json:"started_at"`
		DurationMS int64  `json:"duration_ms"`
	} `json:"agent"`
	Breakdown struct {
		WallDurationMS int64 `json:"wall_duration_ms"`
		LLMDurationMS  int64 `json:"llm_duration_ms"`
		ToolDurationMS int64 `json:"tool_duration_ms"`
	} `json:"breakdown"`
}

type StepOutputContent struct {
	FilePath string      `json:"file_path"`
	Content  interface{} `json:"content"`
	IsJSON   bool        `json:"is_json"`
}

type EvaluationStepScore struct {
	StepID   string  `json:"step_id"`
	Score    float64 `json:"score"`
	MaxScore float64 `json:"max_score,omitempty"`
	// Pointer preserves the difference between a new report's explicit false
	// (source output had no score) and a legacy report where the field did not
	// exist. The frontend can safely apply compatibility inference only to nil.
	ScoreCaptured *bool              `json:"score_captured,omitempty"`
	Reasoning     string             `json:"reasoning"`
	Evidence      string             `json:"evidence"`
	Skipped       bool               `json:"skipped,omitempty"`
	ContextOutput string             `json:"context_output,omitempty"`
	OutputContent *StepOutputContent `json:"output_content,omitempty"`
}

type EvaluationReport struct {
	EvaluationID    string                `json:"evaluation_id,omitempty"`
	TargetRunFolder string                `json:"target_run_folder"`
	GeneratedAt     string                `json:"generated_at"`
	StepScores      []EvaluationStepScore `json:"step_scores"`
}

type EvaluationReportEntry struct {
	EvaluationID      string           `json:"evaluation_id,omitempty"`
	RunFolder         string           `json:"run_folder"`
	ArchivedRunFolder string           `json:"archived_run_folder,omitempty"`
	Report            EvaluationReport `json:"report"`
}

type EvaluationAggregate struct {
	TotalRuns int `json:"total_runs"`
}

type workflowEvaluationReportsResponse struct {
	Success        bool                    `json:"success"`
	Reports        []EvaluationReportEntry `json:"reports"`
	Aggregate      *EvaluationAggregate    `json:"aggregate,omitempty"`
	EvaluationPlan *string                 `json:"evaluation_plan,omitempty"`
	Error          string                  `json:"error,omitempty"`
}

type workflowReviewDataResponse struct {
	Success     bool                              `json:"success"`
	Costs       workflowCostsResponse             `json:"costs"`
	Evaluations workflowEvaluationReportsResponse `json:"evaluations"`
}

func loadWorkflowCosts(ctx context.Context, workspacePath string) workflowCostsResponse {
	var scopedCosts *costledger.Summary
	if ledger := costledger.DefaultLedger(); ledger != nil {
		if summary, err := ledger.SummarizeWorkflow(workspacePath); err == nil {
			scopedCosts = summary
		}
	}
	var phaseTokenUsage *orchestrator.PhaseTokenUsageFile
	if phaseUsage, err := readPhaseTokenUsageFromCosts(ctx, workspacePath); err == nil {
		phaseTokenUsage = phaseUsage
	}

	phaseDailyCosts, err := readAllPhaseTokenUsageFromCosts(ctx, workspacePath)
	if err != nil {
		phaseDailyCosts = []workflowPhaseDailyCostEntry{}
	}

	executionCosts, err := readAllRunTokenUsageFromCosts(ctx, workspacePath, orchestrator.CostScopeExecution)
	if err != nil {
		executionCosts = map[string]*storedRunTokenUsage{}
	}

	evaluationCosts, err := readAllRunTokenUsageFromCosts(ctx, workspacePath, orchestrator.CostScopeEvaluation)
	if err != nil {
		evaluationCosts = map[string]*storedRunTokenUsage{}
	}

	runDailyCosts := readWorkflowRunDailyCosts(ctx, workspacePath)

	return workflowCostsResponse{
		Success:         true,
		ScopedCosts:     scopedCosts,
		PhaseTokenUsage: phaseTokenUsage,
		PhaseDailyCosts: phaseDailyCosts,
		RunDailyCosts:   runDailyCosts,
		Runs:            buildWorkflowRunCostEntries(executionCosts, evaluationCosts),
		ActivityTiming:  loadWorkflowActivityTiming(ctx, workspacePath),
	}
}

func newWorkflowActivityTimingSummary() workflowActivityTimingSummary {
	return workflowActivityTimingSummary{
		ByScope: map[string]*workflowActivityTimingAggregate{},
		ByDate:  map[string]*workflowActivityTimingDate{},
	}
}

func addWorkflowActivityTiming(summary *workflowActivityTimingSummary, date, scope, executionID string, durationMS, llmDurationMS, toolDurationMS int64) {
	if summary == nil || strings.TrimSpace(scope) == "" || durationMS <= 0 {
		return
	}
	add := func(scopes map[string]*workflowActivityTimingAggregate) {
		aggregate := scopes[scope]
		if aggregate == nil {
			aggregate = &workflowActivityTimingAggregate{ByExecution: map[string]*workflowActivityTimingAggregate{}}
			scopes[scope] = aggregate
		}
		aggregate.DurationMS += durationMS
		aggregate.LLMDurationMS += llmDurationMS
		aggregate.ToolDurationMS += toolDurationMS
		if strings.TrimSpace(executionID) != "" {
			execution := aggregate.ByExecution[executionID]
			if execution == nil {
				execution = &workflowActivityTimingAggregate{}
				aggregate.ByExecution[executionID] = execution
			}
			execution.DurationMS += durationMS
			execution.LLMDurationMS += llmDurationMS
			execution.ToolDurationMS += toolDurationMS
		}
	}
	add(summary.ByScope)
	if date != "" {
		day := summary.ByDate[date]
		if day == nil {
			day = &workflowActivityTimingDate{ByScope: map[string]*workflowActivityTimingAggregate{}}
			summary.ByDate[date] = day
		}
		add(day.ByScope)
	}
}

func timingDate(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
		return parsed.UTC().Format("2006-01-02")
	}
	if len(value) >= 10 {
		return value[:10]
	}
	return ""
}

func loadWorkflowActivityTiming(ctx context.Context, workspacePath string) workflowActivityTimingSummary {
	summary := newWorkflowActivityTimingSummary()
	if metrics, err := step_based_workflow.LoadPulseAgentMetrics(ctx, workspacePath, "", "", "", -1); err == nil {
		for _, metric := range metrics {
			date := timingDate(metric.StartedAt)
			if date == "" {
				date = timingDate(metric.CompletedAt)
			}
			addWorkflowActivityTiming(&summary, date, "pulse", metric.ExecutionID, metric.DurationMS, 0, 0)
		}
	}

	workspaceRoot := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath))
	loadWorkflowTimingFiles(&summary, filepath.Join(workspaceRoot, "runs"), "workflow_execution", "step:")
	loadWorkflowTimingFiles(&summary, filepath.Join(workspaceRoot, "evaluation", "runs"), "evaluation", "evaluation:")
	return summary
}

func loadWorkflowTimingFiles(summary *workflowActivityTimingSummary, root, scope, executionPrefix string) {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry == nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), "-timing.json") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var timing persistedWorkflowTiming
		if err := json.Unmarshal(content, &timing); err != nil || strings.TrimSpace(timing.StepID) == "" {
			return nil
		}
		durationMS := timing.Breakdown.WallDurationMS
		if durationMS <= 0 {
			durationMS = timing.Agent.DurationMS
		}
		date := timingDate(timing.Agent.StartedAt)
		if date == "" {
			if info, err := entry.Info(); err == nil {
				date = info.ModTime().UTC().Format("2006-01-02")
			}
		}
		addWorkflowActivityTiming(summary, date, scope, executionPrefix+timing.StepID, durationMS, timing.Breakdown.LLMDurationMS, timing.Breakdown.ToolDurationMS)
		return nil
	})
}

func readWorkflowRunDailyCosts(ctx context.Context, workspacePath string) []workflowRunDailyCostEntry {
	entries := make([]workflowRunDailyCostEntry, 0)
	if executionDailyCosts, err := readAllRunDailyTokenUsageFromCosts(ctx, workspacePath, orchestrator.CostScopeExecution); err == nil {
		entries = append(entries, executionDailyCosts...)
	}
	if evaluationDailyCosts, err := readAllRunDailyTokenUsageFromCosts(ctx, workspacePath, orchestrator.CostScopeEvaluation); err == nil {
		entries = append(entries, evaluationDailyCosts...)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date > entries[j].Date
		}
		if entries[i].RunFolder != entries[j].RunFolder {
			return entries[i].RunFolder < entries[j].RunFolder
		}
		return entries[i].Scope < entries[j].Scope
	})
	return entries
}

func loadWorkflowEvaluationReports(ctx context.Context, workspacePath, runFolder string) workflowEvaluationReportsResponse {
	evaluationPlan := readWorkflowEvaluationPlan(ctx, workspacePath)
	reportMap, err := readAllEvaluationReportsFromScores(ctx, workspacePath)
	if err != nil {
		return workflowEvaluationReportsResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read evaluation scores: %v", err),
		}
	}

	var reports []EvaluationReportEntry
	for evaluationID, stored := range reportMap {
		if stored == nil || stored.Report == nil || !workflowRunFolderMatches(stored.effectiveRunFolder(), runFolder) {
			continue
		}
		reports = append(reports, EvaluationReportEntry{
			EvaluationID:      evaluationID,
			RunFolder:         stored.RunFolder,
			ArchivedRunFolder: stored.ArchivedRunFolder,
			Report:            *stored.Report,
		})
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Report.GeneratedAt > reports[j].Report.GeneratedAt
	})

	return workflowEvaluationReportsResponse{
		Success:        true,
		Reports:        reports,
		Aggregate:      buildEvaluationAggregate(reports),
		EvaluationPlan: evaluationPlan,
	}
}

func workflowRunFolderMatches(candidate, requested string) bool {
	if strings.TrimSpace(requested) == "" {
		return true
	}
	return candidate == requested ||
		strings.HasPrefix(candidate, requested+"/") ||
		strings.HasPrefix(requested, candidate+"/")
}

func buildEvaluationAggregate(reports []EvaluationReportEntry) *EvaluationAggregate {
	if len(reports) == 0 {
		return nil
	}

	return &EvaluationAggregate{
		TotalRuns: len(reports),
	}
}

func readWorkflowEvaluationPlan(ctx context.Context, workspacePath string) *string {
	evaluationPlanPath := filepath.Join(workspacePath, "evaluation", "evaluation_plan.json")
	planContent, exists, err := readFileFromWorkspace(ctx, evaluationPlanPath)
	if err != nil || !exists {
		return nil
	}

	var planJSON interface{}
	if err := json.Unmarshal([]byte(planContent), &planJSON); err == nil {
		if formatted, err := json.MarshalIndent(planJSON, "", "  "); err == nil {
			formattedStr := string(formatted)
			return &formattedStr
		}
	}

	return &planContent
}

func (api *StreamingAPI) handleGetWorkflowReviewData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspacePath := r.URL.Query().Get("workspace_path")
	runFolder := r.URL.Query().Get("run_folder")
	if workspacePath == "" {
		http.Error(w, "workspace_path parameter is required", http.StatusBadRequest)
		return
	}

	cleanedWorkspacePath := filepath.Clean(workspacePath)
	if strings.Contains(cleanedWorkspacePath, "..") {
		http.Error(w, "Invalid workspace path", http.StatusBadRequest)
		return
	}

	response := workflowReviewDataResponse{
		Success:     true,
		Costs:       loadWorkflowCosts(r.Context(), cleanedWorkspacePath),
		Evaluations: loadWorkflowEvaluationReports(r.Context(), cleanedWorkspacePath, runFolder),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
