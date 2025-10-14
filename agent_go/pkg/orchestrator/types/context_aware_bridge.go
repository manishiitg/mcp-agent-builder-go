package types

import (
	"context"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/events"
	"mcp-agent/agent_go/pkg/mcpagent"
	"sync"
)

// ContextAwareEventBridge wraps an existing AgentEventListener and adds orchestrator context
type ContextAwareEventBridge struct {
	underlyingBridge mcpagent.AgentEventListener
	currentPhase     string
	currentStep      int
	currentIteration int
	currentAgentName string
	mu               sync.RWMutex
	logger           utils.ExtendedLogger
}

// NewContextAwareEventBridge creates a new context-aware event bridge
func NewContextAwareEventBridge(underlyingBridge mcpagent.AgentEventListener, logger utils.ExtendedLogger) *ContextAwareEventBridge {
	return &ContextAwareEventBridge{
		underlyingBridge: underlyingBridge,
		logger:           logger,
	}
}

// SetOrchestratorContext sets the current orchestrator context
func (c *ContextAwareEventBridge) SetOrchestratorContext(phase string, step, iteration int, agentName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentPhase = phase
	c.currentStep = step
	c.currentIteration = iteration
	c.currentAgentName = agentName

	c.logger.Infof("🎯 Set orchestrator context: %s (step %d, iteration %d)", phase, step+1, iteration+1)
}

// ClearOrchestratorContext clears the orchestrator context
func (c *ContextAwareEventBridge) ClearOrchestratorContext() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentPhase = ""
	c.currentStep = 0
	c.currentIteration = 0
	c.currentAgentName = ""

	c.logger.Infof("🧹 Cleared orchestrator context")
}

// HandleEvent implements AgentEventListener interface
func (c *ContextAwareEventBridge) HandleEvent(ctx context.Context, event *events.AgentEvent) error {
	c.logger.Infof("🔍 ContextAwareBridge: Received event %s", event.Type)

	// Add orchestrator context to the event if we have context
	c.mu.RLock()
	if c.currentPhase != "" {
		c.logger.Infof("🔍 ContextAwareBridge: Processing event %s with phase %s", event.Type, c.currentPhase)

		// Add orchestrator context to metadata
		// We need to check if the event data has a BaseEventData field
		if eventData, ok := event.Data.(interface {
			GetBaseEventData() *events.BaseEventData
		}); ok {
			baseData := eventData.GetBaseEventData()
			if baseData.Metadata == nil {
				baseData.Metadata = make(map[string]interface{})
			}
			baseData.Metadata["orchestrator_phase"] = c.currentPhase
			baseData.Metadata["orchestrator_step"] = c.currentStep
			baseData.Metadata["orchestrator_iteration"] = c.currentIteration
			baseData.Metadata["orchestrator_agent_name"] = c.currentAgentName

			c.logger.Infof("✅ ContextAwareBridge: Added metadata to event %s", event.Type)
		} else {
			c.logger.Warnf("⚠️ ContextAwareBridge: Event data %T does not have GetBaseEventData method", event.Data)
		}
	}
	c.mu.RUnlock()

	// Forward to underlying bridge
	c.logger.Infof("🔍 ContextAwareBridge: Forwarding event %s to underlying bridge", event.Type)
	err := c.underlyingBridge.HandleEvent(ctx, event)
	if err != nil {
		c.logger.Warnf("⚠️ ContextAwareBridge: Error forwarding event %s: %v", event.Type, err)
	} else {
		c.logger.Infof("✅ ContextAwareBridge: Successfully forwarded event %s", event.Type)
	}
	return err
}

// Name implements AgentEventListener interface
func (c *ContextAwareEventBridge) Name() string {
	return "context_aware_bridge"
}
