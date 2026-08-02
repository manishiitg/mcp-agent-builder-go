package step_based_workflow

import (
	"context"
	"time"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// DefinitionToolRegistrar is the builder-owned construction surface used by
// workflow tool factories. It writes into an AgentDefinition draft; it is not
// a mutable runtime Agent API.
type DefinitionToolRegistrar interface {
	RegisterCustomTool(string, string, map[string]interface{}, func(context.Context, map[string]interface{}) (string, error), string) error
	RegisterCustomToolWithTimeout(string, string, map[string]interface{}, func(context.Context, map[string]interface{}) (string, error), time.Duration, string) error
}

type DefinitionRegistrar interface {
	DefinitionToolRegistrar
	AttachedSkills() []*llmtypes.Skill
}
