package server

import "testing"

// TestParseWorkflowAccessLevelReadAliases locks in PLAT-262's restoration:
// these aliases must resolve to the real read tier, not collapse into write
// the way they did while the tier was removed (2026-08-17 to PLAT-262).
func TestParseWorkflowAccessLevelReadAliases(t *testing.T) {
	for _, alias := range []string{"read", "reader", "run", "runner", "view", "viewer", "READ", " Reader "} {
		level, ok := parseWorkflowAccessLevel(alias)
		if !ok {
			t.Fatalf("parseWorkflowAccessLevel(%q): expected ok=true", alias)
		}
		if level != WorkflowAccessRead {
			t.Fatalf("parseWorkflowAccessLevel(%q) = %q, want %q", alias, level, WorkflowAccessRead)
		}
	}
}

func TestParseWorkflowAccessLevelWriteAndOwnerUnaffected(t *testing.T) {
	writeAliases := []string{"write", "writer", "edit", "editor"}
	for _, alias := range writeAliases {
		level, ok := parseWorkflowAccessLevel(alias)
		if !ok || level != WorkflowAccessWrite {
			t.Fatalf("parseWorkflowAccessLevel(%q) = (%q, %v), want (%q, true)", alias, level, ok, WorkflowAccessWrite)
		}
	}
	ownerAliases := []string{"owner", "admin", "manage", "manager"}
	for _, alias := range ownerAliases {
		level, ok := parseWorkflowAccessLevel(alias)
		if !ok || level != WorkflowAccessOwner {
			t.Fatalf("parseWorkflowAccessLevel(%q) = (%q, %v), want (%q, true)", alias, level, ok, WorkflowAccessOwner)
		}
	}
}

func TestParseWorkflowAccessLevelUnknown(t *testing.T) {
	if _, ok := parseWorkflowAccessLevel("bogus"); ok {
		t.Fatal("parseWorkflowAccessLevel(\"bogus\"): expected ok=false")
	}
}

// TestWorkflowPermissionInfoReadTier locks in the read tier's permission
// shape: can chat/run, cannot write or manage access.
func TestWorkflowPermissionInfoReadTier(t *testing.T) {
	info := workflowPermissionInfo(WorkflowAccessRead)
	if !info.CanRunWorkflows {
		t.Fatal("read tier must still be able to run/chat")
	}
	if info.CanWriteWorkflows {
		t.Fatal("read tier must not be able to write workflows")
	}
	if info.CanManageWorkflowAccess {
		t.Fatal("read tier must not be able to manage workflow access")
	}
	if info.WorkflowAccess != WorkflowAccessRead {
		t.Fatalf("WorkflowAccess = %q, want %q", info.WorkflowAccess, WorkflowAccessRead)
	}
}
