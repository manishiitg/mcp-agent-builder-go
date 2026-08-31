package virtualtools

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/browser"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// GetWorkspaceBrowserToolCategory returns the category name for workspace browser tools
func GetWorkspaceBrowserToolCategory() string {
	return "workspace_browser"
}

// CreateWorkspaceBrowserTools creates the single agent_browser virtual tool
func CreateWorkspaceBrowserTools() []llmtypes.Tool {
	return []llmtypes.Tool{browser.GetToolDefinition()}
}

// CreateWorkspaceBrowserToolExecutors creates the execution functions for workspace browser tools.
// Optional CDP ports authorize one or more independently-profiled Chrome browsers.
func CreateWorkspaceBrowserToolExecutors(cdpPort ...int) map[string]func(ctx context.Context, args map[string]interface{}) (string, error) {
	return CreateWorkspaceBrowserToolExecutorsWithSession("", cdpPort...)
}

// CreateWorkspaceBrowserToolExecutorsWithSession creates browser tool executors with chat session tracking.
// sessionID is the chat/workflow session ID — used to enforce per-session browser limits.
// Multiple ports are an explicit opt-in for separate login identities within
// one run; normal workflow concurrency should continue sharing one CDP port.
func CreateWorkspaceBrowserToolExecutorsWithSession(sessionID string, cdpPort ...int) map[string]func(ctx context.Context, args map[string]interface{}) (string, error) {
	mode := "headless"
	if len(cdpPort) > 0 {
		mode = "cdp"
	}
	return CreateWorkspaceBrowserToolExecutorsWithRuntime(
		sessionID,
		browser.NewBrowserRuntimeConfig(mode, cdpPort),
	)
}

// CreateWorkspaceBrowserToolExecutorsWithRuntime creates an executor backed by
// configured browser intent. In auto mode the shared runtime performs a live
// CDP status check for every status/action call; resolved availability is never
// copied into chat history or cached workshop prompts.
func CreateWorkspaceBrowserToolExecutorsWithRuntime(sessionID string, runtime *browser.BrowserRuntimeConfig) map[string]func(ctx context.Context, args map[string]interface{}) (string, error) {
	executors := make(map[string]func(ctx context.Context, args map[string]interface{}) (string, error))

	// Wire up the browser executor from the pkg/browser package
	browserClient := browser.NewClient(getWorkspaceAPIURL())
	browserExecutor := browser.NewExecutor(browserClient,
		browser.WithBrowserRuntimeConfig(runtime),
		browser.WithOversizedOutputSpiller(spillOversizedBrowserOutput),
	)

	// Wrap executor to inject the workflow session ID. Delegated agents inherit
	// this browser session; tool-session isolation does not fork browser state.
	executors["agent_browser"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		if existingID, ok := ctx.Value(common.ChatSessionIDKey).(string); ok && existingID != "" {
			// Preserve the session injected by /s/{session_id}/tools/... routes.
			log.Printf("[BROWSER_TOOLS] Preserving context agent session: %s (parent workflow: %s)", existingID, sessionID)
		} else if sessionID != "" {
			ctx = context.WithValue(ctx, common.ChatSessionIDKey, sessionID)
		}
		// Always set the workflow-level session to the parent (for per-workflow limits)
		if sessionID != "" {
			ctx = context.WithValue(ctx, common.WorkflowSessionIDKey, sessionID)
		}
		return browserExecutor.HandleAgentBrowser(ctx, args)
	}

	return executors
}

// spillOversizedBrowserOutput persists an oversized agent_browser result
// (PLAT-200: the default truncated path used to just discard it) so the
// calling step can read it back without re-running the whole snapshot.
//
// It never invents a write location. tool_output_folder is deliberately
// read-only for agent-facing writes (PLAT-073 cluster F / PLAT-078): only the
// trusted bridge machinery writes there, so every step's folder guard already
// grants it read access without also granting write access. This function
// checks the CALLING SESSION'S OWN already-granted ReadPaths for an entry
// ending in tool_output_folder and only writes if one is present -- reusing
// PLAT-078's proven grant instead of re-deriving a workspace path itself. No
// grant found (or no session at all, e.g. the schema-only, session-less
// executor variant) means no spill; the caller's existing truncate-and-offer-
// a-rerun behavior is unchanged.
func spillOversizedBrowserOutput(ctx context.Context, content string) (string, error) {
	sessionID, _ := ctx.Value(common.ChatSessionIDKey).(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("no session id on context, declining to spill")
	}
	cfg := common.GetSessionShellConfig(sessionID)
	if cfg == nil {
		return "", fmt.Errorf("session %s has no folder guard configured, declining to spill", sessionID)
	}
	toolOutputDir := ""
	for _, p := range cfg.ReadPaths {
		p = strings.TrimSuffix(strings.TrimSpace(p), "/")
		if p == "tool_output_folder" || strings.HasSuffix(p, "/tool_output_folder") {
			toolOutputDir = p
			break
		}
	}
	if toolOutputDir == "" {
		return "", fmt.Errorf("session %s has no tool_output_folder read grant, declining to spill", sessionID)
	}

	relPath := fmt.Sprintf("%s/agent_browser_snapshot_%d.txt", toolOutputDir, time.Now().UnixNano())
	client := workspace.NewClient(getWorkspaceAPIURL(), workspace.WithExtraEnv(getMCPExtraEnv(sessionID)))
	// tool_output_folder is not in this session's own WritePaths by design (see
	// comment above) -- this trusted, platform-initiated write (never an agent
	// tool-call argument) is the documented exception WithSystemManagedWritePaths
	// exists for, scoped to exactly this directory.
	writeCtx := workspace.WithSystemManagedWritePaths(ctx, toolOutputDir)
	if _, err := client.UpdateWorkspaceFile(writeCtx, workspace.UpdateWorkspaceFileParams{
		Filepath: relPath,
		Content:  content,
	}); err != nil {
		return "", fmt.Errorf("failed to spill oversized snapshot to %s: %w", relPath, err)
	}
	return relPath, nil
}
