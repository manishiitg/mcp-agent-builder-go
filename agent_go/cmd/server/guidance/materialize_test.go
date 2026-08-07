package guidance

import (
	"os"
	"strings"
	"testing"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func materializedFileContent(t *testing.T, skill *llmtypes.Skill, relPath string) string {
	t.Helper()
	if skill == nil {
		t.Fatal("skill is nil")
	}
	for _, file := range skill.SupportingFiles {
		if file.RelPath == relPath {
			return string(file.Content)
		}
	}
	t.Fatalf("missing supporting file %q; files=%v", relPath, skill.SupportingFiles)
	return ""
}

func TestMaterializedReferenceSkillIncludesConfigToolOnlyDocs(t *testing.T) {
	skill := MaterializeReferenceSkill("workshop")
	if skill == nil {
		t.Fatal("expected builder-reference skill")
	}

	for _, want := range []string{"LLM/provider configuration via tools", "not by reading or editing `config/` files", "references/llm-selection.md", "references/workspace-media-tools.md"} {
		if !strings.Contains(skill.Description+skill.Content, want) {
			t.Fatalf("builder-reference skill should contain %q\nDescription:\n%s\nContent:\n%s", want, skill.Description, skill.Content)
		}
	}

	llmSelection := materializedFileContent(t, skill, "references/llm-selection.md")
	for _, want := range []string{"list_published_llms", "set_provider_auth", "never paste API keys into shell or config files"} {
		if !strings.Contains(llmSelection, want) {
			t.Fatalf("llm-selection reference should contain %q\n%s", want, llmSelection)
		}
	}
	for _, banned := range []string{"config/published-llms.json", "config/provider-api-keys.json"} {
		if strings.Contains(llmSelection, banned) {
			t.Fatalf("llm-selection reference should not expose raw config file %q\n%s", banned, llmSelection)
		}
	}

	mediaTools := materializedFileContent(t, skill, "references/workspace-media-tools.md")
	for _, want := range []string{"set_provider_auth", "workspace-backed image generation defaults", "**Search provider routing** comes from the published LLM set"} {
		if !strings.Contains(mediaTools, want) {
			t.Fatalf("workspace-media-tools reference should contain %q\n%s", want, mediaTools)
		}
	}
	for _, banned := range []string{"config/published-llms.json", "config/provider-api-keys.json", "config/image-generation-config.json", "config/image-analysis-config.json"} {
		if strings.Contains(mediaTools, banned) {
			t.Fatalf("workspace-media-tools reference should not expose raw config file %q\n%s", banned, mediaTools)
		}
	}
}

func TestMaterializedReferenceSkillUsesMultiAgentSurface(t *testing.T) {
	skill := MaterializeReferenceSkill("multi-agent")
	if skill == nil {
		t.Fatal("expected builder-reference skill")
	}
	if skill.Name != "builder-reference" {
		t.Fatalf("skill name = %q, want builder-reference", skill.Name)
	}

	for _, want := range []string{"Multi-agent chat reference docs", "references/llm-provider-config.md", "references/delegation.md"} {
		if !strings.Contains(skill.Description+skill.Content, want) {
			t.Fatalf("builder-reference skill should contain %q\nDescription:\n%s\nContent:\n%s", want, skill.Description, skill.Content)
		}
	}
	if strings.Contains(skill.Content, "references/llm-selection.md") {
		t.Fatalf("builder-reference should not expose workflow-only llm-selection\n%s", skill.Content)
	}

	llmProviderConfig := materializedFileContent(t, skill, "references/llm-provider-config.md")
	for _, want := range []string{"list_published_llms", "list_provider_models", "save_published_llm", "reasoning_effort", "Never inspect or edit `config/` files"} {
		if !strings.Contains(llmProviderConfig, want) {
			t.Fatalf("llm-provider-config reference should contain %q\n%s", want, llmProviderConfig)
		}
	}
	for _, banned := range []string{"context_window", "input_cost_per_1m", "temperature", "tool-call settings"} {
		if strings.Contains(llmProviderConfig, `"`+banned+`"`) {
			t.Fatalf("llm-provider-config should not present %q as a stored JSON field\n%s", banned, llmProviderConfig)
		}
	}
}

func TestSystemToolsSkillTeachesConfigToolOnlyAccess(t *testing.T) {
	skill := BuildSystemToolsSkill("workshop")
	if skill == nil {
		t.Fatal("expected system-tools skill")
	}
	for _, want := range []string{"## Configuration access", "LLM/provider configuration is tool-managed", "Do not read or edit `config/` files", "read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/llm-provider-config.md\"}])", "read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/llm-selection.md\"}])", "read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/workspace-media-tools.md\"}])"} {
		if !strings.Contains(skill.Content, want) {
			t.Fatalf("system-tools skill should contain %q\n%s", want, skill.Content)
		}
	}
}

func TestSystemToolsSkillDoesNotPointMultiAgentAtWorkflowOnlyLLMSelection(t *testing.T) {
	skill := BuildSystemToolsSkill("multi-agent")
	if skill == nil {
		t.Fatal("expected system-tools skill")
	}
	for _, want := range []string{"read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/llm-provider-config.md\"}])", "list_published_llms", "save_published_llm"} {
		if !strings.Contains(skill.Content, want) {
			t.Fatalf("multi-agent system-tools skill should contain %q\n%s", want, skill.Content)
		}
	}
	if strings.Contains(skill.Content, "read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/llm-selection.md\"}])") {
		t.Fatalf("multi-agent system-tools should not mention workflow-only llm-selection\n%s", skill.Content)
	}
}

// Pulse reviewers and Fixers run as stage agents whose prompt names five
// reference docs. That worked while get_reference_doc was a global tool needing
// no attachment. read_skill replaced it and resolves only against attached
// skills, so a stage with none attached gets "available skills: " — an empty
// list — for every one of them. The stage runs in an isolated tmp cwd with no
// MCP servers and no shell built-ins, so a doc it cannot load is a doc it cannot
// reach at all. This pins both halves: the prompt must say how to load, and the
// bundle must actually carry what it names.
func TestPulseReviewFixerDocsAreNamedAndLoadable(t *testing.T) {
	prompt := RenderSystemDoc("pulse-review-fixer")
	if prompt == "" {
		t.Fatalf("pulse-review-fixer rendered empty")
	}
	if !strings.Contains(prompt, `read_skill(skills=[{"name":"builder-reference"`) {
		t.Fatalf("pulse-review-fixer names reference docs but never says how to load them")
	}
	if strings.Contains(prompt, "get_reference_doc") {
		t.Fatalf("pulse-review-fixer still references the removed get_reference_doc tool")
	}
	if !strings.Contains(prompt, "references/pulse-fixer-practices.md") {
		t.Fatal("pulse-review-fixer does not require the canonical Fixer practices reference")
	}

	// The docs are split across bundles — review-artifact-drift is in
	// workflow-commands, the rest in builder-reference — so the whole surface
	// must be attached, not just the reference skill.
	var attached []*llmtypes.Skill
	if err := AttachReferenceSurface("workshop", func(s *llmtypes.Skill) error {
		attached = append(attached, s)
		return nil
	}); err != nil {
		t.Fatalf("attach reference surface: %v", err)
	}
	for _, kind := range []string{
		"fix-verification", "pulse-fixer-practices", "strategy-auditor", "pulse-bug-review",
		"llm-selection", "review-artifact-drift",
	} {
		want := "references/" + kind + ".md"
		found := false
		for _, skill := range attached {
			for _, f := range skill.SupportingFiles {
				if f.RelPath == want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no attached skill carries %s, which pulse-review-fixer tells the agent to read", want)
		}
	}
}

func TestEngineeringReviewUsesTheCanonicalReviewAndFixSequence(t *testing.T) {
	raw, err := os.ReadFile("templates/improve/engineering-review.md")
	if err != nil {
		t.Fatalf("read engineering-review template: %v", err)
	}
	prompt := string(raw)
	for _, want := range []string{
		"continuing Workflow Builder conversation",
		"pulse_run_id=\"current\"",
		"Own the review yourself",
		"Persist typed findings and verification",
		"normal Workflow Builder tools",
		"one terminal module result for Engineering and one for Operations",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("engineering-review prompt is missing canonical sequence contract %q", want)
		}
	}
	for _, forbidden := range []string{"STANDALONE PULSE FIXER", `role="fixer"`, `review_lanes=[`} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("engineering-review retained obsolete standalone Fixer contract %q", forbidden)
		}
	}
}

func TestPulseFixerPracticesRequireExhaustiveAgenticDrain(t *testing.T) {
	raw, err := os.ReadFile("templates/system/pulse-fixer-practices.md")
	if err != nil {
		t.Fatalf("read pulse-fixer-practices template: %v", err)
	}
	practices := string(raw)
	for _, want := range []string{
		"Full-backlog drain contract",
		"Freeze a starting manifest",
		"Classify every manifest item",
		"Maintain an explicit remaining list",
		"Reconcile before completion",
		`record_pulse_result`,
	} {
		if !strings.Contains(practices, want) {
			t.Errorf("pulse-fixer practices missing exhaustive-drain rule %q", want)
		}
	}

	scheduled := RenderSystemDoc("pulse-review-fixer")
	for _, want := range []string{
		"complete active starting manifest",
		"do not narrow the consolidated Fixer's retained",
		"Full-backlog drain contract",
		"must not claim completion",
	} {
		if !strings.Contains(scheduled, want) {
			t.Errorf("scheduled Fixer prompt missing exhaustive-drain rule %q", want)
		}
	}
}
