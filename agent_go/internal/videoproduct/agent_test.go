package videoproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func TestBuiltinAgentProfileIsValidAndRenderSafe(t *testing.T) {
	profile := BuiltinAgentProfile()
	if err := agentprofiles.Validate(profile); err != nil {
		t.Fatalf("invalid built-in profile: %v", err)
	}
	rendered, err := agentprofiles.RenderPrompt(profile, agentprofiles.PromptContext{
		ProjectTitle:  "Launch Film",
		LocalDateTime: "Friday, 7 August 2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "Launch Film") || !strings.Contains(rendered, "Friday, 7 August 2026") {
		t.Fatalf("profile context was not rendered: %q", rendered[:min(len(rendered), 300)])
	}
	if profile.Version != 2 || profile.Runtime.Provider != "claude-code" || profile.Runtime.ModelID != DefaultClaudeModel {
		t.Fatalf("Video Studio main agent is not pinned to Claude Code/Sonnet 5: version=%d runtime=%+v", profile.Version, profile.Runtime)
	}
	hasBrowserSkill := false
	for _, skill := range profile.Skills {
		if skill == "agent-browser" {
			hasBrowserSkill = true
			break
		}
	}
	if profile.Runtime.Capabilities.Browser != agentprofiles.CapabilityRequired || profile.Runtime.Capabilities.Secrets != agentprofiles.CapabilityRequired || !hasBrowserSkill {
		t.Fatalf("Video Studio does not reuse AgentWorks browser/secrets capabilities: runtime=%+v skills=%v", profile.Runtime.Capabilities, profile.Skills)
	}
	if profile.Runtime.AgentTools.Mode != "hybrid" || profile.Runtime.Approvals.Mode != "provider_auto" {
		t.Fatalf("Video Studio native-tool policy = %+v %+v, want hybrid/provider_auto", profile.Runtime.AgentTools, profile.Runtime.Approvals)
	}
	profiles := BuiltinAgentProfiles()
	if len(profiles) != 2 || profiles[0].Version != 1 || profiles[0].Runtime.Provider != "" || profiles[1].Version != 2 {
		t.Fatalf("unexpected immutable profile versions: %+v", profiles)
	}
}

func TestIntegratedWorkflowPinsClaudeCodeSonnet5(t *testing.T) {
	manifest := integratedWorkflowManifest("demo", "Demo")
	capabilities := manifest["capabilities"].(map[string]interface{})
	llmConfig := capabilities["llm_config"].(map[string]interface{})
	if llmConfig["mode"] != "explicit" {
		t.Fatalf("workflow LLM mode = %v, want explicit", llmConfig["mode"])
	}
	builderLLM := llmConfig["builder_llm"].(map[string]interface{})
	if builderLLM["provider"] != "claude-code" || builderLLM["model_id"] != DefaultClaudeModel {
		t.Fatalf("workflow builder LLM = %+v", builderLLM)
	}
	config := integratedStepConfig()
	steps := config["steps"].([]map[string]interface{})
	if len(steps) == 0 {
		t.Fatal("integrated step config has no steps")
	}
	for _, step := range steps {
		executionLLM := step["agent_configs"].(map[string]interface{})["execution_llm"].(map[string]interface{})
		if executionLLM["provider"] != "claude-code" || executionLLM["model_id"] != DefaultClaudeModel {
			t.Fatalf("step %v execution LLM = %+v", step["id"], executionLLM)
		}
	}
}

func TestBuiltinVideoSkills(t *testing.T) {
	want := map[string]string{
		"product-infographic": "BRIEF.md",
		"video-creation":      "work/production.json",
		"video-editing":       "hard cuts",
		"video-quality":       "work/qa/",
		"html-composition":    "headless Chrome",
	}

	skills := builtinSkills()
	if len(skills) != len(want) {
		t.Fatalf("builtin skill count = %d, want %d", len(skills), len(want))
	}
	for _, skill := range skills {
		required, ok := want[skill.Name]
		if !ok {
			t.Fatalf("unexpected builtin skill %q", skill.Name)
		}
		if strings.TrimSpace(skill.Content) == "" {
			t.Fatalf("builtin skill %q has empty content", skill.Name)
		}
		if !strings.Contains(skill.Content, required) {
			t.Fatalf("builtin skill %q does not contain %q", skill.Name, required)
		}
		if skill.Source.Origin != "builtin" {
			t.Fatalf("builtin skill %q origin = %q", skill.Name, skill.Source.Origin)
		}
	}
}

func TestHyperFramesRuntimeIsAgentManaged(t *testing.T) {
	const managedCommand = "npx --yes hyperframes@latest"

	prompt := videoSystemPrompt("Runtime test")
	if strings.Contains(prompt, managedCommand) {
		t.Fatalf("video system prompt contains an executable HyperFrames command")
	}
	for _, required := range []string{"read the relevant attached video skills", "Runtime dependencies are Studio-owned", "Never ask the user to install production dependencies", "HyperFrames is the primary composition system", "YAML-managed HyperFrames skills"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("video system prompt is missing product/runtime guidance %q", required)
		}
	}

	var editingSkill string
	for _, skill := range builtinSkills() {
		if skill.Name == "video-editing" {
			editingSkill = skill.Content
			break
		}
	}
	if !strings.Contains(editingSkill, managedCommand) {
		t.Fatalf("video-editing skill does not use the managed HyperFrames command")
	}
	if !strings.Contains(editingSkill, "optional transcription") || !strings.Contains(editingSkill, "require Version, Node.js, FFmpeg, FFprobe, and Chrome") {
		t.Fatalf("video-editing skill does not distinguish required Doctor checks from optional capabilities")
	}
}

func TestVideoPromptKeepsTechnicalWorkInternal(t *testing.T) {
	prompt := videoSystemPrompt("Creator-friendly test")
	for _, required := range []string{
		"including non-technical users",
		"Keep all implementation details internal",
		"Do not mention shell commands",
		"ready in the Videos panel",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("video system prompt is missing creator-friendly guidance %q", required)
		}
	}
}

func TestVideoPromptExplainsDirectProjectTools(t *testing.T) {
	prompt := videoSystemPrompt("Tool guidance test")
	for _, required := range []string{
		"normal coding-agent tools",
		"`show_video`",
		"The browser is skill-led",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("video system prompt is missing tool guidance %q", required)
		}
	}
	for _, removed := range []string{"`execute_shell_command`", "`diff_patch_workspace_file`"} {
		if strings.Contains(prompt, removed) {
			t.Fatalf("video system prompt must not direct native-tool agents to %s", removed)
		}
	}
}

func TestVideoPromptHasProductIdentityAndCurrentTime(t *testing.T) {
	zone := time.FixedZone("IST", 5*60*60+30*60)
	now := time.Date(2026, time.August, 6, 20, 15, 0, 0, zone)
	prompt := videoSystemPromptAt("Identity test", now)
	for _, required := range []string{
		"You are Video Studio, an expert creative director, video producer, and editor",
		"user's current conversation is the authority",
		"Identify yourself as Video Studio",
		"Thursday, 6 August 2026 at 8:15 PM IST (UTC+05:30)",
		"Current local date and time",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("video system prompt is missing identity/time guidance %q", required)
		}
	}
	if strings.Contains(prompt, "Identity test") {
		t.Fatalf("project title leaked into system prompt: %q", prompt)
	}
	if strings.Contains(prompt, "You are Claude Code") || strings.Contains(prompt, "Video Studio coding agent") {
		t.Fatalf("video system prompt exposes implementation identity")
	}
}

func TestVideoPromptChoosesExistingExecutionTools(t *testing.T) {
	prompt := videoSystemPrompt("Routing test")
	for _, required := range []string{
		"Skills are the default production path",
		"Do not start a full workflow merely because the request is multi-stage",
		"Work directly in chat by default",
		"This includes new productions, revisions, and ground-up reconcepts",
		"does not create cinematic or AI-generated footage",
		"infographic",
		"quality",
		"Product-control tools are schema-on-demand",
		"get_api_spec",
		"query_step",
		"send_step_message",
		"stop_step",
		"stop_all_executions",
		`{"route": "quality"}`,
		"QA is mandatory after every render",
		"qa_report_path",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("video system prompt is missing orchestration guidance %q", required)
		}
	}
}

func TestQualityReportGateAcceptsPassingEvidenceForExactVideo(t *testing.T) {
	workspace := t.TempDir()
	videoPath := "outputs/final.mp4"
	if err := os.MkdirAll(filepath.Join(workspace, "outputs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "work", "qa", "final"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(videoPath)), []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	contactSheet := "work/qa/final/qa-contact-sheet.jpg"
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(contactSheet)), []byte("contact-sheet"), 0600); err != nil {
		t.Fatal(err)
	}
	frames := make([]map[string]interface{}, 0, 4)
	for i := 0; i < 4; i++ {
		path := filepath.ToSlash(filepath.Join("work", "qa", "final", "frame-"+string(rune('1'+i))+".jpg"))
		if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte("frame"), 0600); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, map[string]interface{}{"timestamp_seconds": float64(i), "path": path})
	}
	checks := map[string]interface{}{}
	for _, name := range []string{"technical", "visual", "audio", "content", "captions", "promise"} {
		checks[name] = map[string]interface{}{"status": "pass", "evidence": []string{"inspected"}}
	}
	report := map[string]interface{}{
		"schema_version": 1, "candidate_path": videoPath, "contact_sheet_path": contactSheet,
		"verdict": "pass", "ready_to_present": true, "checks": checks,
		"sampled_frames": frames, "issues": []string{}, "recommended_action": "present",
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := "work/qa/final/quality-report.json"
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(reportPath)), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateQualityReport(workspace, videoPath, reportPath, "Final video"); err != nil {
		t.Fatalf("passing QA report was rejected: %v", err)
	}

	report["candidate_path"] = "outputs/different.mp4"
	data, _ = json.Marshal(report)
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(reportPath)), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateQualityReport(workspace, videoPath, reportPath, "Final video"); err == nil || !strings.Contains(err.Error(), "different video") {
		t.Fatalf("mismatched QA report error = %v", err)
	}
}

func TestQualityReportGateResolvesWorkflowExecutionRelativeEvidence(t *testing.T) {
	workspace := t.TempDir()
	executionRoot := filepath.Join("runs", "iteration-0", "airbnb", "execution")
	videoPath := filepath.ToSlash(filepath.Join(executionRoot, "infographic-render", "infographic.mp4"))
	reportPath := filepath.ToSlash(filepath.Join(executionRoot, "infographic-check", "quality-report.json"))
	contactPath := filepath.ToSlash(filepath.Join(executionRoot, "infographic-check", "qa-contact-sheet.jpg"))
	for _, path := range []string{videoPath, contactPath} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(workspace, filepath.FromSlash(path))), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte("evidence"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	frames := make([]map[string]interface{}, 0, 4)
	for i := 0; i < 4; i++ {
		shortPath := filepath.ToSlash(filepath.Join("infographic-check", "frames", fmt.Sprintf("frame-%d.jpg", i+1)))
		fullPath := filepath.Join(workspace, filepath.FromSlash(executionRoot), filepath.FromSlash(shortPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("frame"), 0600); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, map[string]interface{}{"timestamp_seconds": float64(i), "path": shortPath})
	}
	checks := map[string]interface{}{}
	for _, name := range []string{"technical", "visual", "audio", "content", "captions", "promise"} {
		checks[name] = map[string]interface{}{"status": "pass", "evidence": []string{"inspected"}}
	}
	report := map[string]interface{}{
		"schema_version":     1,
		"candidate_path":     "infographic-render/infographic.mp4",
		"contact_sheet_path": "infographic-check/qa-contact-sheet.jpg",
		"verdict":            "pass", "ready_to_present": true, "checks": checks,
		"sampled_frames": frames, "recommended_action": "present",
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(reportPath)), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateQualityReport(workspace, videoPath, reportPath, "Final video"); err != nil {
		t.Fatalf("workflow-relative evidence was rejected: %v", err)
	}
}

func TestProductExplainerPipelineNameAndLegacyAlias(t *testing.T) {
	if infographicPipeline.Name != "Product explainer / infographic" {
		t.Fatalf("infographic pipeline name = %q", infographicPipeline.Name)
	}
	if got := PipelineByName("Product infographic"); got != infographicPipeline {
		t.Fatalf("legacy infographic name resolved to %#v", got)
	}
}

func TestVideoShellUsesAgentWorksAccessModel(t *testing.T) {
	iso := videoShellIsolator(t.TempDir())
	if iso.StrictAllowlist {
		t.Fatal("Video Studio shell unexpectedly uses SparkQuill-style strict isolation")
	}
	if len(iso.WritePaths) != 1 || iso.WritePaths[0] != "." {
		t.Fatalf("Video Studio project write paths = %#v", iso.WritePaths)
	}
	if len(iso.BlockedWritePaths) != 2 {
		t.Fatalf("Video Studio blocked write paths = %#v", iso.BlockedWritePaths)
	}
}

func TestVideoShellToolEmitsSafeActivity(t *testing.T) {
	workspace := t.TempDir()
	var events []AgentEvent
	tool := videoShellTool(workspace, nil, func(event AgentEvent) { events = append(events, event) })
	result, err := tool.Handler(context.Background(), map[string]interface{}{"command": "printf hello"})
	if err != nil || strings.TrimSpace(result) != "hello" {
		t.Fatalf("shell result = %q, err = %v", result, err)
	}
	if len(events) != 2 || events[0].Status != "running" || events[1].Status != "completed" {
		t.Fatalf("tool events = %#v", events)
	}
	if events[0].ToolCallID == "" || events[0].ToolCallID != events[1].ToolCallID {
		t.Fatalf("tool call IDs = %q and %q", events[0].ToolCallID, events[1].ToolCallID)
	}
	if events[0].Text != "" || events[1].Text != "" {
		t.Fatalf("tool events exposed command text: %#v", events)
	}
	if _, err := os.Stat(filepath.Join(workspace, "hello")); !os.IsNotExist(err) {
		t.Fatalf("unexpected workspace output: %v", err)
	}
}
