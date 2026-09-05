package step_based_workflow

import (
	"strings"
	"testing"
)

func TestWorkshopFastPathModeGate(t *testing.T) {
	for _, tc := range []struct {
		mode               string
		requested, allowed bool
	}{
		{"workshop", true, true}, {"run", true, false}, {"", true, false},
		{"workshop", false, true}, {"run", false, true},
	} {
		if err := validateWorkshopFastPathRequest(tc.mode, tc.requested); (err == nil) != tc.allowed {
			t.Fatalf("mode=%q requested=%v err=%v", tc.mode, tc.requested, err)
		}
	}
	src := readSourceFile(t, "interactive_workshop_manager.go")
	if !strings.Contains(src, "validateWorkshopFastPathRequest(iwm.currentWorkshopModeFromConfigs(nil), fastPathOnly)") {
		t.Fatal("execute_step does not use the validated mode gate")
	}
}
