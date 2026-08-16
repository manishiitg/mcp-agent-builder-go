package cursorauth

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestValidateAPIKeyRejectsEmpty(t *testing.T) {
	if err := ValidateAPIKey(context.Background(), "   "); err == nil {
		t.Fatal("an empty key must be rejected before any CLI call")
	}
}

// CheckEnv is the guarantee that a candidate key is judged on its own merits:
// an ambient CURSOR_API_KEY would otherwise authenticate the check and pass a
// typo'd key, which is exactly the machine-login fallback this prevents.
func TestCheckEnvStripsOnlyAmbientCursorCredentials(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"HOME=/home/someone",
		"CURSOR_API_KEY=ambient-key",
		"CURSOR_AUTH_TOKEN=ambient-token",
		"CURSOR_UNRELATED=keep-me",
	}
	got := CheckEnv(base)
	joined := strings.Join(got, "\n")
	for _, blocked := range []string{"CURSOR_API_KEY=", "CURSOR_AUTH_TOKEN="} {
		if strings.Contains(joined, blocked) {
			t.Errorf("%s survived CheckEnv, so the check could authenticate with the ambient credential", blocked)
		}
	}
	for _, kept := range []string{"PATH=/usr/bin", "HOME=/home/someone", "CURSOR_UNRELATED=keep-me"} {
		if !strings.Contains(joined, kept) {
			t.Errorf("CheckEnv dropped %q; the CLI's security helper needs the rest of the environment", kept)
		}
	}
}

func TestStripANSILeavesTheWordsMatchable(t *testing.T) {
	warning := "\x1b[33m⚠ Warning: The provided API key is invalid.\x1b[0m"
	got := strings.ToLower(stripANSI(warning))
	if !strings.Contains(got, "api key is invalid") {
		t.Fatalf("color escapes still hide the rejection wording: %q", got)
	}
}

// TestValidateAPIKeyRejectsGarbageLive exercises the real CLI. It is the only
// check that proves the rejection wording still matches; Cursor can reword it.
// Skipped unless the CLI is present.
func TestValidateAPIKeyRejectsGarbageLive(t *testing.T) {
	if os.Getenv("CURSOR_AUTH_LIVE") == "" {
		t.Skip("set CURSOR_AUTH_LIVE=1 to run the live Cursor CLI check")
	}
	err := ValidateAPIKey(context.Background(), "crsr_deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatal("a garbage key must not validate")
	}
	if !strings.Contains(err.Error(), "rejected this API key") {
		t.Fatalf("garbage key produced an unexpected error, so the wording match may have drifted: %v", err)
	}
}
