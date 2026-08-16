package browser

import "testing"

func TestShouldRunGlobalStartupCleanup(t *testing.T) {
	t.Run("normal instance", func(t *testing.T) {
		t.Setenv("AGENTWORKS_SKIP_GLOBAL_BROWSER_CLEANUP", "")
		if !ShouldRunGlobalStartupCleanup() {
			t.Fatal("normal singleton instance should retain startup cleanup")
		}
	})

	t.Run("isolated instance", func(t *testing.T) {
		t.Setenv("AGENTWORKS_SKIP_GLOBAL_BROWSER_CLEANUP", "true")
		if ShouldRunGlobalStartupCleanup() {
			t.Fatal("isolated instance must not run workspace-wide browser cleanup")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Setenv("AGENTWORKS_SKIP_GLOBAL_BROWSER_CLEANUP", " TRUE ")
		if ShouldRunGlobalStartupCleanup() {
			t.Fatal("trimmed case-insensitive true should disable cleanup")
		}
	})
}
