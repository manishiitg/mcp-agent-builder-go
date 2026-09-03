package step_based_workflow

import "testing"

// PLAT-072. The contract that matters is not the value but how emptiness is
// read: an unknown revision must never be treated as an old one, or a staleness
// sweep would close live findings — the exact failure the stamp exists to
// prevent.
//
// This binary is normally built inside a Go workspace (a go.work above the
// repo), which silently disables VCS stamping, so without the link-time
// injection PlatformVersion is empty. That is why run_server_with_logging.sh
// passes -ldflags; this test pins that the injected path is the one that
// answers, since a silent regression there would make every future finding
// unstampable without anything failing.
func TestPlatformVersionPrefersInjectedValue(t *testing.T) {
	got := PlatformVersion()
	if injectedPlatformVersion != "" && got != injectedPlatformVersion {
		t.Fatalf("PlatformVersion() = %q, want the injected %q", got, injectedPlatformVersion)
	}
	if injectedPlatformVersion == "" && got != "" {
		// Not a failure: a non-workspace build can legitimately resolve a
		// revision from build info. Recorded so the source is visible when this
		// test is read after a build-mode change.
		t.Logf("no injected value; resolved %q from build info", got)
	}
}

// TestPlatformVersionIsStableAcrossCalls pins the sync.Once behavior. It is
// called on every concern write, so a per-call ReadBuildInfo would be wasteful,
// and a value that changed mid-process would make first_seen_platform_version
// meaningless.
func TestPlatformVersionIsStableAcrossCalls(t *testing.T) {
	if a, b := PlatformVersion(), PlatformVersion(); a != b {
		t.Fatalf("PlatformVersion() is not stable: %q then %q", a, b)
	}
}
