//go:build linux

package security

import (
	"os"
	"path/filepath"
	"testing"
)

// A deployment-installed CLI tool outside the standard system dirs (e.g.
// Dominion's /srv/dominion/tools/bin, holding the alpaca CLI) needs an
// explicit grant or every exec of it from a landlocked step fails with
// "Permission denied" (rc=126) -- not a missing-file error, an exec-rights
// one. Confirmed live: PLACE_PAPER_TRADES on Dominion hit exactly this on
// every run since the alpaca CLI was installed there.
func TestLandlockSystemReadPathsIncludesExtraDeploymentToolPaths(t *testing.T) {
	toolsDir := t.TempDir()
	binDir := filepath.Join(toolsDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SANDBOX_EXTRA_SYSTEM_PATHS", binDir)

	paths := landlockSystemReadPaths()

	found := false
	for _, p := range paths {
		if p == binDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %q (from SANDBOX_EXTRA_SYSTEM_PATHS) in %v", binDir, paths)
	}
}

func TestLandlockSystemReadPathsAcceptsMultipleColonSeparatedExtraPaths(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	t.Setenv("SANDBOX_EXTRA_SYSTEM_PATHS", dirA+":"+dirB)

	paths := landlockSystemReadPaths()

	for _, want := range []string{dirA, dirB} {
		found := false
		for _, p := range paths {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in %v", want, paths)
		}
	}
}

// A path that doesn't exist on disk is silently dropped by
// existingCanonicalPaths -- same behaviour as every other baseline entry --
// rather than failing the whole ruleset.
func TestLandlockSystemReadPathsDropsAMissingExtraPath(t *testing.T) {
	t.Setenv("SANDBOX_EXTRA_SYSTEM_PATHS", "/does/not/exist/at/all")
	paths := landlockSystemReadPaths()
	for _, p := range paths {
		if p == "/does/not/exist/at/all" {
			t.Fatalf("a missing path must not appear in the resolved list: %v", paths)
		}
	}
}

func TestLandlockSystemReadPathsUnsetLeavesBaselineUnchanged(t *testing.T) {
	t.Setenv("SANDBOX_EXTRA_SYSTEM_PATHS", "")
	withoutExtra := landlockSystemReadPaths()
	if len(withoutExtra) == 0 {
		t.Fatal("expected the standard system baseline even with no extra paths configured")
	}
}
