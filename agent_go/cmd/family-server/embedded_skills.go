package main

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

var (
	embeddedSkillsOnce sync.Once
	embeddedSkillList  []*llmtypes.Skill
)

// embeddedSkills parses the app's embedded skills (skills/<name>/SKILL.md)
// into the runtime's own skill type, so a session gets them through
// agentsession.Config.Skills: an "Available Skills" listing in the prompt,
// read_skill for the body on demand, and native SKILL.md projection for
// coding-CLI transports — instead of relying only on the prompt telling the
// model to `cat skills/<name>/SKILL.md`. seedSkills still writes the files to
// disk for that path and for skills/_shared/*.md, which is reference
// material several skills inline and not a skill itself (its name would be
// rejected by the skill-id rules anyway), so it is skipped here.
func embeddedSkills() []*llmtypes.Skill {
	embeddedSkillsOnce.Do(func() {
		entries, err := fs.ReadDir(seededSkillsFS, "skills")
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			raw, err := seededSkillsFS.ReadFile(path.Join("skills", e.Name(), "SKILL.md"))
			if err != nil {
				continue
			}
			front, body, err := skills.ParseSkillFile(string(raw))
			if err != nil || front == nil {
				continue
			}
			name := strings.TrimSpace(front.Name)
			if name == "" {
				name = e.Name()
			}
			embeddedSkillList = append(embeddedSkillList, &llmtypes.Skill{
				Name:        name,
				Description: strings.TrimSpace(front.Description),
				Content:     body,
				Source:      llmtypes.SkillSource{Origin: "family-server"},
			})
		}
		sort.Slice(embeddedSkillList, func(i, j int) bool { return embeddedSkillList[i].Name < embeddedSkillList[j].Name })
	})
	return embeddedSkillList
}
