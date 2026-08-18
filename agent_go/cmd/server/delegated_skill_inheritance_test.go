package server

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func TestDelegatedParentSkillContextPreservesFullBundlesAcrossBackgroundBoundary(t *testing.T) {
	inherited := []*llmtypes.Skill{{
		Name:    "chat-specialist",
		Content: "skill body",
		SupportingFiles: []llmtypes.SkillFile{{
			RelPath: "references/brief.md",
			Content: []byte("brief body"),
		}},
	}}

	parentCtx := withDelegatedParentSkillDefinitions(context.Background(), inherited)
	backgroundCtx := copyDelegatedParentSkills(parentCtx, context.Background())
	got := delegatedParentSkillsFromContext(backgroundCtx)
	if len(got) != 1 || got[0] != inherited[0] {
		t.Fatalf("delegated skill snapshot = %#v, want original immutable definition", got)
	}
	if len(got[0].SupportingFiles) != 1 || string(got[0].SupportingFiles[0].Content) != "brief body" {
		t.Fatalf("supporting files were not preserved: %#v", got[0].SupportingFiles)
	}

	got[0] = nil
	if again := delegatedParentSkillsFromContext(backgroundCtx); len(again) != 1 || again[0] == nil {
		t.Fatal("reading delegated skills should return an independent slice")
	}
}

func TestUniqueDelegatedSkillsDeduplicatesInheritedAndExplicitBundles(t *testing.T) {
	inherited := &llmtypes.Skill{Name: "builder-reference"}
	extra := &llmtypes.Skill{Name: "pdf-extract"}
	seen := map[string]struct{}{"system-tools": {}}
	got := uniqueDelegatedSkills(seen, []*llmtypes.Skill{
		nil,
		{Name: " "},
		inherited,
		{Name: "builder-reference"},
		extra,
		extra,
	})
	if len(got) != 2 || got[0] != inherited || got[1] != extra {
		t.Fatalf("unique delegated skills = %#v, want inherited then one explicit skill", got)
	}
}

func TestAgentWorksDirectChatDoesNotAttachDelegationContext(t *testing.T) {
	assertSourceContains := func(filename string, wants ...string) {
		t.Helper()
		source, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		text := string(source)
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s is missing %q", filename, want)
			}
		}
	}

	assertSourceContains("server.go", "GetAgentWorksChatInstructionsWithUser(")

	serverSource, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	for _, forbidden := range []string{
		"subCtx = withDelegatedParentSkills(subCtx, llmAgent.GetUnderlyingAgent())",
		"bgCtx = withDelegatedParentSkills(bgCtx, llmAgent.GetUnderlyingAgent())",
		"virtualtools.CreateDelegationTools(",
	} {
		if strings.Contains(string(serverSource), forbidden) {
			t.Errorf("direct chat still contains delegation wiring %q", forbidden)
		}
	}
}
