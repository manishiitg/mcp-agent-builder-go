package main

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/browser"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

// agentBrowserTool REUSES AgentWorks' pkg/browser.Executor — the entire CDP
// tab manager (cdp_tabs.go/cdp_registry.go), artifact broker, live status and
// error rewriting — instead of reimplementing any of it. The Executor talks to
// a workspace-api server; family-server runs a tiny loopback one (see
// browser_backend.go) backed by its own security.Isolator. All this file does
// is: build that Executor once, inject family-server's FolderGuard/session
// into the context on each call, and expose it as an agentsession.Tool.
//
// CDP-only, port 9222 (WithCdpPort below): the Executor connects only to the
// parent's dedicated "Chrome CDP" browser (set up via the Browser connector),
// never a headless fallback. Parent-only tool.
const browserCDPPort = 9222

var (
	browserExecutor     *browser.Executor
	browserExecutorErr  error
	browserExecutorOnce sync.Once
)

func getBrowserExecutor() (*browser.Executor, error) {
	browserExecutorOnce.Do(func() {
		url, err := startBrowserBackend()
		if err != nil {
			browserExecutorErr = err
			return
		}
		client := browser.NewClient(url)
		browserExecutor = browser.NewExecutor(client, browser.WithCdpPort(browserCDPPort))
	})
	return browserExecutor, browserExecutorErr
}

// browserFolderGuardContext injects the same sandbox intent execute_shell_command
// uses for the parent (the whole workspace root writable, plus the host
// Downloads folder readable) so the reused Executor + loopback backend
// sandbox agent-browser identically and can stage downloads/screenshots into
// the workspace. agent_browser is a parent-only tool (see chat.go).
func browserFolderGuardContext(ctx context.Context) context.Context {
	writePaths := []string{"."}
	readPaths := append([]string{"skills"}, writePaths...)
	if dl := hostDownloadsPath(); dl != "" {
		readPaths = append(readPaths, dl)
	}
	ctx = context.WithValue(ctx, common.FolderGuardWritePathsKey, writePaths)
	ctx = context.WithValue(ctx, common.FolderGuardReadPathsKey, readPaths)
	// Stable session id for this single-family browser; in CDP mode the Executor
	// remaps it to a shared per-port name anyway, so it just needs to be non-empty.
	ctx = context.WithValue(ctx, common.ChatSessionIDKey, "family-browser")
	ctx = context.WithValue(ctx, common.WorkflowSessionIDKey, "family-browser")
	return ctx
}

// rawStringArgs mirrors pkg/browser's own (unexported) stringArgs: the
// "args" field arrives as []interface{} after JSON decoding, occasionally as
// []string when built in-process. Needed here, separately, only because that
// helper isn't exported across the package boundary.
func rawStringArgs(raw interface{}) []string {
	switch v := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, a := range v {
			if s, ok := a.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}

// browserAllowedFileRoots are the only directories a file:// URL may resolve
// into: the family workspace and the host Downloads folder — the same two the
// browser tool is already granted through browserFolderGuardContext.
func browserAllowedFileRoots() []string {
	roots := []string{workspaceRoot()}
	if dl := hostDownloadsPath(); dl != "" {
		roots = append(roots, dl)
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if abs, err := filepath.Abs(strings.TrimSpace(r)); err == nil && abs != "" {
			if resolved, err := filepath.EvalSymlinks(abs); err == nil {
				abs = resolved
			}
			out = append(out, filepath.Clean(abs))
		}
	}
	return out
}

// validateBrowserFileURLs rejects a file:// URL pointing outside those roots.
//
// This closes a real, verified bypass: the CDP browser is the PARENT'S OWN
// Chrome — a pre-existing host process family-server does not own — so it reads
// with the user's full privileges and no sandbox we apply to our own child
// processes can reach it. Confirmed live: execute_shell_command was denied
// ~/sparkquill-decoy-test.txt ("Operation not permitted") while agent_browser
// read the same file through file:// and returned its contents. Reads inside
// the workspace/Downloads stay allowed, since previewing a generated page or a
// downloaded PDF is a legitimate use.
//
// Checked AFTER secret substitution, so what is validated is exactly what
// reaches the browser.
func validateBrowserFileURLs(args []string) error {
	roots := browserAllowedFileRoots()
	for _, arg := range args {
		lower := strings.ToLower(arg)
		idx := strings.Index(lower, "file:")
		if idx < 0 {
			continue
		}
		// Covers "file:..." and wrappers such as "view-source:file:...".
		raw := strings.TrimSpace(arg[idx:])
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("refusing malformed file URL %q", raw)
		}
		// A host other than localhost would be a remote file share, not a
		// local path — reject rather than guess at its meaning.
		if h := strings.ToLower(u.Host); h != "" && h != "localhost" {
			return fmt.Errorf("refusing non-local file URL %q", raw)
		}
		decoded, err := url.PathUnescape(u.Path)
		if err != nil {
			decoded = u.Path
		}
		if strings.TrimSpace(decoded) == "" {
			return fmt.Errorf("refusing file URL with no path: %q", raw)
		}
		abs, err := filepath.Abs(decoded)
		if err != nil {
			return fmt.Errorf("refusing unresolvable file URL %q", raw)
		}
		// Resolve symlinks so a link inside the workspace cannot point out of
		// it. A path that does not exist yet keeps its cleaned absolute form.
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		abs = filepath.Clean(abs)
		allowed := false
		for _, root := range roots {
			if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("refusing to open %q: file:// URLs are limited to the family workspace and Downloads. "+
				"The browser runs outside this app's sandbox, so it must not be used to read elsewhere on this computer", decoded)
		}
	}
	return nil
}

func agentBrowserTool() agentsession.Tool {
	return agentsession.Tool{
		Name: "agent_browser",
		Description: "Control the parent's own signed-in web browser (their dedicated Chrome CDP browser, set up via the Browser " +
			"connector). Use it to read a page the parent can't get as plain text — a school portal, a class site, a shared link. " +
			"PROTOCOL: (1) call agent_browser(command=\"status\", session=\"family\") FIRST — it returns the authorized CDP endpoint to use. " +
			"(2) Call agent_browser(command=\"skills\", args=[\"--cdp\", \"<endpoint>\", \"get\", \"core\"], session=\"family\") once to load the " +
			"CLI's command guide. (3) Every later call must start its args with [\"--cdp\", \"<endpoint>\", ...]. " +
			"The parent's browser usually has MANY tabs and the active one is often unrelated — run command=\"tab\", args=[\"--cdp\", \"<endpoint>\"] " +
			"to list them, pick the one matching what you want, switch to it, then read/snapshot. Never act on an unrelated tab. " +
			"DOWNLOADS land in the parent's Downloads folder, not the workspace — after downloading, copy the file into materials/ or the " +
			"activity folder it belongs to with execute_shell_command before reading it. If status reports CDP isn't reachable, tell the parent to open their signed-in browser " +
			"from Connectors → Browser; don't keep retrying. " +
			"LOGGING INTO A SITE WITH A SAVED SECRET: list_secrets gives you names only, never values — that's intentional. In a " +
			"fill/type arg (e.g. the value for a password field), write the literal placeholder $SECRET_<NAME> (list_secrets gives you " +
			"the exact env-var form to use) and the real value is substituted server-side before it ever reaches the browser; you will " +
			"never see it, and must never ask the parent for it or claim you can't use it. If the site rejects the login (2FA, a " +
			"CAPTCHA, an unfamiliar-device challenge), stop and tell the parent to finish signing in themselves — don't keep retrying " +
			"a form submission blind.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "agent-browser subcommand: status (first, no args), skills, tab, open, read, snapshot, click, fill, type, download, screenshot, etc.",
				},
				"args": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
					"description": "arguments; for CDP mode every call after status must begin with [\"--cdp\", \"<endpoint>\"]. " +
						"A fill/type value may contain $SECRET_<NAME> as a literal placeholder — substituted with the real value " +
						"before it reaches the browser; never write the actual credential here.",
				},
				"session": map[string]interface{}{
					"type":        "string",
					"description": "session name (use \"family\")",
				},
			},
			"required": []string{"command"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			exec, err := getBrowserExecutor()
			if err != nil {
				return "", fmt.Errorf("browser backend unavailable: %w", err)
			}
			if s, _ := args["session"].(string); strings.TrimSpace(s) == "" {
				args["session"] = "family" // Executor requires a non-empty session
			}
			// Substitute $SECRET_<NAME> placeholders in the args array — same
			// syntax and mental model as execute_shell_command's env vars, just
			// resolved here instead of by a shell. The model's own tool call
			// (and the transcript of it) only ever contains the placeholder
			// token; the decrypted value is filled in right here, server-side.
			if rawArgs, ok := args["args"]; ok {
				resolved := rawStringArgs(rawArgs)
				for i, a := range resolved {
					resolved[i] = substituteSecretPlaceholders(a)
				}
				// Validate AFTER substitution: what is checked is exactly what
				// reaches the browser.
				if err := validateBrowserFileURLs(resolved); err != nil {
					return "", err
				}
				args["args"] = resolved
			}
			result, err := exec.HandleAgentBrowser(browserFolderGuardContext(ctx), args)
			// Defense in depth: if the underlying CDP tool ever echoes back what
			// it typed (e.g. a fill confirmation), scrub any secret value that
			// might have leaked into the result before it reaches the model.
			return redactSecrets(result, allSecretValues()), err
		},
	}
}
