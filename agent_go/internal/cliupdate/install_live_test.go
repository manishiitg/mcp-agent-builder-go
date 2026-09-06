package cliupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Explicit network opt-in. Downloads only into a disposable directory; does not
// activate shims, touch authentication, start model turns, or change PATH CLIs.
func TestLivePrivateReleaseInstall(t *testing.T) {
	selected := os.Getenv("AGENTWORKS_TEST_CLI_INSTALL")
	if selected == "" {
		t.Skip("set AGENTWORKS_TEST_CLI_INSTALL=all or an executable name for private installer acceptance")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range providers {
		if selected != "all" && selected != p.Name {
			continue
		}
		t.Run(p.Name, func(t *testing.T) {
			binary, version, err := installRelease(context.Background(), p, root, Result{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(binary, root+string(os.PathSeparator)) {
				t.Fatal("escaped private root")
			}
			t.Logf("verified private %s %s", p.Name, version)
		})
	}
}
