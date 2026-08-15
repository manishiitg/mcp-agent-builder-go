package videoproduct

import (
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

const DefaultClaudeModel = "claude-sonnet-5"

var videoExtensions = map[string]bool{".mp4": true, ".mov": true, ".webm": true, ".m4v": true}

type qualityReportCheck struct {
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type qualityReportFrame struct {
	TimestampSeconds float64 `json:"timestamp_seconds"`
	Path             string  `json:"path"`
}

type qualityReport struct {
	SchemaVersion     int                           `json:"schema_version"`
	CandidatePath     string                        `json:"candidate_path"`
	ContactSheetPath  string                        `json:"contact_sheet_path"`
	Verdict           string                        `json:"verdict"`
	ReadyToPresent    bool                          `json:"ready_to_present"`
	Checks            map[string]qualityReportCheck `json:"checks"`
	SampledFrames     []qualityReportFrame          `json:"sampled_frames"`
	RecommendedAction string                        `json:"recommended_action"`
}

//go:embed skills/*/SKILL.md
var profileSkillFiles embed.FS

var profileSkills = []struct{ name, description, path string }{
	{"product-infographic", "Turn verified product evidence into a clear HyperFrames explainer through an adaptive brief, specialist routing, and production QA.", "skills/product-infographic/SKILL.md"},
	{"video-creation", "Direct a conversational video project from brief through reproducible production.", "skills/video-creation/SKILL.md"},
	{"video-editing", "Assemble clips, captions, overlays, narration, music, and versioned exports.", "skills/video-editing/SKILL.md"},
	{"video-quality", "Validate candidate videos technically, visually, and editorially.", "skills/video-quality/SKILL.md"},
	{"hyperframes-quality", "Gate editable HyperFrames compositions and rendered evidence for layout, timing, contrast, and motion quality.", "skills/hyperframes-quality/SKILL.md"},
	{"html-composition", "Design video frames as HTML/CSS and render them with headless Chrome and ffmpeg.", "skills/html-composition/SKILL.md"},
	{"fal-ai", "Generate AI video, image, voice, and music clips via fal.ai's Python client for long-form narrative productions.", "skills/fal-ai/SKILL.md"},
}

var registerProductSkillsOnce sync.Once
var registerProductSkillsErr error

func RegisterProductSkills() error {
	registerProductSkillsOnce.Do(func() {
		for _, definition := range profileSkills {
			data, err := profileSkillFiles.ReadFile(definition.path)
			if err != nil {
				registerProductSkillsErr = err
				return
			}
			content := string(data)
			if strings.HasPrefix(content, "---\n") {
				if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
					content = content[end+9:]
				}
			}
			if err := skills.RegisterBuiltin(&llmtypes.Skill{Name: definition.name, Description: definition.description, Content: content, Source: llmtypes.SkillSource{Origin: "builtin"}}); err != nil {
				registerProductSkillsErr = fmt.Errorf("register skill %q: %w", definition.name, err)
				return
			}
		}
	})
	return registerProductSkillsErr
}

func BuiltinAgentProfile() agentprofiles.Profile {
	manifest := mustVideoStudioManifest()
	profile := manifest.Profile
	profile.SystemPromptTemplate = renderProductPrompt("{{.ProjectTitle}}", "{{.LocalDateTime}}")
	return profile
}

func BuiltinAgentProfiles() []agentprofiles.Profile {
	current := BuiltinAgentProfile()
	legacy := current
	legacy.Version, legacy.Runtime.Provider, legacy.Runtime.ModelID = 1, "", ""
	return []agentprofiles.Profile{legacy, current}
}

func cleanProjectPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("path is missing")
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(raw, "/")))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay inside the project")
	}
	return filepath.ToSlash(clean), nil
}
