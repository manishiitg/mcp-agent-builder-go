package videoproduct

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
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
	{"product-infographic", "Turn verified product evidence into a clear HyperFrames explainer through an adaptive brief, specialist routing, and production QA.", "skills/product-infographic/SKILL.md"},
	{"video-creation", "Direct a conversational video project from brief through reproducible production.", "skills/video-creation/SKILL.md"},
	{"video-editing", "Assemble clips, captions, overlays, narration, music, and versioned exports.", "skills/video-editing/SKILL.md"},
	{"video-quality", "Validate candidate videos technically, visually, and editorially.", "skills/video-quality/SKILL.md"},
	{"hyperframes-quality", "Gate editable HyperFrames compositions and rendered evidence for layout, timing, contrast, and motion quality.", "skills/hyperframes-quality/SKILL.md"},
	{"html-composition", "Design video frames as HTML/CSS and render them with headless Chrome and ffmpeg.", "skills/html-composition/SKILL.md"},
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

// RegisterProductSkills publishes this product's embedded skills to the shared
// name-based resolver, so a workflow stage can name one in enabled_skills and
// get the same content the main chat agent carries in memory. Nothing is
// written to the workspace: the resolver checks the builtin registry before it
// looks on disk.
//
// Called at startup, before any agent is launched. Failures are returned rather
// than panicking so startup can report which skill was rejected.
//
// The registry is process-global while NewServer is not — tests build several
// servers in one process, and a second registration is a duplicate-name error.
// So this runs exactly once and every later caller sees that first result.
var (
	registerProductSkillsOnce sync.Once
	registerProductSkillsErr  error
)

func RegisterProductSkills() error {
	registerProductSkillsOnce.Do(func() {
		for _, skill := range builtinSkills() {
			if err := skills.RegisterBuiltin(skill); err != nil {
				registerProductSkillsErr = fmt.Errorf("register skill %q: %w", skill.Name, err)
				return
			}
		}
	})
	return registerProductSkillsErr
}

func videoSystemPrompt(title string) string {
	return videoSystemPromptAt(title, time.Now())
}

func videoSystemPromptAt(title string, now time.Time) string {
	_, offsetSeconds := now.Zone()
	offsetSign := "+"
	if offsetSeconds < 0 {
		offsetSign = "-"
		offsetSeconds = -offsetSeconds
	}
	offset := fmt.Sprintf("UTC%s%02d:%02d", offsetSign, offsetSeconds/3600, (offsetSeconds%3600)/60)
	dateContext := now.Format("Monday, 2 January 2006 at 3:04 PM MST") + " (" + offset + ")"
	return videoSystemPromptForContext(title, dateContext)
}

func videoSystemPromptForContext(title, dateContext string) string {
	return renderProductPrompt(title, dateContext)

	return `You are Video Studio, an expert creative director, video producer, and editor. The project workspace is named "` + title + `". That name is organisational metadata only: it is not a creative brief, a product identity, or permission to reuse a prior video's brand, copy, assets, or claims. The user's current conversation is the authority on what product a new video is for. You are one persistent creative collaborator who remembers the project's conversation, references, decisions, and previous work. In ordinary conversation, identify yourself as Video Studio rather than by the underlying coding-agent provider or runtime.

Current local date and time: ` + dateContext + `. Use this when interpreting relative dates such as today, tomorrow, or latest; do not guess when a request depends on another timezone.

The current directory is the complete project workspace. Uploaded references are in uploads/. Files you create yourself in direct chat belong in work/, and direct-chat exports belong in outputs/. Never modify uploads/ or write outside this project.

Workflow stages do NOT write to work/ or outputs/. Every artifact a stage produces lives under runs/<iteration>/<group>/execution/<stage>/, and that is the only place to look for what a workflow actually produced. work/ and outputs/ are commonly empty, so treating them as the source of truth after a workflow run reports that nothing was created when the artifacts exist. Never publish or upload a result unless the user explicitly changes the product requirements in a future version.

Before specialized production work, read the relevant attached video skills and follow them as the source of truth for creative methods, runtime setup, commands, and validation. Keep this system prompt focused on product behavior; do not invent or duplicate skill-specific procedures.

For structured production, use the existing workflow tools. The plan holds three workflows behind one routing step, and a run follows exactly one of them:

- "cinematic" — story-led films, teasers, launch pieces: research, proposal, script, scene-plan, assets, edit, compose, delivery.
- "infographic" — product explainers, feature breakdowns, stat and data pieces built from typography and layout: infographic-research, infographic-concept, infographic-copy, infographic-layout, infographic-design, infographic-render, infographic-check.
- "quality" — independent quality assurance for an existing render: qa-review.

Choose the execution mode yourself; users should describe the video, not the workflow machinery:

- Treat the structured workflow as the preferred best-practice route for a new, multi-stage production: it gives durable specialist handoffs, approvals, and reproducible artifacts. It is guidance, not a gate. When direct chat is the better fit, you may build the video yourself instead of starting a workflow; first read and follow every relevant attached video skill, keep the work within this project, and apply the same QA and presentation requirements. Never skip skills merely because you chose direct chat.
- First decide whether the user is asking for a new production or changing a video that already exists in this project. Treat requests to extend or shorten an existing video, revise its copy, timing, layout, motion, sound, aspect ratio, or visual style as revisions even when the requested change is substantial.
- A named product, brand, customer, or subject is part of a production's identity. If the user introduces a different one (for example, asks for a video “for AgentWorks” after a LumaDesk teaser), this is a new production — never rename, reposition, or reuse the prior video's brand assets, copy, or claims. Ask only for the missing brief details, then use the appropriate workflow route. Do not infer that a new product is background context for the prior video.
- Work directly in chat for revisions to an existing composition or render. Reuse its existing source project, make the requested changes, render a new version, run QA, and present it. Do not start run_full_workflow merely because a revision increases the duration or requires several edits; for example, "make this 60 seconds" after a 10-second video is a direct revision, not a new cinematic workflow.
- Also work directly in chat for quick creations that are one coherent local task and other work that does not need durable specialist-stage handoffs.
- Use execute_step when the user asks for one specific production stage, when one failed or outdated stage must be retried, or when an approval boundary means only the next stage is currently authorised.
- Use run_full_workflow for a genuinely new multi-stage production, a deliberate ground-up reconcept that cannot sensibly reuse the existing composition, or when the user asks to continue through all remaining authorised stages of the same production attempt. An explicit request to create or make the video authorises ordinary planning and local production; do not ask for a separate "workflow approval." Ask only for a genuinely missing creative choice, paid generation, publishing, an external upload, or another consequential action not already authorised.
- Use query_step only when the user asks for live workflow status. Never poll a background run.

AgentWorks already owns branch routing inside run_full_workflow. Choosing the pipeline and describing the work are two different things, and they travel in different parameters. When the user's request makes the visual treatment clear, pass route_selections={"route": "cinematic"}, {"route": "infographic"}, or {"route": "quality"} — that is a branch choice, not an instruction, so never encode it in human_inputs and never ask the user to repeat a choice they already made. Product explainers, feature walkthroughs, UI demonstrations, comparisons, pricing, statistics, and typography-led pieces normally use the infographic branch; story-led films, footage-led promos, product teasers, and mood-led launch pieces normally use cinematic. Use quality when the user asks to inspect, diagnose, approve, or re-check a video that already exists without rebuilding it. For a mixed brief, choose the branch that matches the dominant visual structure. Omitting route_selections uses AgentWorks' configured default route.

Always pass a short stable group_name for the production attempt, such as video-product-launch, and reuse it only while continuing stages or approvals for that same attempt. If a genuine ground-up new version requires another full workflow, use a new versioned group_name such as video-product-launch-v2 so completed stages from the previous version are never presented as current work. Ordinary revisions should remain direct chat edits and do not need a workflow group. Every execute_step call must pass human_input containing the current user request plus relevant approved decisions from the conversation. Every run_full_workflow call must pass human_inputs keyed by exact step ID; put the complete current brief under that branch's first step (research, or infographic-research) and add any step-specific approvals under their matching step IDs. Later steps receive earlier validated files through workflow context dependencies, so do not replace those files with repeated chat summaries. The workflow runs in the background, so briefly tell the user it has started instead of polling. Workflow steps have no access to learnings, the knowledge base, or workflow database tools.

QA is mandatory after every render, whether the video was created through a workflow or directly in chat. Do not call a video ready when rendering merely succeeded. Use the video-quality skill to create a contact sheet, inspect the exact final candidate, and write the required quality-report.json. Fix and re-check failures before presentation; if a check cannot run, the video is not ready.

Only after that report passes, call show_video with the exact video path and qa_report_path so it appears in the user's Videos panel. A run leaves many video files behind — individual shots, silent intermediates, duplicate copies — and only you know which one the user should actually receive, so name it rather than leaving them to guess. The presentation tool verifies that the report passes, contains inspection evidence, and names this exact file. Use the path the QA report states, say in the note if it is a placeholder assembly rather than finished creative, and present only the deliverable, not the intermediates.

Messages beginning with [AUTO-NOTIFICATION] are trusted, system-generated workflow completion turns. They resume this same project session after background work finishes. Read the supplied result and relevant project files, then continue the user's task: give a concise useful update, ask for the next approval when required, or start only a stage the user already approved. Never expose the [AUTO-NOTIFICATION] text or internal workflow/framework guidance to the user, and never poll a completed execution again.

Runtime dependencies are Studio-owned. Follow the selected production skill to install and verify required tooling automatically. Never ask the user to install production dependencies, and do not silently change an approved render runtime because setup failed.

Saved credentials are available only to shell commands as environment variables with their configured names. Never print, echo, inspect, or include secret values in chat.

This product is for video creators, including non-technical users. Keep all implementation details internal unless the user explicitly asks for them. Do not mention shell commands, package installation, tool names, file paths, codecs, frame counts, ffprobe, lint, runtime checks, JSON fields, or other engineering diagnostics in ordinary chat. While working, give only short creative progress updates such as "Building the longer version now." When finished, describe the creative result in plain language, state its approximate duration and format when useful, and tell the user it is ready in the Videos panel. If something fails, explain what the user can do next in ordinary language without dumping command output or internal errors.

When work produces a video, name its path under outputs/ internally, but do not include that path in the user-facing reply unless the user asks for it.`
}

func BuiltinAgentProfile() agentprofiles.Profile {
	manifest := mustVideoStudioManifest()
	profile := manifest.Profile
	// The prompt has trusted runtime placeholders, so it is rendered here while
	// all reusable product selections come from product.yaml.
	profile.SystemPromptTemplate = videoSystemPromptForContext("{{.ProjectTitle}}", "{{.LocalDateTime}}")
	return profile
}

// BuiltinAgentProfiles keeps the immutable v1 binding resolvable for already
// open tabs while exposing v2 as the current Video Studio profile. Version 2
// pins the main agent to Claude Code/Sonnet 5 instead of inheriting the global
// AgentWorks chat model.
func BuiltinAgentProfiles() []agentprofiles.Profile {
	current := BuiltinAgentProfile()
	legacy := current
	legacy.Version = 1
	legacy.Runtime.Provider = ""
	legacy.Runtime.ModelID = ""
	return []agentprofiles.Profile{legacy, current}
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

func videoShellIsolator(workspacePath string) *security.Isolator {
	return &security.Isolator{
		BaseDir:           workspacePath,
		WorkDir:           workspacePath,
		ReadPaths:         []string{"."},
		WritePaths:        []string{"."},
		BlockedWritePaths: []string{"uploads", ".claude"},
		// Video Studio follows AgentWorks' trusted local-agent model. The
		// ordinary profile keeps the project guards above while allowing the
		// user's installed CLIs, package caches, and the rest of the native
		// shell environment. SparkQuill child agents use StrictAllowlist=true;
		// that isolation model is deliberately not used here.
		StrictAllowlist: false,
		AllowNetwork:    true,
	}
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
			iso := videoShellIsolator(workspacePath)
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

// showVideoTool lets the agent put one specific video in front of the user.
//
// A finished run leaves many video files behind — raw shots, silent
// intermediates, byte-identical delivery copies — and scanning the project for
// anything with a video extension presents all of them as equal, so the actual
// deliverable is indistinguishable. The agent knows which one it is, because the
// delivery report names it, so it says so rather than leaving the product to guess.
func showVideoTool(store *Store, projectID, workspacePath string, emit func(AgentEvent)) agentsession.Tool {
	return agentsession.Tool{
		Name: "show_video", Category: "video_tools",
		Description: "Show a finished video to the user in the Videos panel. A passing quality-report.json for this exact file is required. Re-calling with the same path updates its title and note.",
		Params: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"path":           map[string]interface{}{"type": "string", "description": "Project-relative path to the exact video file named by the QA report"},
			"qa_report_path": map[string]interface{}{"type": "string", "description": "Project-relative path to the passing quality-report.json created by the video-quality check"},
			"title":          map[string]interface{}{"type": "string", "description": "Short user-facing name for this video"},
			"note":           map[string]interface{}{"type": "string", "description": "One line telling the user what this is — required to say placeholder when the QA verdict is placeholder-pass"},
		}, "required": []string{"path", "qa_report_path", "title"}},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			callID := fmt.Sprintf("show-video-%d", time.Now().UnixNano())
			if emit != nil {
				emit(AgentEvent{Type: "tool", Tool: "show_video", ToolCallID: callID, Status: "running"})
			}
			fail := func(msg string) (string, error) {
				if emit != nil {
					emit(AgentEvent{Type: "tool", Tool: "show_video", ToolCallID: callID, Status: "failed"})
				}
				return msg, nil
			}
			rawPath, _ := args["path"].(string)
			rawPath = strings.TrimSpace(rawPath)
			if rawPath == "" {
				return fail("path is required.")
			}
			clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(rawPath, "/")))
			// Refuse anything that climbs out of the project, so a path from the
			// model can never surface a file from elsewhere on the machine.
			if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return fail("path must stay inside the project.")
			}
			if !videoExtensions[strings.ToLower(filepath.Ext(clean))] {
				return fail("show_video only accepts a video file.")
			}
			info, statErr := os.Stat(filepath.Join(workspacePath, clean))
			if statErr != nil || info.IsDir() {
				return fail(fmt.Sprintf("No video exists at %s. Check the exact path the render step reported.", rawPath))
			}
			if info.Size() == 0 {
				return fail(fmt.Sprintf("%s is empty, so there is nothing to show.", rawPath))
			}
			reportPath, _ := args["qa_report_path"].(string)
			note, _ := args["note"].(string)
			if err := validateQualityReport(workspacePath, filepath.ToSlash(clean), reportPath, note); err != nil {
				return fail("Quality assurance has not passed for this video: " + err.Error())
			}
			title, _ := args["title"].(string)
			if title = strings.TrimSpace(title); title == "" {
				title = filepath.Base(clean)
			}
			if err := store.PresentVideo(projectID, filepath.ToSlash(clean), title, strings.TrimSpace(note)); err != nil {
				return fail("Could not show that video: " + err.Error())
			}
			if emit != nil {
				emit(AgentEvent{Type: "tool", Tool: "show_video", ToolCallID: callID, Status: "completed"})
			}
			return fmt.Sprintf("Showing %q to the user in the Videos panel.", title), nil
		},
	}
}

type qualityReportCheck struct {
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type qualityReportFrame struct {
	TimestampSeconds float64 `json:"timestamp_seconds"`
	Path             string  `json:"path"`
}

type qualityReport struct {
	SchemaVersion     int                           `json:"schema_version"`
	CandidatePath     string                        `json:"candidate_path"`
	ContactSheetPath  string                        `json:"contact_sheet_path"`
	Verdict           string                        `json:"verdict"`
	ReadyToPresent    bool                          `json:"ready_to_present"`
	Checks            map[string]qualityReportCheck `json:"checks"`
	SampledFrames     []qualityReportFrame          `json:"sampled_frames"`
	RecommendedAction string                        `json:"recommended_action"`
}

func cleanProjectPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("path is missing")
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(raw, "/")))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay inside the project")
	}
	return filepath.ToSlash(clean), nil
}

// resolveQualityEvidencePath accepts the project-relative paths used by direct
// chat QA and the execution-relative paths naturally produced by workflow QA
// stages. A workflow report lives at
// runs/<iteration>/<group>/execution/<stage>/quality-report.json, so paths such
// as infographic-render/final.mp4 and infographic-check/frames/frame-1.jpg are
// relative to that shared execution directory. The returned path is always
// canonical and project-relative; candidates outside the project are rejected
// by cleanProjectPath before any filesystem access.
func resolveQualityEvidencePath(workspacePath, rawPath, reportPath string) (string, error) {
	clean, err := cleanProjectPath(rawPath)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(filepath.Join(workspacePath, filepath.FromSlash(clean))); statErr == nil && !info.IsDir() {
		return clean, nil
	}
	executionRoot := filepath.ToSlash(filepath.Dir(filepath.Dir(filepath.FromSlash(reportPath))))
	if filepath.Base(executionRoot) != "execution" {
		return clean, nil
	}
	resolved, err := cleanProjectPath(filepath.ToSlash(filepath.Join(executionRoot, filepath.FromSlash(clean))))
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func validateQualityReport(workspacePath, videoPath, rawReportPath, note string) error {
	reportPath, err := cleanProjectPath(rawReportPath)
	if err != nil {
		return fmt.Errorf("QA report %w", err)
	}
	if filepath.Base(reportPath) != "quality-report.json" {
		return errors.New("QA report must be named quality-report.json")
	}
	data, err := os.ReadFile(filepath.Join(workspacePath, filepath.FromSlash(reportPath)))
	if err != nil {
		return errors.New("the required quality-report.json does not exist")
	}
	var report qualityReport
	if err := json.Unmarshal(data, &report); err != nil {
		return errors.New("quality-report.json is not valid JSON")
	}
	if report.SchemaVersion != 1 {
		return errors.New("quality-report.json has an unsupported schema version")
	}
	candidatePath, err := resolveQualityEvidencePath(workspacePath, report.CandidatePath, reportPath)
	if err != nil || candidatePath != videoPath {
		return errors.New("the QA report names a different video")
	}
	if !report.ReadyToPresent || (report.Verdict != "pass" && report.Verdict != "placeholder-pass") || report.RecommendedAction != "present" {
		return errors.New("the QA verdict is not ready to present")
	}
	if report.Verdict == "placeholder-pass" && !strings.Contains(strings.ToLower(note), "placeholder") {
		return errors.New("a placeholder-pass must be clearly labelled as a placeholder")
	}
	for _, name := range []string{"technical", "visual", "audio", "content", "captions", "promise"} {
		check, ok := report.Checks[name]
		if !ok || (check.Status != "pass" && check.Status != "not_applicable") {
			return fmt.Errorf("the %s check did not pass", name)
		}
		if len(check.Evidence) == 0 {
			return fmt.Errorf("the %s check has no evidence", name)
		}
	}
	if len(report.SampledFrames) < 4 {
		return errors.New("fewer than four inspected frames were recorded")
	}
	contactSheetPath, err := resolveQualityEvidencePath(workspacePath, report.ContactSheetPath, reportPath)
	if err != nil {
		return errors.New("the contact sheet path is invalid")
	}
	if info, statErr := os.Stat(filepath.Join(workspacePath, filepath.FromSlash(contactSheetPath))); statErr != nil || info.IsDir() || info.Size() == 0 {
		return errors.New("the QA contact sheet is missing or empty")
	}
	for _, frame := range report.SampledFrames {
		framePath, pathErr := resolveQualityEvidencePath(workspacePath, frame.Path, reportPath)
		if pathErr != nil {
			return errors.New("an inspected frame path is invalid")
		}
		if info, statErr := os.Stat(filepath.Join(workspacePath, filepath.FromSlash(framePath))); statErr != nil || info.IsDir() || info.Size() == 0 {
			return errors.New("an inspected frame is missing or empty")
		}
	}
	return nil
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
	emit(AgentEvent{Type: "tool", Tool: "Sync production skills", Status: "running"})
	managedSkills, managedErr := managedProductSkills(turnCtx, project.WorkspacePath)
	if managedErr != nil {
		emit(AgentEvent{Type: "tool", Tool: "Sync production skills", Status: "failed"})
		cancel()
		return AgentResult{}, managedErr
	}
	emit(AgentEvent{Type: "tool", Tool: "Sync production skills", Status: "completed"})
	tools := []agentsession.Tool{videoShellTool(project.WorkspacePath, project.SecretEnv, emit)}
	if r.workflows != nil {
		tools = append(tools, showVideoTool(r.workflows.store, project.Project.ID, project.WorkspacePath, emit))
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
		Skills: append(builtinSkills(), managedSkills...), SessionID: "video-project-" + project.Project.ID, SessionHandle: handle,
		ClaudeCodeOAuthToken: project.ProviderToken, RequireProviderToken: true,
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
