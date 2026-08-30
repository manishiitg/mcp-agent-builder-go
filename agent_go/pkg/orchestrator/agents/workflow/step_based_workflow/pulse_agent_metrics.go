package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/costledger"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
)

const pulseAgentMetricsSchema = `CREATE TABLE IF NOT EXISTS pulse_agent_metrics (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	execution_id TEXT NOT NULL UNIQUE,
	agent_session_id TEXT NOT NULL DEFAULT '',
	pulse_run_id TEXT NOT NULL DEFAULT '',
	review_run_id TEXT NOT NULL DEFAULT '',
	module TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	queued_at TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL DEFAULT '',
	completed_at TEXT NOT NULL DEFAULT '',
	queue_duration_ms INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	llm_call_count INTEGER NOT NULL DEFAULT 0,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	reasoning_tokens INTEGER NOT NULL DEFAULT 0,
	cache_read_tokens INTEGER NOT NULL DEFAULT 0,
	cache_write_tokens INTEGER NOT NULL DEFAULT 0,
	total_cost_usd REAL NOT NULL DEFAULT 0,
	models_json TEXT NOT NULL DEFAULT '{}',
	usage_status TEXT NOT NULL DEFAULT 'unavailable',
	usage_error TEXT NOT NULL DEFAULT '',
	recorded_at TEXT NOT NULL DEFAULT ''
)`

const pulseAgentMetricsIndexes = `
CREATE INDEX IF NOT EXISTS idx_pulse_agent_metrics_run ON pulse_agent_metrics(pulse_run_id, role, module);
CREATE INDEX IF NOT EXISTS idx_pulse_agent_metrics_review ON pulse_agent_metrics(review_run_id, module, role);`

// PulseAgentMetricRecord is one durable measurement for Pulse work.
type PulseAgentMetricRecord struct {
	ID               int64                            `json:"id"`
	ExecutionID      string                           `json:"execution_id"`
	AgentSessionID   string                           `json:"agent_session_id,omitempty"`
	PulseRunID       string                           `json:"pulse_run_id,omitempty"`
	ReviewRunID      string                           `json:"review_run_id,omitempty"`
	Module           string                           `json:"module"`
	Role             string                           `json:"role"`
	Status           string                           `json:"status"`
	QueuedAt         string                           `json:"queued_at,omitempty"`
	StartedAt        string                           `json:"started_at,omitempty"`
	CompletedAt      string                           `json:"completed_at,omitempty"`
	QueueDurationMS  int64                            `json:"queue_duration_ms"`
	DurationMS       int64                            `json:"duration_ms"`
	LLMCallCount     int                              `json:"llm_call_count"`
	PromptTokens     int                              `json:"prompt_tokens"`
	CompletionTokens int                              `json:"completion_tokens"`
	ReasoningTokens  int                              `json:"reasoning_tokens"`
	CacheReadTokens  int                              `json:"cache_read_tokens"`
	CacheWriteTokens int                              `json:"cache_write_tokens"`
	TotalCostUSD     float64                          `json:"total_cost_usd"`
	Models           map[string]*costledger.Aggregate `json:"models,omitempty"`
	UsageStatus      string                           `json:"usage_status"`
	UsageError       string                           `json:"usage_error,omitempty"`
	RecordedAt       string                           `json:"recorded_at"`
}

func ensurePulseAgentMetricsSchema(ctx context.Context, db pulseFindingLifecycleDB) error {
	if _, err := db.ExecContext(ctx, pulseAgentMetricsSchema); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE pulse_agent_metrics SET module=? WHERE module IN (?, ?)`,
		pulsemodules.TechnicalReviewID, pulsemodules.LegacyWorkflowReviewID, pulsemodules.LegacyLLMOpsReviewID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, pulseAgentMetricsIndexes)
	return err
}

// RecordPulseAgentMetric snapshots central cost-ledger rows by the exact child
// execution id into the workflow-local DB. Missing usage is explicit rather
// than silently rendered as zero tokens or zero cost.
func RecordPulseAgentMetric(ctx context.Context, workspacePath string, metric PulseAgentMetricRecord) error {
	metric.ExecutionID = strings.TrimSpace(metric.ExecutionID)
	if metric.ExecutionID == "" {
		return fmt.Errorf("Pulse agent metric requires execution_id")
	}
	metric.Module = strings.TrimSpace(metric.Module)
	metric.Module = pulsemodules.Normalize(metric.Module)
	metric.Role = strings.ToLower(strings.TrimSpace(metric.Role))
	if metric.Role != "reviewer" && metric.Role != "fixer" {
		return fmt.Errorf("Pulse agent metric role must be reviewer or fixer")
	}
	// Usage is backend-derived only. Ignore any caller-populated counters so an
	// agent can never self-report cheaper or faster work than the ledger saw.
	metric.LLMCallCount = 0
	metric.PromptTokens = 0
	metric.CompletionTokens = 0
	metric.ReasoningTokens = 0
	metric.CacheReadTokens = 0
	metric.CacheWriteTokens = 0
	metric.TotalCostUSD = 0
	metric.Models = nil
	metric.UsageStatus = "unavailable"
	metric.UsageError = ""
	if ledger := costledger.DefaultLedger(); ledger == nil {
		metric.UsageError = "cost ledger is not initialized"
	} else if summary, err := ledger.SummarizeExecution(metric.ExecutionID); err != nil {
		metric.UsageError = err.Error()
	} else if summary.Total.AccountingEventCount == 0 {
		metric.UsageError = "no cost events were attributed to this execution"
	} else {
		metric.LLMCallCount = summary.Total.CallCount
		metric.PromptTokens = summary.Total.PromptTokens
		metric.CompletionTokens = summary.Total.CompletionTokens
		metric.ReasoningTokens = summary.Total.ReasoningTokens
		metric.CacheReadTokens = summary.Total.CacheReadTokens
		metric.CacheWriteTokens = summary.Total.CacheWriteTokens
		metric.Models = summary.ByModel
		metric.TotalCostUSD, metric.UsageStatus, metric.UsageError = pricePulseAgentModels(metric.Models)
	}

	modelsJSON, err := json.Marshal(metric.Models)
	if err != nil {
		return fmt.Errorf("marshal Pulse agent models: %w", err)
	}
	if metric.RecordedAt == "" {
		metric.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := ensurePulseAgentMetricsSchema(ctx, db); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO pulse_agent_metrics (
		execution_id, agent_session_id, pulse_run_id, review_run_id, module, role,
		status, queued_at, started_at, completed_at, queue_duration_ms, duration_ms,
		llm_call_count, prompt_tokens, completion_tokens, reasoning_tokens,
		cache_read_tokens, cache_write_tokens, total_cost_usd, models_json,
		usage_status, usage_error, recorded_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(execution_id) DO UPDATE SET
		agent_session_id=excluded.agent_session_id, pulse_run_id=excluded.pulse_run_id,
		review_run_id=excluded.review_run_id, module=excluded.module, role=excluded.role,
		status=excluded.status, queued_at=excluded.queued_at, started_at=excluded.started_at,
		completed_at=excluded.completed_at, queue_duration_ms=excluded.queue_duration_ms,
		duration_ms=excluded.duration_ms, llm_call_count=excluded.llm_call_count,
		prompt_tokens=excluded.prompt_tokens, completion_tokens=excluded.completion_tokens,
		reasoning_tokens=excluded.reasoning_tokens, cache_read_tokens=excluded.cache_read_tokens,
		cache_write_tokens=excluded.cache_write_tokens, total_cost_usd=excluded.total_cost_usd,
		models_json=excluded.models_json, usage_status=excluded.usage_status,
		usage_error=excluded.usage_error, recorded_at=excluded.recorded_at`,
		metric.ExecutionID, strings.TrimSpace(metric.AgentSessionID), strings.TrimSpace(metric.PulseRunID),
		strings.TrimSpace(metric.ReviewRunID), metric.Module, metric.Role, strings.TrimSpace(metric.Status),
		metric.QueuedAt, metric.StartedAt, metric.CompletedAt, metric.QueueDurationMS, metric.DurationMS,
		metric.LLMCallCount, metric.PromptTokens, metric.CompletionTokens, metric.ReasoningTokens,
		metric.CacheReadTokens, metric.CacheWriteTokens, metric.TotalCostUSD, string(modelsJSON),
		metric.UsageStatus, metric.UsageError, metric.RecordedAt,
	)
	return err
}

// pricePulseAgentModels applies the same immutable model pricing contract used
// by execution and phase ledgers when the central observer captured tokens but
// the CLI event did not carry a provider-side or adapter-side cost estimate.
// Only the explicitly unpriced token slice is priced, so a mixed aggregate can
// never double-charge calls that already carried cost.
func pricePulseAgentModels(models map[string]*costledger.Aggregate) (float64, string, string) {
	total := 0.0
	remainingUnpriced := 0
	for modelID, aggregate := range models {
		if aggregate == nil {
			continue
		}
		if aggregate.UnpricedCallCount > 0 {
			usage := &orchestrator.ModelTokenUsage{
				Provider:         aggregate.Provider,
				InputTokens:      aggregate.UnpricedPromptTokens,
				OutputTokens:     aggregate.UnpricedCompletionTokens,
				ReasoningTokens:  aggregate.UnpricedReasoningTokens,
				CacheReadTokens:  aggregate.UnpricedCacheReadTokens,
				CacheWriteTokens: aggregate.UnpricedCacheWriteTokens,
				CacheTokens:      aggregate.UnpricedCacheReadTokens + aggregate.UnpricedCacheWriteTokens,
				LLMCallCount:     aggregate.UnpricedCallCount,
			}
			orchestrator.EnsureModelTokenUsagePricing(modelID, usage)
			if usage.TotalCost > 0 {
				aggregate.TotalCostUSD += usage.TotalCost
				if isPulseSubscriptionProvider(aggregate.Provider) {
					aggregate.SubscriptionShadowUSD += usage.TotalCost
				} else {
					aggregate.TokenEstimateCostUSD += usage.TotalCost
				}
				aggregate.InputCostUSD += usage.InputCost
				aggregate.OutputCostUSD += usage.OutputCost
				aggregate.ReasoningCostUSD += usage.ReasoningCost
				aggregate.CacheReadCostUSD += usage.CacheReadCost
				aggregate.CacheWriteCostUSD += usage.CacheWriteCost
				aggregate.PricingModelID = usage.PricingModelID
				aggregate.PricingVersion = usage.PricingVersion
				aggregate.UnpricedCallCount = 0
			}
		}
		remainingUnpriced += aggregate.UnpricedCallCount
		total += aggregate.TotalCostUSD
	}
	if remainingUnpriced > 0 {
		return total, "captured_unpriced", fmt.Sprintf("%d LLM call(s) captured token usage but have no matching price card", remainingUnpriced)
	}
	return total, "captured", ""
}

func isPulseSubscriptionProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude-code", "claude_code", "codex-cli", "codex_cli", "cursor-cli", "cursor_cli", "pi-cli", "pi_cli":
		return true
	default:
		return false
	}
}

// LoadPulseAgentMetrics returns newest-first measurements. Empty filters match
// all values; a negative limit returns the complete history.
func LoadPulseAgentMetrics(ctx context.Context, workspacePath, pulseRunID, module, role string, limit int) ([]PulseAgentMetricRecord, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return []PulseAgentMetricRecord{}, err
	}
	defer db.Close()
	if err := ensurePulseAgentMetricsSchema(ctx, db); err != nil {
		return nil, err
	}
	query := `SELECT _id, execution_id, agent_session_id, pulse_run_id, review_run_id,
		module, role, status, queued_at, started_at, completed_at, queue_duration_ms,
		duration_ms, llm_call_count, prompt_tokens, completion_tokens, reasoning_tokens,
		cache_read_tokens, cache_write_tokens, total_cost_usd, models_json,
		usage_status, usage_error, recorded_at FROM pulse_agent_metrics WHERE 1=1`
	args := []interface{}{}
	for _, filter := range []struct {
		column string
		value  string
	}{{"pulse_run_id", pulseRunID}, {"module", module}, {"role", role}} {
		if value := strings.TrimSpace(filter.value); value != "" {
			if filter.column == "module" {
				value = pulsemodules.Normalize(value)
			}
			query += " AND " + filter.column + "=?"
			args = append(args, value)
		}
	}
	query += ` ORDER BY completed_at DESC, _id DESC`
	if limit == 0 {
		limit = 200
	}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PulseAgentMetricRecord{}
	for rows.Next() {
		var metric PulseAgentMetricRecord
		var modelsJSON string
		if err := rows.Scan(
			&metric.ID, &metric.ExecutionID, &metric.AgentSessionID, &metric.PulseRunID,
			&metric.ReviewRunID, &metric.Module, &metric.Role, &metric.Status,
			&metric.QueuedAt, &metric.StartedAt, &metric.CompletedAt, &metric.QueueDurationMS,
			&metric.DurationMS, &metric.LLMCallCount, &metric.PromptTokens,
			&metric.CompletionTokens, &metric.ReasoningTokens, &metric.CacheReadTokens,
			&metric.CacheWriteTokens, &metric.TotalCostUSD, &modelsJSON,
			&metric.UsageStatus, &metric.UsageError, &metric.RecordedAt,
		); err != nil {
			return nil, err
		}
		if modelsJSON != "" && modelsJSON != "{}" {
			if err := json.Unmarshal([]byte(modelsJSON), &metric.Models); err != nil {
				return nil, fmt.Errorf("decode Pulse agent models for %s: %w", metric.ExecutionID, err)
			}
		}
		out = append(out, metric)
	}
	return out, rows.Err()
}
