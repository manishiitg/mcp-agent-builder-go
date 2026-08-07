// Package costobserver turns agent LLM events into cost-ledger entries.
//
// It lives outside cmd/server because more than one launch path spends
// tokens: the chat/query path and delegate() run inside package server, while
// workflow steps and background sub-agents are created by the orchestrator.
// Those orchestrator-created agents previously had no ledger attachment at
// all, so their spend was invisible. Both sides now construct the same
// Observer from here — a second copy of attribution logic would drift.
package costobserver

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	unifiedevents "github.com/manishiitg/mcpagent/events"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/costledger"
)

// Cost scopes. Every launch path must name one; see New for what happens when
// it cannot.
const (
	ScopeChat              = "chat"
	ScopeBuilder           = "builder"
	ScopePulse             = "pulse"
	ScopeEvaluation        = "evaluation"
	ScopeChiefOfStaff      = "chief_of_staff"
	ScopeWorkflowExecution = "workflow_execution"
	// ScopeUnknown is the last-resort value recorded when a launch path did
	// not name its scope. Reaching it is a defect and is logged as one.
	ScopeUnknown = "unknown"
)

// Observer persists immutable per-LLM-call events. The cumulative token_usage
// event remains a compatibility fallback for older providers, but is ignored
// once a per-call completion has been observed.
type Observer struct {
	ledger      *costledger.Ledger
	sessionID   string
	userID      string
	agentMode   string
	provider    string
	modelID     string
	workflowID  string
	runID       string
	executionID string
	scope       string
	launchPath  string

	mu         sync.Mutex
	sawPerCall bool
}

// Option configures an Observer at construction time.
type Option func(*Observer)

// WithModel records the provider and model the caller requested. The provider
// may still be overridden per event by what the adapter actually reports.
func WithModel(provider, modelID string) Option {
	return func(o *Observer) {
		o.provider = strings.TrimSpace(provider)
		o.modelID = strings.TrimSpace(modelID)
	}
}

// WithAttribution names the cost scope and the workflow/run/execution this
// agent's spend belongs to. scope is required; see New.
func WithAttribution(scope, workflowID, runID, executionID string) Option {
	return func(o *Observer) {
		o.scope = strings.TrimSpace(scope)
		o.workflowID = strings.TrimSpace(workflowID)
		o.runID = strings.TrimSpace(runID)
		o.executionID = strings.TrimSpace(executionID)
	}
}

// WithLaunchPath names the code path that created this agent. It is only used
// to make an unattributed observer identifiable in the logs.
func WithLaunchPath(launchPath string) Option {
	return func(o *Observer) {
		o.launchPath = strings.TrimSpace(launchPath)
	}
}

// New builds an observer for one agent.
//
// The scope is not defaulted. A launch path that cannot name its scope is a
// bug, and silently writing "unknown" is how 15k rows ended up unattributable:
// nothing downstream can tell a genuinely unclassifiable event from a caller
// that forgot. Such a path is still recorded — losing the tokens would be
// worse than misfiling them — but it logs an error naming itself first.
func New(ledger *costledger.Ledger, sessionID, userID, agentMode string, opts ...Option) *Observer {
	observer := &Observer{
		ledger:    ledger,
		sessionID: sessionID,
		userID:    userID,
		agentMode: agentMode,
	}
	for _, opt := range opts {
		opt(observer)
	}
	if observer.scope == "" {
		launchPath := observer.launchPath
		if launchPath == "" {
			launchPath = "<unnamed launch path>"
		}
		log.Printf("[COST_LEDGER] %s did not name a cost scope (session=%q agent_mode=%q); recording as %q",
			launchPath, sessionID, agentMode, ScopeUnknown)
		observer.scope = ScopeUnknown
	}
	return observer
}

// Scope reports the scope every entry from this observer carries.
func (o *Observer) Scope() string {
	if o == nil {
		return ""
	}
	return o.scope
}

// ExecutionID reports the execution every entry from this observer is grouped
// under.
func (o *Observer) ExecutionID() string {
	if o == nil {
		return ""
	}
	return o.executionID
}

// Name identifies this listener to the agent runtime.
func (o *Observer) Name() string { return "costObserver" }

// HandleEvent implements the agent event listener contract.
func (o *Observer) HandleEvent(_ context.Context, event *unifiedevents.AgentEvent) error {
	if o == nil || o.ledger == nil || event == nil {
		return nil
	}
	switch event.Type {
	case unifiedevents.LLMGenerationEnd:
		generation, ok := event.Data.(*unifiedevents.LLMGenerationEndEvent)
		if !ok || generation == nil {
			return nil
		}
		o.mu.Lock()
		o.sawPerCall = true
		o.mu.Unlock()
		o.recordLLMGeneration(event, generation)
	case unifiedevents.TokenUsage:
		o.mu.Lock()
		sawPerCall := o.sawPerCall
		o.mu.Unlock()
		if sawPerCall {
			return nil
		}
		tu, ok := event.Data.(*unifiedevents.TokenUsageEvent)
		if !ok || tu == nil {
			return nil
		}
		o.recordLegacyTokenUsage(event, tu)
	}
	return nil
}

func (o *Observer) recordLLMGeneration(event *unifiedevents.AgentEvent, generation *unifiedevents.LLMGenerationEndEvent) {
	metadata := generation.Metadata
	cacheRead, cacheWrite := ExtractCacheTokens(metadata)
	effectiveModel, estimatedCost := ExtractCostAndEffectiveModel(metadata)
	provider := FirstNonEmpty(stringValue(metadata["provider"]), o.provider)
	modelID := o.modelID
	if modelID == "" {
		modelID = stringValue(metadata["requested_model_id"])
	}
	totalCostUSD := numberValue(metadata["cost_usd"])
	billingBasis := "provider_actual"
	pricingSource := "provider"
	if totalCostUSD <= 0 && estimatedCost > 0 {
		totalCostUSD = estimatedCost
		billingBasis = "token_estimate"
		pricingSource = "model_registry"
		if isSubscriptionCodingProvider(provider) {
			billingBasis = "subscription_shadow"
		}
	}
	if totalCostUSD <= 0 {
		billingBasis = "unpriced"
		pricingSource = ""
	}
	if effectiveModel == "" {
		effectiveModel = modelID
	}
	entry := o.baseEntry(event)
	entry.Provider = provider
	entry.ModelID = modelID
	entry.EffectiveProvider = provider
	entry.EffectiveModelID = effectiveModel
	entry.TurnCount = 1
	entry.LLMCallCount = 1
	entry.PromptTokens = generation.UsageMetrics.PromptTokens
	entry.CompletionTokens = generation.UsageMetrics.CompletionTokens
	entry.ReasoningTokens = generation.UsageMetrics.ReasoningTokens
	entry.CacheReadTokens = firstPositive(cacheRead, generation.UsageMetrics.CacheTokens)
	entry.CacheWriteTokens = cacheWrite
	entry.TotalCostUSD = totalCostUSD
	entry.BillingBasis = billingBasis
	entry.PricingSource = pricingSource
	o.append(entry)
}

func (o *Observer) recordLegacyTokenUsage(event *unifiedevents.AgentEvent, tu *unifiedevents.TokenUsageEvent) {
	cacheRead, cacheWrite := ExtractCacheTokens(tu.GenerationInfo)
	effectiveModel, estimatedCost := ExtractCostAndEffectiveModel(tu.GenerationInfo)
	provider := FirstNonEmpty(tu.Provider, o.provider)
	modelID := FirstNonEmpty(tu.ModelID, o.modelID)
	totalCostUSD := numberValue(tu.GenerationInfo["cost_usd"])
	billingBasis := "provider_actual"
	pricingSource := "provider"
	if totalCostUSD <= 0 && estimatedCost > 0 {
		totalCostUSD = estimatedCost
		billingBasis = "token_estimate"
		pricingSource = "model_registry"
		if isSubscriptionCodingProvider(provider) {
			billingBasis = "subscription_shadow"
		}
	} else if totalCostUSD <= 0 && tu.TotalCost > 0 {
		// TotalCost on the cumulative event is generally computed from model
		// metadata. It must not be labeled as a provider invoice.
		totalCostUSD = tu.TotalCost
		billingBasis = "token_estimate"
		pricingSource = "agent_token_pricing"
		if isSubscriptionCodingProvider(provider) {
			billingBasis = "subscription_shadow"
		}
	}
	if totalCostUSD <= 0 {
		billingBasis = "unpriced"
		pricingSource = ""
	}
	if effectiveModel == "" {
		effectiveModel = modelID
	}
	entry := o.baseEntry(event)
	entry.Provider = provider
	entry.ModelID = modelID
	entry.EffectiveProvider = provider
	entry.EffectiveModelID = effectiveModel
	entry.TurnCount = 1
	entry.LLMCallCount = ToInt(tu.GenerationInfo["llm_call_count"])
	if entry.LLMCallCount == 0 && (tu.PromptTokens > 0 || tu.CompletionTokens > 0 || tu.ReasoningTokens > 0 || totalCostUSD > 0) {
		entry.LLMCallCount = 1
	}
	entry.PromptTokens = tu.PromptTokens
	entry.CompletionTokens = tu.CompletionTokens
	entry.ReasoningTokens = tu.ReasoningTokens
	entry.CacheReadTokens = cacheRead
	entry.CacheWriteTokens = cacheWrite
	entry.TotalCostUSD = totalCostUSD
	entry.BillingBasis = billingBasis
	entry.PricingSource = pricingSource
	o.append(entry)
}

func (o *Observer) baseEntry(event *unifiedevents.AgentEvent) costledger.Entry {
	eventID := strings.TrimSpace(event.SpanID)
	if eventID == "" {
		eventID = strings.TrimSpace(event.CorrelationID)
	}
	storedEventID := ""
	idempotencyKey := ""
	if eventID != "" {
		storedEventID = strings.Join([]string{o.sessionID, eventID}, ":")
		idempotencyKey = storedEventID
	}
	return costledger.Entry{
		EventID:        storedEventID,
		IdempotencyKey: idempotencyKey,
		Timestamp:      event.Timestamp,
		SessionID:      o.sessionID,
		UserID:         o.userID,
		WorkflowID:     o.workflowID,
		RunID:          o.runID,
		ExecutionID:    o.executionID,
		Scope:          o.scope,
		AgentMode:      o.agentMode,
		Component:      event.Component,
		CorrelationID:  event.CorrelationID,
	}
}

func (o *Observer) append(entry costledger.Entry) {
	if err := o.ledger.Append(entry); err != nil {
		log.Printf("[COST_LEDGER] Failed to append entry: %v", err)
	}
}

// matchPhaseScope classifies the scope signals a phase/step identifier can
// carry. It returns "" when none of them apply, so each caller keeps its own
// default rather than inheriting another path's.
func matchPhaseScope(values ...string) string {
	joined := strings.ToLower(strings.TrimSpace(strings.Join(values, " ")))
	switch {
	case strings.Contains(joined, "post_run_monitor"), strings.Contains(joined, "pulse"):
		return ScopePulse
	case strings.Contains(joined, "evaluation"), strings.Contains(joined, "eval"):
		return ScopeEvaluation
	}
	return ""
}

// InferScope names the scope for a chat-session agent — the chat/query path
// and delegate() sub-agents. Anything not otherwise identified is chat.
func InferScope(agentMode, phaseID string) string {
	if scope := matchPhaseScope(phaseID); scope != "" {
		return scope
	}
	mode := strings.ToLower(strings.TrimSpace(agentMode))
	switch {
	case strings.Contains(mode, "chief"):
		return ScopeChiefOfStaff
	case strings.Contains(mode, "workflow"):
		return ScopeBuilder
	default:
		return ScopeChat
	}
}

// InferWorkflowScope names the scope for an agent created by the workflow
// orchestrator: a workflow step or a background sub-agent.
//
// hasRunFolder is the signal that separates a live workflow execution from
// builder-side work: the orchestrator only sets an iteration/run folder once
// it is executing into runs/. It is a real distinction rather than a guess at
// the agent's name, because both are driven by the same controller type and
// the same agent mode.
func InferWorkflowScope(agentMode string, hasRunFolder bool, identifiers ...string) string {
	if scope := matchPhaseScope(identifiers...); scope != "" {
		return scope
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(agentMode)), "chief") {
		return ScopeChiefOfStaff
	}
	if hasRunFolder {
		return ScopeWorkflowExecution
	}
	return ScopeBuilder
}

func isSubscriptionCodingProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude-code", "claude_code", "codex-cli", "codex_cli", "cursor-cli", "cursor_cli":
		return true
	default:
		return false
	}
}

func stringValue(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func numberValue(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		value, _ := n.Float64()
		return value
	default:
		return 0
	}
}

// FirstNonEmpty returns the first trimmed non-empty value.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// ExtractCostAndEffectiveModel pulls the unified cost+model fields the
// CLI adapters now emit on GenerationInfo.Additional:
//
//	cost_usd_estimated  — float, computed from tokens × registry rates
//	cost_model_id       — string, model used for the rate lookup
//	<provider>_effective_model / claude_code_model / cursor_model /
//	  gemini_effective_model / codex_effective_model — string, the model
//	  the CLI actually served the turn with
//
// Returns ("", 0) when none of those are present.
func ExtractCostAndEffectiveModel(info map[string]interface{}) (effectiveModel string, estimatedCost float64) {
	if info == nil {
		return "", 0
	}
	if v, ok := info["cost_usd_estimated"]; ok {
		switch n := v.(type) {
		case float64:
			estimatedCost = n
		case float32:
			estimatedCost = float64(n)
		}
	}
	for _, key := range []string{
		"cost_model_id",
		"claude_code_model",
		"codex_effective_model",
		"gemini_effective_model",
		"cursor_model",
	} {
		if v, ok := info[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				effectiveModel = s
				break
			}
		}
	}
	return effectiveModel, estimatedCost
}

// ExtractCacheTokens pulls cache_read_input_tokens / cache_creation_input_tokens
// out of the provider's raw generation info blob. Providers that don't report
// cache usage just return zeros.
func ExtractCacheTokens(info map[string]interface{}) (read, write int) {
	if info == nil {
		return 0, 0
	}
	if v, ok := info["cache_read_input_tokens"]; ok {
		read = ToInt(v)
	}
	if v, ok := info["cache_creation_input_tokens"]; ok {
		write = ToInt(v)
	}
	return read, write
}

// ToInt coerces the numeric shapes provider metadata arrives in to an int.
func ToInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return int(f)
		}
	}
	return 0
}
