package videoproduct

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncClaudeProductSkillsMaterializesEmbeddedGuidance(t *testing.T) {
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, "skills", "video-studio")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "SKILL.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncClaudeProductSkills(workspace); err != nil {
		t.Fatalf("syncClaudeProductSkills: %v", err)
	}
	root := filepath.Join(workspace, ".claude", "skills")
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy reference mirror remains: %v", err)
	}
	for _, definition := range profileSkills {
		content, err := os.ReadFile(filepath.Join(root, definition.name, "SKILL.md"))
		if err != nil {
			t.Fatalf("read %s Claude Code skill: %v", definition.name, err)
		}
		if len(content) == 0 {
			t.Fatalf("Claude Code %s skill is empty", definition.name)
		}
	}
	cost, err := os.ReadFile(filepath.Join(root, "video-model-selection", "references", "cost-guidance.md"))
	if err != nil || !strings.Contains(string(cost), "Video cost guidance") {
		t.Fatalf("cost guidance = %q, %v", cost, err)
	}
}

func TestSyncVisibleSkillsForExistingProjectsUpgradesOnlyVideoStudioProjects(t *testing.T) {
	docs := t.TempDir()
	videoProject := filepath.Join(docs, "_users", "user-1", "Chats", "Video Studio", "projects", "demo")
	otherProject := filepath.Join(docs, "_users", "user-1", "Chats", "Other Product", "projects", "ignore")
	for _, path := range []string{videoProject, otherProject} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := SyncVisibleSkillsForExistingProjects(docs); err != nil {
		t.Fatalf("SyncVisibleSkillsForExistingProjects: %v", err)
	}
	if _, err := os.Stat(filepath.Join(videoProject, ".claude", "skills", "seedance-video", "SKILL.md")); err != nil {
		t.Fatalf("Video Studio project not upgraded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(otherProject, "skills", "video-studio")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-Video Studio project was unexpectedly changed: %v", err)
	}
}
