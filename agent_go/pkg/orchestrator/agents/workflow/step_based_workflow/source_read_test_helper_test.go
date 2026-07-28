package step_based_workflow

import (
	"os"
	"testing"
)

// readSourceFile lets a test assert on a construction that cannot be exercised
// without a live workspace service — here, the exact headers on an outbound
// request and the wording handed to a step agent on failure.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
