package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/cliruntime"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	mcpagent "github.com/manishiitg/mcpagent/agent"
)

// Workflow chat isolation is the default for coding-agent providers. Keep an
// explicit environment rollback while deployments transition; workflow
// manifests cannot enable or disable this server-owned boundary.
func workflowCLIIsolationEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTWORKS_ISOLATE_WORKFLOW_CLI")), "false")
}

func workflowCLIMode(req *QueryRequest, readOnly bool) string {
	if readOnly {
		return "run"
	}
	if req != nil && req.ExecutionOptions != nil {
		if mode := normalizeChatHistoryWorkshopMode(req.ExecutionOptions.WorkshopMode); mode != "" {
			return mode
		}
	}
	return "workshop"
}

func workflowCLIWorkingDir(folder, user, session, provider, mode string) (string, error) {
	shared := codingAgentWorkspaceWorkingDir(folder)
	if !workflowCLIIsolationEnabled() || !isCodingAgentProvider(provider, "") {
		return shared, nil
	}
	dir, err := cliruntime.Prepare(strings.TrimSpace(os.Getenv("AGENTWORKS_STATE_ROOT")), fsutil.WorkspaceDocsRoot(), user, shared, session, provider, mode)
	if err != nil {
		return "", fmt.Errorf("cannot isolate workflow CLI session: %w (configure a private AGENTWORKS_STATE_ROOT)", err)
	}
	return dir, nil
}

func workflowCLIWorkspaceInstructions(folder string) string {
	return fmt.Sprintf("\nCLI runtime location: the current directory contains this chat's private instructions and skills. The authoritative workflow root is %q. Use absolute paths rooted there for native file tools, and explicitly cd there for native shell commands that use workflow-relative paths. Workspace bridge tools already resolve to that workflow. Keep generated CLI instructions and skills in the private runtime directory.\n", codingAgentWorkspaceWorkingDir(folder))
}

func workflowCLIResumeAllowed(agent *mcpagent.Agent, runtime *ChatHistoryAgentRuntime) bool {
	if !workflowCLIIsolationEnabled() || runtime == nil {
		return true
	}
	current := mcpagent.SnapshotAgentSession(agent)
	if current == nil {
		return false
	}
	// Apply this only to agents constructed with a private runtime directory.
	// Ordinary chats and existing isolated step agents keep their own policy.
	if !strings.Contains(current.Provider.WorkingDir, string(os.PathSeparator)+"cli-runtimes"+string(os.PathSeparator)+"v1"+string(os.PathSeparator)) {
		return true
	}
	if runtime.AgentSessionHandle == nil || !cliruntime.CanResume(current.Provider.WorkingDir, runtime.AgentSessionHandle.Provider.WorkingDir) {
		return false
	}
	// Codex's project directory also selects the process cwd and can override
	// WorkingDir. Validate both persisted representations before applying either.
	if strings.EqualFold(current.Provider.Provider, "codex-cli") {
		for _, projectDir := range []string{runtime.ProjectDirID, runtime.AgentSessionHandle.Provider.ProjectDirID} {
			if projectDir != "" && !cliruntime.CanResume(current.Provider.WorkingDir, projectDir) {
				return false
			}
		}
	}
	return true
}
