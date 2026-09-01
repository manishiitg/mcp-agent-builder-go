package step_based_workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseStepConfigMigratesLegacyLearningLocks(t *testing.T) {
	content := `{
  "steps": [
    {"id":"locked","agent_configs":{"learnings_access":"read-write","learning_objective":"capture selectors","lock_learnings":true,"lock_learnings_reason":"stable"}},
    {"id":"none","agent_configs":{"learnings_access":"none","lock_learnings":true}},
    {"id":"unlocked","agent_configs":{"learnings_access":"read-write","learning_objective":"capture retries","lock_learnings":false}}
  ]
}`

	configs, err := ParseStepConfigContent(content)
	if err != nil {
		t.Fatalf("ParseStepConfigContent: %v", err)
	}
	if got := configs[0].AgentConfigs.LearningsAccess; got != LearningsAccessRead {
		t.Fatalf("legacy true lock access = %q, want %q", got, LearningsAccessRead)
	}
	if got := configs[1].AgentConfigs.LearningsAccess; got != LearningsAccessNone {
		t.Fatalf("explicit none widened to %q", got)
	}
	if got := configs[2].AgentConfigs.LearningsAccess; got != LearningsAccessReadWrite {
		t.Fatalf("legacy false lock changed access to %q", got)
	}

	encoded, err := json.Marshal(StepConfigFile{Steps: configs})
	if err != nil {
		t.Fatalf("marshal migrated config: %v", err)
	}
	for _, retired := range []string{"lock_learnings", "lock_learnings_reason"} {
		if strings.Contains(string(encoded), retired) {
			t.Fatalf("migrated JSON still contains retired field %q: %s", retired, encoded)
		}
	}
}

func TestLegacyLearningLockClearFieldsAreHonestNoOps(t *testing.T) {
	for _, field := range []string{"lock_learnings", "lock_learnings_reason"} {
		if _, ok := isRetiredStepConfigClearField(field); !ok {
			t.Fatalf("%q is not registered as retired", field)
		}
		if clearStepConfigField(&StepConfig{ID: "step", AgentConfigs: &AgentConfigs{}}, field) {
			t.Fatalf("%q was reported as an active clearable field", field)
		}
	}
}
