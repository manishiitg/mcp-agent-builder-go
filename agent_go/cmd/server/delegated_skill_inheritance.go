package server

import (
	"context"
	"strings"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// delegatedParentSkillsContextKey carries the immutable skill snapshot of the
// AgentWorks root agent across the async delegation context boundary.
// These definitions are runtime identity, not persisted request configuration.
type delegatedParentSkillsContextKey struct{}

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
