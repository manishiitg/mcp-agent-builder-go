package videoproduct

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncVisibleProductSkillsMaterializesEmbeddedGuidance(t *testing.T) {
	workspace := t.TempDir()
	if err := syncVisibleProductSkills(workspace); err != nil {
		t.Fatalf("syncVisibleProductSkills: %v", err)
	}
	root := filepath.Join(workspace, "skills", "video-studio")
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || !strings.Contains(string(readme), "built into the Video Studio agent") {
		t.Fatalf("README = %q, %v", readme, err)
	}
	for _, definition := range profileSkills {
		content, err := os.ReadFile(filepath.Join(root, definition.name, "SKILL.md"))
		if err != nil {
			t.Fatalf("read %s visible skill: %v", definition.name, err)
		}
		if len(content) == 0 {
			t.Fatalf("visible %s skill is empty", definition.name)
		}
	}
	cost, err := os.ReadFile(filepath.Join(root, "video-model-selection", "references", "cost-guidance.md"))
	if err != nil || !strings.Contains(string(cost), "Video cost guidance") {
		t.Fatalf("cost guidance = %q, %v", cost, err)
	}
}
