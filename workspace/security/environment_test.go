package security

import (
	"strings"
	"testing"
)

func TestNativeEnvironmentRepairsPath(t *testing.T) {
	t.Setenv("NATIVE_WORKSPACE", "true")
	t.Setenv("HOME", "/tmp/native-home")
	t.Setenv("PATH", "/custom/bin")

	env := BuildSafeEnvironment()
	pathValue := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			pathValue = strings.TrimPrefix(kv, "PATH=")
			break
		}
	}
	if pathValue == "" {
		t.Fatalf("expected PATH in native environment")
	}

	required := []string{
		"/custom/bin",
		"/tmp/native-home/.local/bin",
		"/tmp/native-home/go/bin",
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
	}
	for _, path := range required {
		if !pathInList(pathValue, path) {
			t.Fatalf("expected PATH to contain %q, got %q", path, pathValue)
		}
	}
}

func TestNativeEnvironmentDoesNotExposeWorkspaceExecutionToken(t *testing.T) {
	t.Setenv("NATIVE_WORKSPACE", "true")
	t.Setenv("WORKSPACE_API_TOKEN", "server-only-token")
	for _, entry := range BuildSafeEnvironment() {
		if strings.HasPrefix(entry, "WORKSPACE_API_TOKEN=") {
			t.Fatal("workspace execution token leaked into shell environment")
		}
	}
}

func TestDockerEnvironmentUsesConfiguredBrowserExecutable(t *testing.T) {
	t.Setenv("NATIVE_WORKSPACE", "")
	t.Setenv("AGENT_BROWSER_EXECUTABLE_PATH", "/usr/bin/google-chrome")

	foundAgentBrowser := false
	foundHyperFramesBrowser := false
	for _, entry := range BuildSafeEnvironment() {
		if entry == "AGENT_BROWSER_EXECUTABLE_PATH=/usr/bin/google-chrome" {
			foundAgentBrowser = true
		}
		if entry == "HYPERFRAMES_BROWSER_PATH=/usr/bin/google-chrome" {
			foundHyperFramesBrowser = true
		}
	}
	if !foundAgentBrowser || !foundHyperFramesBrowser {
		t.Fatalf("configured browser executable was not preserved for both runtimes: agent-browser=%v hyperframes=%v", foundAgentBrowser, foundHyperFramesBrowser)
	}
}

func pathInList(pathValue, target string) bool {
	for _, path := range strings.Split(pathValue, ":") {
		if path == target {
			return true
		}
	}
	return false
}
