package agentprofiles

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// SkillFileBinding is one product-owned skill: a stable name and description
// the agent sees when deciding whether to load it, and the path to its
// SKILL.md inside the product's own embedded filesystem.
type SkillFileBinding struct {
	Name        string
	Description string
	Path        string
}

// RegisterEmbeddedSkills reads each binding's SKILL.md from fsys, strips its
// frontmatter, and registers it as a builtin skill. Every product that owns
// skills did this same three-step loop -- read, strip frontmatter, register
// -- as its own private copy; this is that loop, written once.
//
// fsys is a product's own //go:embed'd filesystem (embed.FS satisfies fs.FS),
// so this has no dependency on any one product's package layout. Call it from
// inside a sync.Once in the product's own registration entry point -- that
// part stays per-product, since each product's "have I registered yet" is
// independent of every other product's.
func RegisterEmbeddedSkills(fsys fs.FS, bindings []SkillFileBinding) error {
	for _, binding := range bindings {
		data, err := fs.ReadFile(fsys, binding.Path)
		if err != nil {
			return fmt.Errorf("read skill %q: %w", binding.Name, err)
		}
		content := string(data)
		if strings.HasPrefix(content, "---\n") {
			if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
				content = content[end+9:]
			}
		}
		if err := skills.RegisterBuiltin(&llmtypes.Skill{
			Name:        binding.Name,
			Description: binding.Description,
			Content:     content,
			Source:      llmtypes.SkillSource{Origin: "builtin"},
		}); err != nil {
			return fmt.Errorf("register skill %q: %w", binding.Name, err)
		}
	}
	return nil
}

// RegisterSkills registers already-materialized skills directly -- for a
// product whose skill content isn't a static embedded file (e.g. rendered
// from a shared template registry, like
// guidance.MaterializeReferenceKindsAsSkills) but should still be an
// individually-named, product-declared skill the same way
// RegisterEmbeddedSkills' file-backed ones are.
func RegisterSkills(items []*llmtypes.Skill) error {
	for _, item := range items {
		if err := skills.RegisterBuiltin(item); err != nil {
			return fmt.Errorf("register skill %q: %w", item.Name, err)
		}
	}
	return nil
}
