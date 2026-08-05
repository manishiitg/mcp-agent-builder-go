package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/browser"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"

	"github.com/manishiitg/mcpagent/events"
)

// workspaceEventEmitterKey matches the key used in virtualtools package
const workspaceEventEmitterKey common.ContextKey = "workspace_event_emitter"

// emitWorkspaceFileEvent emits a workspace_file_operation event if an emitter is present in context
func emitWorkspaceFileEvent(ctx context.Context, operation, filepath, folder string) {
	emitter, ok := ctx.Value(workspaceEventEmitterKey).(interface {
		HandleEvent(ctx context.Context, event *events.AgentEvent) error
	})
	if !ok || emitter == nil {
		return
	}

	turn, _ := ctx.Value(common.ContextKey("turn")).(int)
	serverName, _ := ctx.Value(common.ContextKey("server_name")).(string)

	eventData := events.NewWorkspaceFileOperationEvent(operation, filepath, folder, turn, serverName)
	agentEvent := &events.AgentEvent{
		Type:      events.WorkspaceFileOperation,
		Timestamp: time.Now(),
		Data:      eventData,
	}
	_ = emitter.HandleEvent(ctx, agentEvent)
}

// Helper to convert generic map args to typed struct
func mapToStruct(args map[string]interface{}, v interface{}) error {
	data, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// The basic file tools (read/list/update/delete/move_workspace_file) were
// removed on 2026-08-05. They had not been reachable by an agent since the
// basic/advanced split: the server only ever registers the advanced set, and
// the "workspace_tools" category resolves to the advanced registry plus the
// browser. They survived as an executor map used by one dev harness, and cost
// more than they were worth — a reader (human or agent) auditing which tool
// results are bounded finds them in this file and reasonably concludes agents
// can call them. Reading a workspace file is execute_shell_command's job; it
// is capped and can be sliced with head/tail/sed. The typed Client methods
// (ReadWorkspaceFile and friends) are unaffected — the server uses them
// throughout to read files in Go.

// NewAdvancedExecutor creates executors for advanced workspace tools (shell, image, pdf, diff patch)
func NewAdvancedExecutor(client *Client) map[string]func(ctx context.Context, args map[string]interface{}) (string, error) {
	executors := make(map[string]func(ctx context.Context, args map[string]interface{}) (string, error))

	executors["execute_shell_command"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		var params ExecuteShellCommandParams
		if err := mapToStruct(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		result, err := client.ExecuteShellCommand(ctx, params)
		if err != nil {
			return "", err
		}
		// Only the agent-facing result is capped. Scripted steps call
		// client.ExecuteShellCommand directly and parse stdout as schema-validated
		// JSON, so they must keep every byte.
		//
		// The cap is enforced on the encoded payload, not on the raw streams:
		// JSON escaping happens after any stream-level check and can multiply the
		// delivered size several times over.
		return marshalCappedShellResultForAgent(result)
	}

	executors["read_image"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		var params ReadImageParams
		if err := mapToStruct(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		return client.ReadImage(ctx, params)
	}

	executors["diff_patch_workspace_file"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		var params DiffPatchWorkspaceFileParams
		if err := mapToStruct(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		result, err := client.DiffPatchWorkspaceFile(ctx, params)
		if err != nil {
			return "", err
		}
		emitWorkspaceFileEvent(ctx, "patch", params.Filepath, "")
		return marshalResult(result)
	}

	return executors
}

// NewBrowserExecutor creates executors for browser tools
func NewBrowserExecutor(client *Client) map[string]func(ctx context.Context, args map[string]interface{}) (string, error) {
	executors := make(map[string]func(ctx context.Context, args map[string]interface{}) (string, error))

	browserClient := browser.NewClient(client.BaseURL)
	browserExecutor := browser.NewExecutor(browserClient)
	executors["agent_browser"] = browserExecutor.HandleAgentBrowser

	return executors
}
