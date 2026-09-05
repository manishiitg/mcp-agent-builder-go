package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envValue(t *testing.T, env []string, key string) (string, bool) {
	t.Helper()
	value, found := "", false
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			if found {
				t.Fatalf("%s appears more than once in %v", key, env)
			}
			value, found = strings.TrimPrefix(kv, key+"="), true
		}
	}
	return value, found
}

func mustEnv(t *testing.T, env []string, key string) string {
	t.Helper()
	value, found := envValue(t, env, key)
	if !found {
		t.Fatalf("missing %s in %v", key, env)
	}
	return value
}

func mustDir(t *testing.T, path string) {
	t.Helper()
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("%q was not pre-created as a directory: %v", path, err)
	}
}

// PLAT-284: with the workflow's .sandbox-cache granted, installs persist across
// runs -- pip's user site, npm's global prefix and the caches all live there,
// their bin/ folders lead PATH, and only TMPDIR stays in the run folder.
func TestSandboxToolEnvUsesThePersistentDirWhenGranted(t *testing.T) {
	workflow := t.TempDir()
	persistent := filepath.Join(workflow, SandboxPersistentDirName)
	runDir := filepath.Join(workflow, "runs", "iteration-0", "default", "execution")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(persistent, 0755); err != nil {
		t.Fatal(err)
	}

	env := sandboxToolEnv([]string{"HOME=/nowhere", "PATH=/usr/bin:/bin"}, runDir, []string{runDir, persistent})

	if got := mustEnv(t, env, SandboxPersistentDirEnv); got != persistent {
		t.Errorf("%s = %q, want %q", SandboxPersistentDirEnv, got, persistent)
	}
	want := map[string]string{
		"PYTHONUSERBASE":    filepath.Join(persistent, "python"),
		"PIP_CACHE_DIR":     filepath.Join(persistent, "pip"),
		"XDG_CACHE_HOME":    filepath.Join(persistent, "xdg"),
		"npm_config_cache":  filepath.Join(persistent, "npm"),
		"npm_config_prefix": filepath.Join(persistent, "npm-global"),
		"GOPATH":            filepath.Join(persistent, "go"),
		"CARGO_HOME":        filepath.Join(persistent, "cargo"),
		"PIPX_BIN_DIR":      filepath.Join(persistent, "bin"),
		"TMPDIR":            filepath.Join(runDir, ".tmp"),
	}
	for key, wantValue := range want {
		if got := mustEnv(t, env, key); got != wantValue {
			t.Errorf("%s = %q, want %q", key, got, wantValue)
		}
		mustDir(t, wantValue)
	}
	path := mustEnv(t, env, "PATH")
	wantPrefix := strings.Join([]string{
		filepath.Join(persistent, "bin"),
		filepath.Join(persistent, "python", "bin"),
		filepath.Join(persistent, "npm-global", "bin"),
	}, string(os.PathListSeparator))
	if !strings.HasPrefix(path, wantPrefix) || !strings.HasSuffix(path, "/usr/bin:/bin") {
		t.Errorf("PATH = %q: want the persistent bin folders first and the original PATH kept", path)
	}
	if got := mustEnv(t, env, "HOME"); got != "/nowhere" {
		t.Errorf("unrelated entries must pass through untouched, HOME = %q", got)
	}
}

// Without the persistent grant (older orchestrator, non-workflow caller) the
// PLAT-283 behaviour stands: caches routed into the run folder so an install
// works for that run, PATH untouched, no SANDBOX_PERSISTENT_DIR advertised.
func TestSandboxToolEnvFallsBackToTheRunFolderWithoutAPersistentDir(t *testing.T) {
	runDir := t.TempDir()

	env := sandboxToolEnv([]string{"PATH=/usr/bin"}, runDir, []string{runDir})

	want := map[string]string{
		"PIP_CACHE_DIR":    filepath.Join(runDir, ".cache", "pip"),
		"XDG_CACHE_HOME":   filepath.Join(runDir, ".cache"),
		"PYTHONUSERBASE":   filepath.Join(runDir, ".local"),
		"npm_config_cache": filepath.Join(runDir, ".cache", "npm"),
		"TMPDIR":           filepath.Join(runDir, ".tmp"),
	}
	for key, wantValue := range want {
		if got := mustEnv(t, env, key); got != wantValue {
			t.Errorf("%s = %q, want %q", key, got, wantValue)
		}
		mustDir(t, wantValue)
	}
	if _, found := envValue(t, env, SandboxPersistentDirEnv); found {
		t.Error("no persistent dir was granted, so none must be advertised")
	}
	if got := mustEnv(t, env, "PATH"); got != "/usr/bin" {
		t.Errorf("PATH must be untouched in fallback mode, got %q", got)
	}
}

// A step whose WorkDir sits outside its own write grant (a read-only scripted
// step) must still be routed somewhere the sandbox actually allows.
func TestSandboxToolEnvFallsBackWhenWorkDirIsNotWritable(t *testing.T) {
	readOnlyWorkDir := t.TempDir()
	writable := t.TempDir()

	env := sandboxToolEnv(nil, readOnlyWorkDir, []string{writable})

	if got, want := mustEnv(t, env, "PIP_CACHE_DIR"), filepath.Join(writable, ".cache", "pip"); got != want {
		t.Errorf("PIP_CACHE_DIR = %q, want %q (the granted write path, not the unwritable WorkDir)", got, want)
	}
}

// No write grant at all: leave the environment alone rather than point pip at
// a folder the sandbox will deny too.
func TestSandboxToolEnvLeavesEnvUntouchedWithoutAnyWritePath(t *testing.T) {
	in := []string{"PATH=/usr/bin", "HOME=/x"}
	out := sandboxToolEnv(in, t.TempDir(), nil)
	if strings.Join(out, "\x00") != strings.Join(in, "\x00") {
		t.Fatalf("expected env untouched, got %v", out)
	}
}

// The persistent grant is recognised by folder name alone, whichever backend
// resolved the paths -- and a persistent dir with no run folder still gets a
// TMPDIR of its own.
func TestSandboxToolEnvRecognisesThePersistentDirWithoutARunFolder(t *testing.T) {
	persistent := filepath.Join(t.TempDir(), SandboxPersistentDirName)
	if err := os.MkdirAll(persistent, 0755); err != nil {
		t.Fatal(err)
	}

	env := sandboxToolEnv(nil, "", []string{persistent})

	if got := mustEnv(t, env, SandboxPersistentDirEnv); got != persistent {
		t.Errorf("%s = %q, want %q", SandboxPersistentDirEnv, got, persistent)
	}
	if got, want := mustEnv(t, env, "TMPDIR"), filepath.Join(persistent, "tmp"); got != want {
		t.Errorf("TMPDIR = %q, want %q", got, want)
	}
	if got := mustEnv(t, env, "PATH"); !strings.HasPrefix(got, filepath.Join(persistent, "bin")) {
		t.Errorf("PATH must be created when the input had none, got %q", got)
	}
}

// The three sandbox backends must all route the same way; the macOS/mount
// backends go through Isolator.toolEnv, which canonicalises iso.WritePaths.
func TestIsolatorToolEnvRoutesThroughTheGrantedPersistentDir(t *testing.T) {
	workflow := t.TempDir()
	persistent := filepath.Join(workflow, SandboxPersistentDirName)
	if err := os.MkdirAll(persistent, 0755); err != nil {
		t.Fatal(err)
	}
	iso := &Isolator{BaseDir: workflow, WorkDir: workflow, WritePaths: []string{workflow, SandboxPersistentDirName}}

	env := iso.toolEnv([]string{"PATH=/bin"})

	if got := mustEnv(t, env, SandboxPersistentDirEnv); got != canonicalPath(persistent) {
		t.Errorf("%s = %q, want %q", SandboxPersistentDirEnv, got, canonicalPath(persistent))
	}
}
