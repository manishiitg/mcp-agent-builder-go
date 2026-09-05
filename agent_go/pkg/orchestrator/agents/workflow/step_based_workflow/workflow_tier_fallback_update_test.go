package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPersistTierFallbackUpdate(t *testing.T) {
	const initial = `{"capabilities":{"llm_config":{"mode":"explicit","tier_1":{"provider":"openai","model_id":"primary","custom":"keep"}},"other":"keep"},"label":"keep"}`
	for _, tc := range []struct {
		name, content        string
		raw                  interface{}
		failWrite, wantError bool
	}{
		{"valid", initial, map[string]interface{}{"tier_1": []interface{}{map[string]interface{}{"provider": "openai", "model_id": "backup"}}}, false, false},
		{"clear", initial, map[string]interface{}{"tier_1": []interface{}{}}, false, false},
		{"profile", strings.Replace(initial, "explicit", "provider_profile", 1), map[string]interface{}{"tier_1": []interface{}{}}, false, true},
		{"missing-primary", initial, map[string]interface{}{"tier_2": []interface{}{}}, false, true},
		{"invalid", initial, map[string]interface{}{"tier_1": []interface{}{map[string]interface{}{"provider": "openai"}}}, false, true},
		{"write-error", initial, map[string]interface{}{"tier_1": []interface{}{}}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := tc.content
			cfg, err := persistTierFallbackUpdate(context.Background(), tc.raw, func(context.Context, string) (string, error) { return stored, nil }, func(_ context.Context, _ string, value string) error {
				if tc.failWrite {
					return fmt.Errorf("denied")
				}
				stored = value
				return nil
			}, nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v", err)
			}
			if tc.wantError {
				if stored != tc.content || cfg != nil {
					t.Fatal("failed update changed persisted/runtime config")
				}
				return
			}
			if !strings.Contains(stored, `"other": "keep"`) || !strings.Contains(stored, `"custom": "keep"`) {
				t.Fatal("lost unrelated fields")
			}
			if tc.name == "valid" && (len(cfg.Tier1.Fallbacks) != 1 || cfg.Tier1.Fallbacks[0].ModelID != "backup" || !strings.Contains(stored, "backup")) {
				t.Fatal("fallback did not persist")
			}
		})
	}
}

func TestPersistTierFallbackUpdatePreservesPublishedPrimary(t *testing.T) {
	const manifest = `{"capabilities":{"llm_config":{"mode":"explicit","tier_1":{"published_llm_id":"primary"}}}}`
	for _, id := range []string{"primary", "different", ""} {
		t.Run(id, func(t *testing.T) {
			stored := manifest
			current := &TieredLLMConfig{Tier1: &AgentLLMConfig{PublishedLLMID: id, Provider: "openai", ModelID: "resolved", Options: map[string]interface{}{"effort": "high"}}}
			updated, err := persistTierFallbackUpdate(context.Background(), map[string]interface{}{"tier_1": []interface{}{map[string]interface{}{"provider": "openai", "model_id": "backup"}}}, func(context.Context, string) (string, error) { return stored, nil }, func(_ context.Context, _ string, v string) error { stored = v; return nil }, current)
			if id != "primary" {
				if err == nil || stored != manifest {
					t.Fatal("unresolved primary was persisted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if NewTierResolver(updated, nil).ResolveTier(TierHigh) == nil || updated.Tier1.ModelID != "resolved" || updated.Tier1.Options["effort"] != "high" {
				t.Fatal("lost resolved primary")
			}
			if len(current.Tier1.Fallbacks) != 0 {
				t.Fatal("mutated old runtime")
			}
			if strings.Contains(stored, "resolved") || !strings.Contains(stored, "backup") {
				t.Fatal("must preserve published reference on disk and persist fallback")
			}
		})
	}
}
