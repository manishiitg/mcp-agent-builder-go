package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// persistTierFallbackUpdate validates the entire patch before writing. Runtime
// configuration is returned only after the durable write succeeds.
func persistTierFallbackUpdate(ctx context.Context, raw interface{}, read func(context.Context, string) (string, error), write func(context.Context, string, string) error, current *TieredLLMConfig) (*TieredLLMConfig, error) {
	patch, ok := raw.(map[string]interface{})
	if !ok || len(patch) == 0 {
		return nil, fmt.Errorf("update_tier_fallbacks requires a non-empty tier object")
	}
	content, err := read(ctx, "workflow.json")
	if err != nil {
		return nil, fmt.Errorf("read fallback configuration: %w", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		return nil, fmt.Errorf("decode fallback configuration: %w", err)
	}
	caps, _ := manifest["capabilities"].(map[string]interface{})
	cfg, _ := caps["llm_config"].(map[string]interface{})
	if cfg["mode"] != "explicit" {
		return nil, fmt.Errorf("update_tier_fallbacks requires explicit LLM mode; current mode is %v. Use set_workflow_llm_config for an approved model-routing change", cfg["mode"])
	}
	for tier, value := range patch {
		if tier != "tier_1" && tier != "tier_2" && tier != "tier_3" {
			return nil, fmt.Errorf("unknown fallback tier %q", tier)
		}
		primary, ok := cfg[tier].(map[string]interface{})
		modelID, _ := primary["model_id"].(string)
		publishedID, _ := primary["published_llm_id"].(string)
		if !ok || (strings.TrimSpace(modelID) == "" && strings.TrimSpace(publishedID) == "") {
			return nil, fmt.Errorf("%s has no primary LLM configured", tier)
		}
		items, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("%s fallbacks must be an array; use [] to clear", tier)
		}
		for _, item := range items {
			entry, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("%s fallback must be an object", tier)
			}
			provider, _ := entry["provider"].(string)
			model, _ := entry["model_id"].(string)
			if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
				return nil, fmt.Errorf("%s fallback requires provider and model_id", tier)
			}
		}
		primary["fallbacks"] = items
	}
	encodedConfig, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var updated TieredLLMConfig
	if err := json.Unmarshal(encodedConfig, &updated); err != nil {
		return nil, fmt.Errorf("invalid fallback configuration: %w", err)
	}
	// Published references on disk are not executable provider/model pairs.
	// Reuse only the matching, already-resolved primary from this session;
	// never substitute a different published entry or erase resolved options.
	resolved := []*AgentLLMConfig{nil, nil, nil}
	if current != nil {
		resolved = []*AgentLLMConfig{current.Tier1, current.Tier2, current.Tier3}
	}
	for i, tier := range []*AgentLLMConfig{updated.Tier1, updated.Tier2, updated.Tier3} {
		if tier == nil {
			continue
		}
		if tier.PublishedLLMID != "" {
			prior := resolved[i]
			if prior == nil || prior.PublishedLLMID != tier.PublishedLLMID || prior.Provider == "" || prior.ModelID == "" {
				return nil, fmt.Errorf("tier_%d published primary %q is not resolved in this session; reload the workflow configuration first", i+1, tier.PublishedLLMID)
			}
			tier.Provider, tier.ModelID = prior.Provider, prior.ModelID
			// Copy via JSON to keep the returned runtime independent of current.
			options, err := json.Marshal(prior.Options)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(options, &tier.Options); err != nil {
				return nil, err
			}
		}
		if tier.Provider == "" || tier.ModelID == "" {
			return nil, fmt.Errorf("tier_%d primary is not executable", i+1)
		}
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := write(ctx, "workflow.json", string(encoded)); err != nil {
		return nil, fmt.Errorf("persist tier fallbacks: %w", err)
	}
	return &updated, nil
}
