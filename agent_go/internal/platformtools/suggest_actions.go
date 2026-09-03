// Package platformtools holds agent-profile tool factories that belong to
// the platform rather than to one product: any product.yaml can bind them,
// and what they render in the chat is declared on the binding.
package platformtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	orchestratorevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// SuggestActionsToolID is the binding id a product.yaml uses for the tool.
const SuggestActionsToolID = "platform.suggest-actions"

const toolCategory = "platform_tools"

// RegisterAgentProfileTools registers every platform-owned tool factory.
func RegisterAgentProfileTools(registry *agentprofiles.Registry) error {
	return registry.RegisterToolFactory(SuggestActionsToolID, SuggestActionsFactory())
}

// interactionKind is the event kind the binding declared for this tool
// (product.yaml `interaction.kind`), or the platform default.
func interactionKind(runtime agentprofiles.ToolRuntimeContext, fallback string) string {
	if runtime.Interaction != nil && strings.TrimSpace(runtime.Interaction.Kind) != "" {
		return strings.TrimSpace(runtime.Interaction.Kind)
	}
	return fallback
}

func emitInteraction(runtime agentprofiles.ToolRuntimeContext, kind string, payload map[string]interface{}) {
	if runtime.Emit == nil {
		return
	}
	product := strings.TrimSpace(runtime.Product)
	if product == "" {
		product = "platform"
	}
	runtime.Emit(&orchestratorevents.ProductInteractionEvent{Product: product, Kind: kind, Payload: payload})
}

func stringArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

// SuggestActionsFactory builds suggest_actions: tappable next steps under the
// reply. The surface renders them when the binding declares
// `interaction: {kind, render: chat.suggestions}`; the payload is
// {actions: [{label, message}]}, message being sent as if the user typed it.
func SuggestActionsFactory() agentprofiles.ToolFactory {
	return func(runtime agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		return agentprofiles.ToolSpec{
			Name: "suggest_actions", Category: toolCategory,
			Description: "Offer the user 2–4 tappable next steps under your reply. Each has a short label (2–4 words) and the exact message sent, as if the user typed it, when tapped. Use it when there is a real next move worth one tap, not as a sign-off on every reply.",
			Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
				"actions": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "properties": map[string]interface{}{
					"label":   map[string]interface{}{"type": "string", "description": "short button text, 2–4 words"},
					"message": map[string]interface{}{"type": "string", "description": "the message sent as the user when tapped"},
				}, "required": []string{"label", "message"}}},
			}, "required": []string{"actions"}},
			Execute: func(_ context.Context, args map[string]interface{}) (string, error) {
				raw, _ := args["actions"].([]interface{})
				var actions []map[string]interface{}
				for _, it := range raw {
					m, ok := it.(map[string]interface{})
					if !ok {
						continue
					}
					label, message := stringArg(m, "label"), stringArg(m, "message")
					if label == "" || message == "" {
						continue
					}
					actions = append(actions, map[string]interface{}{"label": label, "message": message})
				}
				if len(actions) == 0 {
					return "", fmt.Errorf("suggest_actions: give at least one action with both a label and a message")
				}
				emitInteraction(runtime, interactionKind(runtime, "suggestions"), map[string]interface{}{"actions": actions})
				out, _ := json.Marshal(map[string]interface{}{"status": "ok", "count": len(actions)})
				return string(out), nil
			},
		}, nil
	}
}
