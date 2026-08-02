package server

import (
	"context"
	"time"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// definitionToolRegistrar is implemented by the builder wrapper. It mutates
// only the pre-run AgentDefinition draft, never the live mcpagent runtime.
type definitionToolRegistrar interface {
	RegisterCustomTool(string, string, map[string]interface{}, func(context.Context, map[string]interface{}) (string, error), string) error
	RegisterCustomToolWithTimeout(string, string, map[string]interface{}, func(context.Context, map[string]interface{}) (string, error), time.Duration, string) error
}

type definitionRegistrar interface {
	definitionToolRegistrar
	AttachSkill(*llmtypes.Skill) error
	AttachedSkills() []*llmtypes.Skill
}
