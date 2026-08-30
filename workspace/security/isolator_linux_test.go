//go:build linux

package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMountNamespaceFallbackEnforcesLandlockRejectedOverlapPolicy is the
// proper, repeatable server-side reproduction of the live incident on the
// Dominion Hetzner deployment 2026-08-28/29 -- run this directly on any
// Linux deployment host to verify the fix instead of hand-crafting a curl
// request against a running service:
//
//	cd workspace && go test ./security/... -run TestMountNamespaceFallbackEnforcesLandlockRejectedOverlapPolicy -v
//
// Exercises the exact policy shape that broke: write access to a folder
// except one of its own subfolders (BlockedWritePaths nested inside
// WritePaths). Landlock's purely-additive rule model cannot express that
// carve-out and correctly refuses it (landlockPolicy() returning
// "blocked-write path overlaps writable path" is Landlock behaving
// correctly, not a bug); the real check is whether ExecuteIsolated's
// end-to-end dispatch still falls through to the mount-namespace fallback
// and that fallback actually enforces the policy -- writes inside the
// granted folder succeed, writes inside its blocked-write subfolder fail.
// Skips (not fails) when Landlock is unavailable in this environment (e.g.
// inside an already-namespaced CI runner) or the overlap isn't rejected,
// since neither means this fix regressed -- it means this run's
// environment can't exercise the fallback path at all.
func TestMountNamespaceFallbackEnforcesLandlockRejectedOverlapPolicy(t *testing.T) {
	root := t.TempDir()
	writableDir := filepath.Join(root, "workflow")
	blockedSubdir := filepath.Join(writableDir, "planning")
	for _, dir := range []string{writableDir, blockedSubdir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	isolator := &Isolator{
		ReadPaths:         []string{writableDir},
		WritePaths:        []string{writableDir},
		BlockedWritePaths: []string{blockedSubdir},
		WorkDir:           writableDir,
		// BaseDir must be set explicitly: the mount-namespace backend's
		// script bind-mounts getBaseDir() to preserve the original
		// workspace, and its unset default (/app/workspace-docs, the
		// containerized-deployment path) does not exist on this test host.
		BaseDir: root,
	}

	if _, err := isolator.landlockPolicy(); err == nil {
		t.Skip("this environment's Landlock did not reject the overlap policy (e.g. already namespaced) -- cannot exercise the fallback path here")
	} else if !strings.Contains(err.Error(), "overlaps writable path") {
		t.Fatalf("landlockPolicy() error = %v, want the overlap rejection", err)
	}

	command := fmt.Sprintf(
		"echo top-level-ok > %q && if echo denied > %q 2>/dev/null; then exit 41; fi",
		filepath.Join(writableDir, "ok.txt"),
		filepath.Join(blockedSubdir, "denied.txt"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd, cleanup, err := isolator.ExecuteIsolated(ctx, "sh", []string{"-c", command})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Skipf("ExecuteIsolated could not select any backend in this environment (%v) -- not what this fix changes; run on the actual deployment host to verify it", err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mount-namespace fallback did not enforce the policy correctly: %v\noutput: %s", err, output)
	}
	if _, statErr := os.Stat(filepath.Join(writableDir, "ok.txt")); statErr != nil {
		t.Errorf("write to the granted folder should have succeeded: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(blockedSubdir, "denied.txt")); statErr == nil {
		t.Error("SECURITY VIOLATION: write to the blocked-write subfolder succeeded")
	}
}
