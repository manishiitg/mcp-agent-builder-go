package server

import "testing"

func TestHasRetiredLearningLockField(t *testing.T) {
	for _, config := range []map[string]interface{}{
		{"lock_learnings": true},
		{"lock_learnings": false},
		{"lock_learnings_reason": "stable"},
	} {
		if !hasRetiredLearningLockField(config) {
			t.Fatalf("retired learning-lock config was accepted: %#v", config)
		}
	}
	for _, config := range []map[string]interface{}{
		nil,
		{"learnings_access": "read"},
		{"learning_objective": "capture retries"},
	} {
		if hasRetiredLearningLockField(config) {
			t.Fatalf("active learning config was rejected: %#v", config)
		}
	}
}
