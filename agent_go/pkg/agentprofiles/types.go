package agentprofiles

import (
	"context"
	"encoding/json"
)

type CapabilityRequirement string

const (
	CapabilityRequired  CapabilityRequirement = "required"
	CapabilityPreferred CapabilityRequirement = "preferred"
	CapabilityOptional  CapabilityRequirement = "optional"
	CapabilityDisabled  CapabilityRequirement = "disabled"
)

type RuntimePolicy struct {
	Transport       string              `json:"transport" yaml:"transport"`
	Provider        string              `json:"provider,omitempty" yaml:"provider,omitempty"`
	ModelID         string              `json:"model_id,omitempty" yaml:"model_id,omitempty"`
	ProviderOptions []ProviderOption    `json:"provider_options,omitempty" yaml:"provider_options,omitempty"`
	Capabilities    RuntimeCapabilities `json:"capabilities" yaml:"capabilities"`
	// AgentTools selects whether a coding provider receives only the MCP bridge
	// (mcp_only) or both bridge and native tools (hybrid). Empty preserves
	// mcp_only for existing profiles.
	AgentTools AgentToolsPolicy `json:"agent_tools,omitempty" yaml:"agent_tools,omitempty"`
	// Approvals controls the native-tool approval policy when AgentTools enables
	// native tools. Empty preserves provider_auto.
	Approvals ApprovalsPolicy `json:"approvals,omitempty" yaml:"approvals,omitempty"`
}

type AgentToolsPolicy struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

type ApprovalsPolicy struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// ProviderOption is a product-approved coding-agent runtime choice. The
// profile runtime remains authoritative: a request may select only one of
// these YAML-declared bindings, never an arbitrary global AgentWorks model.
type ProviderOption struct {
	ID       string `json:"id" yaml:"id"`
	Label    string `json:"label" yaml:"label"`
	Provider string `json:"provider" yaml:"provider"`
	ModelID  string `json:"model_id" yaml:"model_id"`
	Default  bool   `json:"default,omitempty" yaml:"default,omitempty"`
}

type RuntimeCapabilities struct {
	LiveInput         CapabilityRequirement `json:"live_input" yaml:"live_input"`
	RawTerminal       CapabilityRequirement `json:"raw_terminal" yaml:"raw_terminal"`
	WarmSession       CapabilityRequirement `json:"warm_session" yaml:"warm_session"`
	WorkflowExecution CapabilityRequirement `json:"workflow_execution,omitempty" yaml:"workflow_execution,omitempty"`
	// Browser controls access to AgentWorks' shared managed agent-browser
	// capability. Products opt in by declaring a requirement; they never carry
	// their own copy of the browser tool or its version-matched skill.
	Browser CapabilityRequirement `json:"browser,omitempty" yaml:"browser,omitempty"`
	// Secrets enables the shared AgentWorks encrypted-secret selection and
	// injection flow for a product agent. Values are never part of the prompt;
	// selected values are supplied only as shell environment variables.
	Secrets CapabilityRequirement `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

type ToolBinding struct {
	ID     string          `json:"id" yaml:"id"`
	Config json.RawMessage `json:"config,omitempty" yaml:"config,omitempty"`
}

type Profile struct {
	ID                   string        `json:"id" yaml:"id"`
	Name                 string        `json:"name" yaml:"name"`
	Version              int           `json:"version" yaml:"version"`
	SystemPromptTemplate string        `json:"system_prompt" yaml:"system_prompt"`
	Skills               []string      `json:"skills,omitempty" yaml:"skills,omitempty"`
	Tools                []ToolBinding `json:"tools,omitempty" yaml:"tools,omitempty"`
	ToolPolicy           ToolPolicy    `json:"tool_policy,omitempty" yaml:"tool_policy,omitempty"`
	Runtime              RuntimePolicy `json:"runtime" yaml:"runtime"`
	BuiltIn              bool          `json:"built_in" yaml:"built_in"`
	OwnerID              string        `json:"owner_id,omitempty" yaml:"owner_id,omitempty"`
}

// ToolPolicy controls generic AgentWorks capabilities a product receives.
// Product-specific tools are still declared in Tools above.
type ToolPolicy struct {
	Disabled []string `json:"disabled,omitempty" yaml:"disabled,omitempty"`
}

type PromptContext struct {
	ProjectTitle         string `json:"project_title"`
	LocalDateTime        string `json:"local_date_time,omitempty"`
	WorkspaceDescription string `json:"workspace_description,omitempty"`
}

type ToolRuntimeContext struct {
	UserID        string
	SessionID     string
	WorkspacePath string
	Emit          func(event any)
}

// RuntimeContext contains trusted, server-resolved state for a profile turn.
// Profile initializers use it to prepare durable workspace-owned state before
// the agent definition is finalized. Values in this struct are never accepted
// from tool arguments.
type RuntimeContext struct {
	UserID        string
	SessionID     string
	WorkspacePath string
}

type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Category    string
	Execute     func(context.Context, map[string]interface{}) (string, error)
}

type ToolFactory func(ToolRuntimeContext, json.RawMessage) (ToolSpec, error)

type RuntimeInitializer func(context.Context, RuntimeContext) error
