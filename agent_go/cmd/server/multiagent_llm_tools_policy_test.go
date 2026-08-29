package server

import (
	"context"
	"testing"
	"time"
)

type llmToolPolicyRecordingRegistrar struct {
	names map[string]bool
}

func (r *llmToolPolicyRecordingRegistrar) RegisterCustomTool(name, _ string, _ map[string]interface{}, _ func(context.Context, map[string]interface{}) (string, error), _ string) error {
	r.names[name] = true
	return nil
}

func (r *llmToolPolicyRecordingRegistrar) RegisterCustomToolWithTimeout(name, description string, params map[string]interface{}, execute func(context.Context, map[string]interface{}) (string, error), _ time.Duration, category string) error {
	return r.RegisterCustomTool(name, description, params, execute, category)
}

func TestRegisterMultiAgentLLMToolsHonorsProductToolPolicy(t *testing.T) {
	registrar := &llmToolPolicyRecordingRegistrar{names: map[string]bool{}}
	api := &StreamingAPI{}

	if err := api.registerMultiAgentLLMTools(registrar, func(name string) bool {
		return name == "set_provider_auth"
	}); err != nil {
		t.Fatalf("register LLM tools: %v", err)
	}

	if registrar.names["set_provider_auth"] {
		t.Fatal("set_provider_auth was registered despite product policy")
	}
	if !registrar.names["list_llm_capabilities"] {
		t.Fatal("unrelated LLM capability tool was not registered")
	}
	if registrar.names["estimate_llm_cost"] {
		t.Fatal("retired media cost-estimation tool was registered")
	}
}

func TestGenericProductToolRegistrationsHonorToolPolicy(t *testing.T) {
	api := &StreamingAPI{}

	skillRegistrar := &llmToolPolicyRecordingRegistrar{names: map[string]bool{}}
	if err := api.registerMultiAgentSkillTools(skillRegistrar, func(name string) bool {
		return name == "install_skill"
	}); err != nil {
		t.Fatalf("register skill tools: %v", err)
	}
	if skillRegistrar.names["install_skill"] {
		t.Fatal("install_skill was registered despite product policy")
	}
	if !skillRegistrar.names["list_skills"] {
		t.Fatal("unrelated skill tool was not registered")
	}

	mcpRegistrar := &llmToolPolicyRecordingRegistrar{names: map[string]bool{}}
	if err := api.registerMultiAgentMCPServerTools(mcpRegistrar, func(name string) bool {
		return name == "add_mcp_server"
	}); err != nil {
		t.Fatalf("register MCP-server tools: %v", err)
	}
	if mcpRegistrar.names["add_mcp_server"] {
		t.Fatal("add_mcp_server was registered despite product policy")
	}
	if !mcpRegistrar.names["list_mcp_servers"] {
		t.Fatal("unrelated MCP-server tool was not registered")
	}
}
