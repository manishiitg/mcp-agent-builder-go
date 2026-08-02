package step_based_workflow

import (
	"context"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// backgroundAgentSkillsContextKey carries the immutable skill snapshot from a
// workshop builder into the synthetic todo-task step used by
// run_in_background(agent_type="orchestrator"). It is runtime identity, not
// persisted step configuration.
type backgroundAgentSkillsContextKey struct{}

func withBackgroundAgentSkills(ctx context.Context, inherited []*llmtypes.Skill) context.Context {
	if len(inherited) == 0 {
		return ctx
	}
	return context.WithValue(ctx, backgroundAgentSkillsContextKey{}, append([]*llmtypes.Skill(nil), inherited...))
}

func backgroundAgentSkillsFromContext(ctx context.Context) []*llmtypes.Skill {
	if ctx == nil {
		return nil
	}
	inherited, _ := ctx.Value(backgroundAgentSkillsContextKey{}).([]*llmtypes.Skill)
	return append([]*llmtypes.Skill(nil), inherited...)
}

func backgroundSkillNames(inherited []*llmtypes.Skill) []string {
	names := make([]string, 0, len(inherited))
	seen := make(map[string]struct{}, len(inherited))
	for _, skill := range inherited {
		if skill == nil {
			continue
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// applyInheritedBackgroundSkills adds the builder's attached skill definitions
// to an isolated background child without reloading by name. Passing the full
// definitions preserves scripts, references, and assets when mcpagent projects
// the skills into the child's temporary coding-CLI workspace.
func applyInheritedBackgroundSkills(ctx context.Context, baseAgent *agents.BaseAgent, inherited []*llmtypes.Skill) error {
	if baseAgent == nil || len(inherited) == 0 {
		return nil
	}

	var existing []*llmtypes.Skill
	if child := baseAgent.Agent(); child != nil {
		existing = child.Definition().SkillDefinitions
	}
	missing := missingBackgroundSkills(existing, inherited)
	if len(missing) == 0 {
		return nil
	}
	return baseAgent.ApplyIdentity(ctx, missing)
}

func missingBackgroundSkills(existing, inherited []*llmtypes.Skill) []*llmtypes.Skill {
	seen := make(map[string]struct{}, len(existing)+len(inherited))
	for _, skill := range existing {
		if skill == nil {
			continue
		}
		if name := strings.TrimSpace(skill.Name); name != "" {
			seen[name] = struct{}{}
		}
	}
	missing := make([]*llmtypes.Skill, 0, len(inherited))
	for _, skill := range inherited {
		if skill == nil {
			continue
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		missing = append(missing, skill)
	}
	return missing
}
