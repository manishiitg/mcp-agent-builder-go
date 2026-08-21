package workspacepathpolicy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMaterializeCreatesManagedDirectory(t *testing.T) {
	root := t.TempDir()
	grants := []Grant{{Path: "Workflow/demo/tool_output_folder", Lifecycle: PlatformManaged, Kind: Directory}}
	active, err := Materialize(root, grants)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active grants = %d, want 1", len(active))
	}
	info, err := os.Stat(filepath.Join(root, grants[0].Path))
	if err != nil || !info.IsDir() {
		t.Fatalf("managed directory was not created: info=%v err=%v", info, err)
	}
}

func TestMaterializeLifecycleRules(t *testing.T) {
	root := t.TempDir()
	if _, err := Materialize(root, []Grant{{Path: "missing", Lifecycle: Required, Kind: Directory}}); err == nil {
		t.Fatal("required missing path unexpectedly succeeded")
	}
	active, err := Materialize(root, []Grant{{Path: "missing", Lifecycle: Optional, Kind: Directory}})
	if err != nil || len(active) != 0 {
		t.Fatalf("optional missing path: active=%v err=%v", active, err)
	}
}

func TestMaterializeRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := Materialize(root, []Grant{{Path: "../escape", Lifecycle: PlatformManaged, Kind: Directory}}); err == nil {
		t.Fatal("path traversal unexpectedly succeeded")
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(root, []Grant{{Path: "linked/new", Lifecycle: PlatformManaged, Kind: Directory}}); err == nil {
		t.Fatal("symlink escape unexpectedly succeeded")
	}
}

func TestPlatformManagedPathMustBeDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := Materialize(root, []Grant{{Path: "file", Lifecycle: PlatformManaged, Kind: File}}); err == nil {
		t.Fatal("managed file unexpectedly succeeded")
	}
}
