// Package cursorauth validates Cursor Agent CLI API keys.
//
// A Cursor API key scopes a session to one person's own Cursor subscription.
// It is the non-interactive counterpart to `cursor-agent login`, and the same
// reason claudeauth exists applies here: there are two callers — AgentWorks
// workflows and Video Studio projects — and a weakened check in either one
// silently falls back to whichever Cursor login happens to be on the machine,
// billing that account instead.
//
// Cursor has no CLI equivalent of `claude setup-token`; keys are minted in the
// cursor.com dashboard and pasted in, so validation is the only place a typo or
// a revoked key can be caught before it becomes a failed turn.
package cursorauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ValidateAPIKey reports whether Cursor accepts key.
//
// It uses `cursor-agent --api-key <key> --list-models`, which costs no model
// tokens and — verified 2026-08-13 — authenticates against the passed key
// alone: with no key it fails "Authentication required" even while
// `cursor-agent status` reports a logged-in account. That property is the
// whole point. A check that consulted the ambient login would accept any
// string on a machine where someone happens to be signed in, which is the
// billing fallback this prevents.
//
// The CLI exits 0 even for a rejected key, so the outcome is read from its
// output rather than its exit status.
func ValidateAPIKey(parent context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("Cursor API key is required")
	}

	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cursor-agent", "--api-key", key, "--list-models")
	cmd.Env = CheckEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return fmt.Errorf("Cursor Agent CLI is not installed")
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("Cursor timed out while checking this API key")
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("Cursor timed out while checking this API key")
	}

	text := string(out)
	lower := strings.ToLower(stripANSI(text))
	switch {
	case strings.Contains(lower, "api key is invalid"):
		return fmt.Errorf("Cursor rejected this API key")
	case strings.Contains(lower, "authentication required"):
		// Reached only if the key were dropped before the CLI saw it; treating
		// it as success would authorize an unauthenticated session.
		return fmt.Errorf("Cursor did not receive this API key")
	case strings.Contains(lower, "available models"):
		return nil
	}
	if detail := firstLine(stripANSI(text)); detail != "" {
		return fmt.Errorf("Cursor could not verify this API key: %s", detail)
	}
	return fmt.Errorf("Cursor could not verify this API key")
}

// CheckEnv strips every ambient Cursor credential from base so the candidate
// key is the only one the CLI can authenticate with. The rest of the
// environment is kept deliberately: the CLI shells out to a platform security
// helper that needs HOME and PATH, and fails "Security process exited with
// code: 154" without them.
func CheckEnv(base []string) []string {
	blocked := map[string]bool{
		"CURSOR_API_KEY":    true,
		"CURSOR_AUTH_TOKEN": true,
	}
	out := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if blocked[key] {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// stripANSI removes the color escapes the CLI wraps its warnings in, so the
// matching above works on the words rather than the styling.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
