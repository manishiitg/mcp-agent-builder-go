package step_based_workflow

import (
	"encoding/json"
	"testing"
)

func TestLearningArtifactChangeIgnoresAgentClaims(t *testing.T) {
	tests := []struct {
		name     string
		before   string
		after    string
		expected bool
	}{
		{
			name:     "identical tree",
			before:   "sha256:same",
			after:    "sha256:same",
			expected: false,
		},
		{
			name:     "changed tree",
			before:   "sha256:old",
			after:    "sha256:new",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _, confidence := learningArtifactChange(tc.before, tc.after)
			if got != tc.expected {
				t.Fatalf("learningArtifactChange(%q, %q) = %v, want %v", tc.before, tc.after, got, tc.expected)
			}
			if confidence != 1 {
				t.Fatalf("confidence = %v, want deterministic 1", confidence)
			}
		})
	}
}

func TestRefreshLearningMetadataIdentityRepairsCapturedStaleStepPath(t *testing.T) {
	// Captured from build-in-public/learnings/step-route-selector: both this
	// step and step-reddit-scan-draft claimed the creation-time path step-1.
	var metadata LearningMetadata
	if err := json.Unmarshal([]byte(`{"step_id":"step-route-selector","step_path":"step-1","total_iterations":0}`), &metadata); err != nil {
		t.Fatal(err)
	}
	refreshLearningMetadataIdentity(&metadata, "step-route-selector", "step-5")
	if metadata.StepID != "step-route-selector" || metadata.StepPath != "step-5" {
		t.Fatalf("identity = %s/%s, want step-route-selector/step-5", metadata.StepID, metadata.StepPath)
	}
}
