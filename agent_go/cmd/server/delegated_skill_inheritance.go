package server

import (
	"context"
	"strings"

	agent "github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentwrapper"
	mcpagent "github.com/manishiitg/mcpagent/agent"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// delegatedParentSkillsContextKey carries the immutable skill snapshot of the
// AgentWorks root agent across the async delegation context boundary.
// These definitions are runtime identity, not persisted request configuration.
type delegatedParentSkillsContextKey struct{}

func withDelegatedParentSkills(ctx context.Context, parent *mcpagent.Agent) context.Context {
	if parent == nil {
		return ctx
	}
	return withDelegatedParentSkillDefinitions(ctx, parent.Definition().SkillDefinitions)
}

func withDelegatedParentSkillDefinitions(ctx context.Context, inherited []*llmtypes.Skill) context.Context {
	if len(inherited) == 0 {
		return ctx
	}
	return context.WithValue(ctx, delegatedParentSkillsContextKey{}, append([]*llmtypes.Skill(nil), inherited...))
}

func delegatedParentSkillsFromContext(ctx context.Context) []*llmtypes.Skill {
	if ctx == nil {
		return nil
	}
	inherited, _ := ctx.Value(delegatedParentSkillsContextKey{}).([]*llmtypes.Skill)
	return append([]*llmtypes.Skill(nil), inherited...)
}

func copyDelegatedParentSkills(from, to context.Context) context.Context {
	return withDelegatedParentSkillDefinitions(to, delegatedParentSkillsFromContext(from))
}

// attachMissingDelegatedSkills preserves full skill bundles while avoiding a
// duplicate when an explicitly requested skill is already attached to the
// parent AgentWorks chat. The coding-agent adapter later projects these exact
// definitions into the delegated agent's dedicated runtime directory.
func attachMissingDelegatedSkills(subAgent *agent.LLMAgentWrapper, candidates []*llmtypes.Skill) (int, error) {
	if subAgent == nil || len(candidates) == 0 {
		return 0, nil
	}

	seen := make(map[string]struct{}, len(candidates))
	if underlying := subAgent.GetUnderlyingAgent(); underlying != nil {
		for _, skill := range underlying.Definition().SkillDefinitions {
			if skill == nil {
				continue
			}
			if name := strings.TrimSpace(skill.Name); name != "" {
				seen[name] = struct{}{}
			}
		}
	}

	missing := uniqueDelegatedSkills(seen, candidates)
	attached := 0
	for _, skill := range missing {
		if err := subAgent.AttachSkill(skill); err != nil {
			return attached, err
		}
		attached++
	}
	return attached, nil
}

func uniqueDelegatedSkills(seen map[string]struct{}, candidates []*llmtypes.Skill) []*llmtypes.Skill {
	if seen == nil {
		seen = make(map[string]struct{}, len(candidates))
	}
	missing := make([]*llmtypes.Skill, 0, len(candidates))
	for _, skill := range candidates {
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
