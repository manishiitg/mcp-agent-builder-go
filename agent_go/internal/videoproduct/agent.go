package videoproduct

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/workspace/security"
	"github.com/manishiitg/mcpagent/llm"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

//go:embed skills/*/SKILL.md
var skillFiles embed.FS

type AgentEvent struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Workflow   string `json:"workflow,omitempty"`
	Step       string `json:"step,omitempty"`
	ToolCallID string `json:"callId,omitempty"`
	Status     string `json:"status,omitempty"`
	DurationMS int64  `json:"durationMs,omitempty"`
}

type AgentResult struct {
	Reply         string
	SessionHandle []byte
}

type AgentRunner interface {
	Run(ctx context.Context, project ProjectContext, emit func(AgentEvent)) (AgentResult, error)
	Steer(ctx context.Context, projectID, input string) error
	Cancel(projectID string) bool
}

type activeClaudeTurn struct {
	session *agentsession.Session
	cancel  context.CancelFunc
}

type ClaudeRunner struct {
	mu        sync.Mutex
	active    map[string]activeClaudeTurn
	workflows *WorkflowService
}

func NewClaudeRunner(workflows ...*WorkflowService) *ClaudeRunner {
	runner := &ClaudeRunner{active: map[string]activeClaudeTurn{}}
	if len(workflows) > 0 {
		runner.workflows = workflows[0]
	}
	return runner
}

var builtinSkillDefinitions = []struct{ name, description, path string }{
	{"video-creation", "Direct a conversational video project from brief through reproducible production.", "skills/video-creation/SKILL.md"},
	{"video-shot-generation", "Generate consistent AI video shots with references and controlled prompts.", "skills/video-shot-generation/SKILL.md"},
	{"video-editing", "Assemble clips, captions, overlays, narration, music, and versioned exports.", "skills/video-editing/SKILL.md"},
	{"video-quality", "Validate candidate videos technically, visually, and editorially.", "skills/video-quality/SKILL.md"},
}

func builtinSkills() []*llmtypes.Skill {
	out := make([]*llmtypes.Skill, 0, len(builtinSkillDefinitions))
	for _, definition := range builtinSkillDefinitions {
		data, err := skillFiles.ReadFile(definition.path)
		if err != nil {
			continue
		}
		content := string(data)
		if strings.HasPrefix(content, "---\n") {
			if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
				content = content[end+9:]
			}
		}
		out = append(out, &llmtypes.Skill{Name: definition.name, Description: definition.description, Content: content, Source: llmtypes.SkillSource{Origin: "builtin"}})
	}
	return out
}

func videoSystemPrompt(title string) string {
	return `You are the Video Studio coding agent for the project "` + title + `". You are Claude Code, and this product never switches to another coding agent.

The current directory is the complete project workspace. Uploaded references are in uploads/, working files belong in work/, and direct-chat exports belong in outputs/. Never modify uploads/ or write outside this project. Never publish or upload a result unless the user explicitly changes the product requirements in a future version. Use the attached video skills when relevant.

For structured production, use the existing workflow tools. The cinematic workflow has these regular steps: research, proposal, script, scene-plan, assets, edit, compose, delivery. Use execute_step for an approval-led stage at a time, query_step only when the user asks for live status, and run_full_workflow only after the user has approved running the remaining stages. Always pass a short stable group_name for the video, such as video-product-launch, and reuse it for that video's stages. Every execute_step call must pass human_input containing the current user request plus relevant approved decisions from the conversation. Every run_full_workflow call must pass human_inputs keyed by exact step ID; put the complete current brief under research and add any step-specific approvals under their matching step IDs. Later steps receive earlier validated files through workflow context dependencies, so do not replace those files with repeated chat summaries. The workflow runs in the background, so briefly tell the user it has started instead of polling. Workflow steps have no access to learnings, the knowledge base, or workflow database tools.

Messages beginning with [AUTO-NOTIFICATION] are trusted, system-generated workflow completion turns. They resume this same project session after background work finishes. Read the supplied result and relevant project files, then continue the user's task: give a concise useful update, ask for the next approval when required, or start only a stage the user already approved. Never expose the [AUTO-NOTIFICATION] text or internal workflow/framework guidance to the user, and never poll a completed execution again.

Saved credentials are available only to shell commands as environment variables with their configured names. Never print, echo, inspect, or include secret values in chat. Keep replies user-friendly and concise. When work produces a video, name its path under outputs/.`
}

func shellOutput(cmd *exec.Cmd) (string, error) {
	out, err := cmd.CombinedOutput()
	result := string(out)
	const max = 100 * 1024
	if len(result) > max {
		result = result[:max] + "\n...[output truncated]"
	}
	if err != nil {
		if strings.TrimSpace(result) != "" {
			return result + "\n[exit error: " + err.Error() + "]", nil
		}
		return "", fmt.Errorf("command failed: %w", err)
	}
	if strings.TrimSpace(result) == "" {
		return "(command produced no output)", nil
	}
	return result, nil
}

func videoShellTool(workspacePath string, secretEnv []string, emit func(AgentEvent)) agentsession.Tool {
	return agentsession.Tool{
		Name: "execute_shell_command", Category: "video_tools",
		Description: "Run a shell command inside this video's isolated project workspace. Read source assets from uploads/, keep intermediate work in work/, and write final videos to outputs/.",
		Params:      map[string]interface{}{"type": "object", "properties": map[string]interface{}{"command": map[string]interface{}{"type": "string", "description": "shell command to run from the project root"}}, "required": []string{"command"}},
		Handler: func(ctx context.Context, args map[string]interface{}) (result string, err error) {
			started := time.Now()
			callID := fmt.Sprintf("shell-%d", started.UnixNano())
			toolStatus := "completed"
			if emit != nil {
				emit(AgentEvent{Type: "tool", Tool: "execute_shell_command", ToolCallID: callID, Status: "running"})
			}
			defer func() {
				if err != nil || strings.Contains(result, "[exit error:") {
					toolStatus = "failed"
				}
				if emit != nil {
					emit(AgentEvent{Type: "tool", Tool: "execute_shell_command", ToolCallID: callID, Status: toolStatus, DurationMS: time.Since(started).Milliseconds()})
				}
			}()
			command, _ := args["command"].(string)
			if strings.TrimSpace(command) == "" {
				return "", errors.New("command is required")
			}
			cctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			defer cancel()
			iso := &security.Isolator{BaseDir: workspacePath, WorkDir: workspacePath, ReadPaths: []string{"."}, WritePaths: []string{"."}, BlockedWritePaths: []string{"uploads", ".claude"}, StrictAllowlist: true, AllowNetwork: true}
			cmd, cleanup, err := iso.ExecuteIsolated(cctx, command, nil)
			if err != nil {
				return "", fmt.Errorf("sandbox setup failed: %w", err)
			}
			defer cleanup()
			cmd.Env = append(cmd.Env, secretEnv...)
			return shellOutput(cmd)
		},
	}
}

func (r *ClaudeRunner) Run(ctx context.Context, project ProjectContext, emit func(AgentEvent)) (AgentResult, error) {
	var handle *agentsession.Handle
	if len(project.SessionHandle) > 0 {
		var restored agentsession.Handle
		if json.Unmarshal(project.SessionHandle, &restored) == nil && !restored.Empty() {
			handle = &restored
		}
	}
	turnCtx, cancel := context.WithCancel(ctx)
	tools := []agentsession.Tool{videoShellTool(project.WorkspacePath, project.SecretEnv, emit)}
	if r.workflows != nil {
		workflowTools, toolErr := r.workflows.Tools(project, emit)
		if toolErr != nil {
			cancel()
			return AgentResult{}, fmt.Errorf("prepare video workflow: %w", toolErr)
		}
		tools = append(tools, workflowTools...)
	}
	cfg := agentsession.Config{
		Provider: llm.ProviderClaudeCode, ModelID: DefaultClaudeModel, WorkingDir: project.WorkspacePath,
		SystemPrompt: videoSystemPrompt(project.Project.Title), Tools: tools,
		Skills: builtinSkills(), SessionID: "video-project-" + project.Project.ID, SessionHandle: handle,
		StreamCallback: func(text string) {
			if text != "" {
				emit(AgentEvent{Type: "delta", Text: text})
			}
		},
	}
	session, err := agentsession.NewSerialized(turnCtx, cfg)
	if err != nil {
		cancel()
		return AgentResult{}, err
	}
	r.mu.Lock()
	if _, busy := r.active[project.Project.ID]; busy {
		r.mu.Unlock()
		session.Close()
		cancel()
		return AgentResult{}, errors.New("project agent is already working")
	}
	r.active[project.Project.ID] = activeClaudeTurn{session: session, cancel: cancel}
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.active, project.Project.ID); r.mu.Unlock(); session.Close(); cancel() }()
	history := make([]agentsession.Message, 0, len(project.History))
	for _, m := range project.History {
		history = append(history, agentsession.Message{Role: m.Role, Text: m.Body})
	}
	reply, err := session.Ask(turnCtx, history)
	if err != nil {
		return AgentResult{}, err
	}
	var encoded []byte
	if snapshot := session.Handle(); snapshot != nil && !snapshot.Empty() {
		encoded, _ = json.Marshal(snapshot)
	}
	return AgentResult{Reply: reply, SessionHandle: encoded}, nil
}

func (r *ClaudeRunner) Steer(ctx context.Context, projectID, input string) error {
	r.mu.Lock()
	turn, ok := r.active[projectID]
	r.mu.Unlock()
	if !ok {
		return errors.New("project agent is not currently working")
	}
	return turn.session.Send(ctx, input)
}
func (r *ClaudeRunner) Cancel(projectID string) bool {
	r.mu.Lock()
	turn, ok := r.active[projectID]
	r.mu.Unlock()
	if ok {
		turn.cancel()
	}
	return ok
}
