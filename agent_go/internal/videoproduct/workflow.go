package videoproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	stepworkflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	"github.com/manishiitg/mcpagent/llm"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// The active pipeline's user-facing name and steps. Both come from the pipeline
// definition (see pipelines.go) so adding a workflow is a definition, not a code
// change. Routing will select the pipeline per project; until then every project
// uses the default.
var cinematicWorkflowName = DefaultPipeline().Name

var cinematicSteps = DefaultPipeline().Steps()

type collectedWorkflowTool struct {
	name, description, category string
	params                      map[string]interface{}
	handler                     func(context.Context, map[string]interface{}) (string, error)
}

type workflowToolCollector struct{ tools []collectedWorkflowTool }

func (c *workflowToolCollector) RegisterCustomTool(name, description string, params map[string]interface{}, handler func(context.Context, map[string]interface{}) (string, error), category string) error {
	c.tools = append(c.tools, collectedWorkflowTool{name: name, description: description, params: params, handler: handler, category: category})
	return nil
}

func (c *workflowToolCollector) RegisterCustomToolWithTimeout(name, description string, params map[string]interface{}, handler func(context.Context, map[string]interface{}) (string, error), _ time.Duration, category string) error {
	return c.RegisterCustomTool(name, description, params, handler, category)
}

func (*workflowToolCollector) AttachedSkills() []*llmtypes.Skill { return nil }

type projectWorkflowSession struct {
	session  *stepworkflow.WorkshopChatSession
	notifier *videoWorkflowNotifier
	env      map[string]string
}

type WorkflowService struct {
	store           *Store
	workspaceAPIURL string
	mcpConfigPath   string
	logger          loggerv2.Logger
	mu              sync.Mutex
	sessions        map[string]*projectWorkflowSession
	autoNotify      func(workflowAutoNotification)
}

func NewWorkflowService(store *Store, workspaceAPIURL, mcpConfigPath string) *WorkflowService {
	return &WorkflowService{store: store, workspaceAPIURL: workspaceAPIURL, mcpConfigPath: mcpConfigPath, logger: loggerv2.NewDefault(), sessions: map[string]*projectWorkflowSession{}}
}

// SetAutoNotificationHandler connects AgentWorks workflow completions back to
// the product's persistent main-agent conversation. The handler is installed by
// the video server before any project session is created, but updating existing
// notifiers as well keeps this safe for tests and future runtime reconfiguration.
func (s *WorkflowService) SetAutoNotificationHandler(handler func(workflowAutoNotification)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoNotify = handler
	for _, state := range s.sessions {
		state.notifier.setAutoNotificationHandler(handler)
	}
}

func (s *WorkflowService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.sessions {
		state.session.Close()
	}
	s.sessions = map[string]*projectWorkflowSession{}
}

func copyCinematicSteps() []WorkflowStep {
	out := make([]WorkflowStep, len(cinematicSteps))
	copy(out, cinematicSteps)
	return out
}

func workflowActivityContext(workflowName string, steps []WorkflowStep, toolName string, args map[string]interface{}) (string, string) {
	if toolName == "run_full_workflow" {
		// A full run is shown as just the workflow name ("Cinematic video").
		// Only a single stage gets the arrow form ("Cinematic video → Research").
		return workflowName, ""
	}
	stepID, _ := args["step_id"].(string)
	stepID = strings.TrimSpace(stepID)
	for _, step := range steps {
		if step.ID == stepID {
			return workflowName, step.Title
		}
	}
	return workflowName, stepID
}

func workflowConversationInput(history []Message) string {
	const maxMessages = 12
	const maxRunes = 16000
	entries := make([]string, 0, maxMessages)
	remaining := maxRunes
	for index := len(history) - 1; index >= 0 && len(entries) < maxMessages && remaining > 0; index-- {
		body := strings.TrimSpace(history[index].Body)
		if body == "" || (history[index].Role != "user" && history[index].Role != "assistant") {
			continue
		}
		role := "User"
		if history[index].Role == "assistant" {
			role = "Assistant"
		}
		entryRunes := []rune(role + ": " + body)
		if len(entryRunes) > remaining {
			if len(entries) == 0 {
				entries = append(entries, string(entryRunes[:remaining]))
			}
			break
		}
		entries = append(entries, string(entryRunes))
		remaining -= len(entryRunes)
	}
	if len(entries) == 0 {
		return ""
	}
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	return "Recent project conversation (the latest User entry is the current request; earlier entries are context and approvals):\n\n" + strings.Join(entries, "\n\n")
}

func applyWorkflowHumanInput(pipeline *Pipeline, toolName string, args map[string]interface{}, history []Message) {
	input := workflowConversationInput(history)
	if input == "" {
		return
	}
	if toolName == "execute_step" {
		if existing, _ := args["human_input"].(string); strings.TrimSpace(existing) == "" {
			args["human_input"] = input
		}
		return
	}
	if toolName != "run_full_workflow" || len(pipeline.Stages) == 0 {
		return
	}
	firstStepID := pipeline.Stages[0].ID
	switch existing := args["human_inputs"].(type) {
	case map[string]interface{}:
		if value, _ := existing[firstStepID].(string); strings.TrimSpace(value) == "" {
			existing[firstStepID] = input
		}
	case map[string]string:
		if strings.TrimSpace(existing[firstStepID]) == "" {
			existing[firstStepID] = input
		}
	default:
		args["human_inputs"] = map[string]interface{}{firstStepID: input}
	}
}

func workflowRelativePath(projectID string) string {
	return filepath.ToSlash(filepath.Join("projects", projectID))
}

func (s *WorkflowService) EnsureProject(project Project) error {
	projectID, title := project.ID, project.Title
	root := s.store.ProjectDir(projectID)
	for _, dir := range []string{"planning", "variables", "soul"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return err
		}
	}
	// The pipeline definition in code is the source of truth for the plan, the
	// step config and the skills each stage gets, so these are rewritten every
	// time a project is opened. Written once, they would freeze at whatever the
	// definition looked like on the day the project was created: a project made
	// before stage skills existed would never get them, and a later correction
	// to a stage's instructions — including the rule that Assets must not spend
	// without approval — would silently never reach it.
	//
	// Safe to overwrite here because this product exposes only execute_step,
	// query_step and run_full_workflow. The plan-editing tools that write
	// step_config.json at runtime are not part of Video Studio, so there is no
	// engine-authored state in these two files to preserve.
	generated := map[string]interface{}{
		"planning/plan.json":        planForAll(pipelineRegistry),
		"planning/step_config.json": stepConfigForAll(pipelineRegistry),
	}
	for name, value := range generated {
		if err := writeProjectJSON(root, name, value); err != nil {
			return err
		}
	}

	// Created once and then left alone: workflow.json carries creation
	// timestamps, and variables.json is runtime state rather than definition.
	seeded := map[string]interface{}{
		"workflow.json":            cinematicWorkflowManifest(projectID, title),
		"variables/variables.json": map[string]interface{}{"variables": []interface{}{}, "groups": []interface{}{}, "extraction_date": time.Now().Format(time.RFC3339)},
	}
	for name, value := range seeded {
		path := filepath.Join(root, filepath.FromSlash(name))
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := writeProjectJSON(root, name, value); err != nil {
			return err
		}
	}
	soulPath := filepath.Join(root, "soul", "soul.md")
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		body := "# " + title + "\n\n## Objective\nCreate strong, source-grounded videos through a clear, approval-led production process.\n\n## Success Criteria\nThe story is coherent, visual and audio quality are checked, and every final export is reproducible from project assets and workflow artifacts.\n\n## Constraints\nDo not publish. Do not invent facts. Do not generate expensive media until the user has approved the creative direction.\n"
		if err := os.WriteFile(soulPath, []byte(body), 0600); err != nil {
			return err
		}
	}
	return nil
}

// writeProjectJSON writes one JSON file under the project root, creating the
// parent folder if the definition introduced a new location.
func writeProjectJSON(root, name string, value interface{}) error {
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func cinematicWorkflowManifest(projectID, title string) map[string]interface{} {
	return map[string]interface{}{
		"schema_version": 1, "id": "video-" + projectID, "version": "1.0.0", "label": title,
		"capabilities": map[string]interface{}{
			"selected_servers": []string{}, "selected_tools": []string{}, "selected_skills": []string{}, "selected_secrets": []string{},
			"browser_mode": "none", "use_code_execution_mode": true,
			"llm_config": map[string]interface{}{"schema_version": 2, "mode": "provider_profile", "provider": "claude-code"},
		},
		"execution_defaults": map[string]interface{}{"always_use_same_run": true, "workshop_mode": "run"},
		"schedules":          []interface{}{}, "created_at": time.Now().UTC().Format(time.RFC3339), "updated_at": time.Now().UTC().Format(time.RFC3339),
	}
}

const routeStepID = "route"

func cinematicPlan() map[string]interface{} { return planForAll(pipelineRegistry) }

// planForAll renders every pipeline into ONE plan: a deterministic routing step,
// then each pipeline's stages as its own branch.
//
// One plan rather than one per pipeline is what AgentWorks' routing expects, and
// it means the branch is chosen when a run starts instead of having to be known
// when the project's files are first written — so nothing has to be stored on the
// project to remember it.
//
// There is deliberately no step that "decides" the route. When the user has
// already said what they want, the chat agent passes route_selections to
// run_full_workflow and the engine seeds the choice; otherwise default_route_id
// applies. Spending an LLM stage to re-derive a choice the user already stated is
// what AgentWorks' own guidance warns against.
func planForAll(pipelines []*Pipeline) map[string]interface{} {
	steps := make([]map[string]interface{}, 0, 1+len(pipelines)*8)

	routes := make([]map[string]interface{}, 0, len(pipelines))
	for _, pipeline := range pipelines {
		routes = append(routes, map[string]interface{}{
			"route_id": pipeline.ID, "route_name": pipeline.Name,
			"condition":    pipeline.WhenToUse,
			"next_step_id": pipeline.Stages[0].ID,
		})
	}

	// Deterministic by contract: the engine rejects a routing step that carries a
	// description, because routing never runs an agent.
	steps = append(steps, map[string]interface{}{
		"type": "routing", "id": routeStepID, "title": "Route",
		"routing_question": "Which pipeline does this brief call for?",
		"routes":           routes,
		"default_route_id": pipelines[0].ID,
	})

	for _, pipeline := range pipelines {
		for i, stage := range pipeline.Stages {
			deps := []string{}
			if i > 0 {
				deps = []string{pipeline.Stages[i-1].Output}
			}
			step := map[string]interface{}{
				"type": "regular", "id": stage.ID, "title": stage.Title, "description": stage.Description,
				"context_dependencies": deps, "context_output": stage.Output, "has_loop": false,
				"validation_schema": map[string]interface{}{"files": []map[string]interface{}{{"file_name": stage.Output, "must_exist": true}}},
			}
			// Without this the last stage of one branch would fall straight into
			// the first stage of the next branch in the step list.
			if i == len(pipeline.Stages)-1 {
				step["next_step_id"] = "end"
			}
			steps = append(steps, step)
		}
	}

	return map[string]interface{}{"steps": steps}
}

func cinematicStepConfig() map[string]interface{} { return stepConfigForAll(pipelineRegistry) }

func baseStageAgentConfig() map[string]interface{} {
	return map[string]interface{}{
		"execution_llm":       map[string]interface{}{"provider": "claude-code", "model_id": DefaultClaudeModel},
		"execution_max_turns": 100, "use_code_execution_mode": true, "declared_execution_mode": "agentic",
		"additional_read_paths": []string{"uploads"}, "learnings_access": "none", "knowledgebase_access": "none", "db_access": "none",
	}
}

// stepConfigForAll renders per-stage agent configuration for every pipeline plus
// the shared intake step. Learnings, knowledge base and workflow DB stay off for
// every stage: this product keeps its state in its own database, and leaving them
// on had stage agents probing facilities that are not there.
//
// The routing step gets no entry — it runs deterministically inside the engine
// and never launches an agent.
func stepConfigForAll(pipelines []*Pipeline) map[string]interface{} {
	steps := make([]map[string]interface{}, 0, len(pipelines)*8)
	for _, pipeline := range pipelines {
		for _, stage := range pipeline.Stages {
			agentConfig := baseStageAgentConfig()
			if len(stage.Skills) > 0 {
				// enabled_skills is the STEP-level selector and takes skill folder
				// names. selected_skills is the workshop-level preset and is ignored
				// here, so using it would silently give the stage no skills at all.
				agentConfig["enabled_skills"] = append([]string{}, stage.Skills...)
			}
			steps = append(steps, map[string]interface{}{"id": stage.ID, "title": stage.Title, "agent_configs": agentConfig})
		}
	}
	return map[string]interface{}{"steps": steps}
}

func secretMap(entries []string) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if index := strings.IndexByte(entry, '='); index > 0 {
			out[entry[:index]] = entry[index+1:]
		}
	}
	return out
}

func secretEntries(values map[string]string) []orchestrator.SecretEntry {
	out := make([]orchestrator.SecretEntry, 0, len(values))
	for name, value := range values {
		out = append(out, orchestrator.SecretEntry{Name: name, Value: value})
	}
	return out
}

func (s *WorkflowService) projectSession(ctx ProjectContext) (*projectWorkflowSession, error) {
	if err := s.EnsureProject(ctx.Project); err != nil {
		return nil, err
	}
	pipeline := DefaultPipeline()
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.sessions[ctx.Project.ID]; state != nil {
		for key, value := range secretMap(ctx.SecretEnv) {
			state.env["SECRET_"+key] = value
		}
		// The bridge is normally warmed before this session was ever created
		// (see agentsession.WarmSharedBridge), but re-derive the MCP bridge
		// vars from the live process env on every reuse anyway: it's cheap,
		// and it self-heals a session whose token snapshot predates the
		// bridge starting instead of leaving every workflow step permanently
		// unauthenticated for the life of the process.
		if token := os.Getenv("MCP_API_TOKEN"); token != "" {
			state.env["MCP_API_TOKEN"] = token
		}
		if url := os.Getenv("MCP_API_URL"); url != "" {
			state.env["MCP_API_URL"] = url
		}
		common.PopulateMCPBridgeShortEnv(state.env)
		return state, nil
	}
	sessionID := "video-workflow-" + ctx.Project.ID
	tools, executors, categories, env := virtualtools.CreateWorkspaceToolRegistryUntyped(virtualtools.WorkspaceToolRegistryConfig{
		WorkspaceAPIURL: s.workspaceAPIURL, UserID: ctx.UserID, SessionID: sessionID, ExtraEnvVars: secretMap(ctx.SecretEnv),
	})
	claude := &stepworkflow.AgentLLMConfig{Provider: string(llm.ProviderClaudeCode), ModelID: DefaultClaudeModel}
	cfg := &stepworkflow.WorkshopConfig{
		WorkspacePath: workflowRelativePath(ctx.Project.ID), MCPConfigPath: s.mcpConfigPath,
		UseCodeExecutionMode: true, CustomTools: tools, CustomToolExecutors: executors, ToolCategories: categories,
		LLMConfig:      &orchestrator.LLMConfig{Primary: orchestrator.LLMModel{Provider: string(llm.ProviderClaudeCode), ModelID: DefaultClaudeModel}},
		PresetPhaseLLM: claude, PresetMaintenanceLLM: claude, UseKnowledgebase: false, LLMAllocationMode: "manual",
		Logger: s.logger, SessionID: sessionID, Secrets: secretEntries(secretMap(ctx.SecretEnv)), WorkspaceEnvRef: env,
	}
	workshop, err := stepworkflow.NewWorkshopChatSession(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	notifier := newVideoWorkflowNotifier(s.store, ctx.Project.ID, pipeline, s.autoNotify)
	workshop.SetWorkshopExecutionNotifier(notifier)
	state := &projectWorkflowSession{session: workshop, notifier: notifier, env: env}
	s.sessions[ctx.Project.ID] = state
	return state, nil
}

var executionIDPattern = regexp.MustCompile(`(?i)execution_id[:= ]+["'` + "`" + `]?([a-z0-9._:-]+)`)

// pipelineFromArgs works out which pipeline a workflow call is really for, so
// the run and its activity are labelled with the branch that will actually
// execute rather than always with the default one.
//
// run_full_workflow carries route_selections; execute_step names a single stage,
// and stage ids are unique across the plan, so the owning pipeline follows from
// the id. Falls back to the default when neither says.
func pipelineFromArgs(args map[string]interface{}) *Pipeline {
	if raw, ok := args["route_selections"]; ok {
		var selected string
		switch typed := raw.(type) {
		case map[string]interface{}:
			selected, _ = typed[routeStepID].(string)
		case map[string]string:
			selected = typed[routeStepID]
		}
		if selected = strings.TrimSpace(selected); selected != "" {
			for _, pipeline := range pipelineRegistry {
				if pipeline.ID == selected {
					return pipeline
				}
			}
		}
	}
	if stepID, _ := args["step_id"].(string); strings.TrimSpace(stepID) != "" {
		if owner := pipelineForStage(strings.TrimSpace(stepID)); owner != nil {
			return owner
		}
	}
	return DefaultPipeline()
}

func (s *WorkflowService) Tools(ctx ProjectContext, emit func(AgentEvent)) ([]agentsession.Tool, error) {
	state, err := s.projectSession(ctx)
	if err != nil {
		return nil, err
	}
	collector := &workflowToolCollector{}
	stepworkflow.RegisterWorkshopChatTools(collector, state.session, s.logger)
	stepworkflow.RegisterRunFullWorkflowTool(collector, state.session, s.logger)
	allowed := map[string]bool{"execute_step": true, "query_step": true, "run_full_workflow": true}
	result := make([]agentsession.Tool, 0, 3)
	for _, definition := range collector.tools {
		if !allowed[definition.name] {
			continue
		}
		def := definition
		result = append(result, agentsession.Tool{Name: def.name, Description: def.description, Params: def.params, Category: "video_workflow", Handler: func(callCtx context.Context, args map[string]interface{}) (reply string, callErr error) {
			// Resolve the branch the caller actually selected. The routing step
			// decides it during execution, but the caller already told us in the
			// same call, so the label can be right immediately instead of always
			// naming the default pipeline.
			activePipeline := pipelineFromArgs(args)
			applyWorkflowHumanInput(activePipeline, def.name, args, ctx.History)
			started := time.Now()
			callID := fmt.Sprintf("workflow-%d", started.UnixNano())
			workflowName, stepName := workflowActivityContext(activePipeline.Name, activePipeline.Steps(), def.name, args)
			if emit != nil {
				emit(AgentEvent{Type: "tool", Tool: def.name, Workflow: workflowName, Step: stepName, ToolCallID: callID, Status: "running"})
			}
			// execute_step and run_full_workflow only DISPATCH; the stages then run
			// for minutes in the background. Reporting how long the dispatch took
			// put "11 ms" next to a workflow's name, which reads as the video
			// having been made in 11ms. Progress for those comes from stage state,
			// not from this tool call.
			backgrounded := def.name == "execute_step" || def.name == "run_full_workflow"
			defer func() {
				status := "completed"
				if callErr != nil {
					status = "failed"
				} else if backgrounded {
					status = "started"
				}
				event := AgentEvent{Type: "tool", Tool: def.name, Workflow: workflowName, Step: stepName, ToolCallID: callID, Status: status}
				if !backgrounded || callErr != nil {
					event.DurationMS = time.Since(started).Milliseconds()
				}
				if emit != nil {
					emit(event)
				}
			}()
			var run WorkflowRun
			if def.name == "execute_step" || def.name == "run_full_workflow" {
				group, _ := args["group_name"].(string)
				group = strings.TrimSpace(group)
				if group == "" {
					return "group_name is required. Use a short video name such as video-launch-film.", nil
				}
				if err := s.ensureGroup(ctx.Project.ID, group); err != nil {
					return "Could not prepare this video run: " + err.Error(), nil
				}
				// Seeded with every pipeline's steps: the routing step picks the
				// branch during execution, so the run row cannot know in advance
				// which stages will report, and a status for a step with no row
				// is silently discarded.
				run, callErr = s.store.BeginWorkflowRun(ctx.Project.ID, activePipeline.Name, group, AllPipelineSteps())
				if callErr != nil {
					return "", callErr
				}
				state.notifier.Prepare(run.ID, def.name, group, ctx.UserID)
				if def.name == "run_full_workflow" {
					if _, exists := args["disable_eval"]; !exists {
						args["disable_eval"] = true
					}
				}
			}
			reply, callErr = def.handler(callCtx, args)
			if run.ID != "" {
				if match := executionIDPattern.FindStringSubmatch(reply); len(match) == 2 {
					_ = s.store.SetWorkflowExecution(run.ID, match[1])
				} else if callErr != nil || strings.Contains(strings.ToLower(reply), "failed") {
					_ = s.store.FinishWorkflowRun(run.ID, "failed")
				}
			}
			return reply, callErr
		}})
	}
	return result, nil
}

func (s *WorkflowService) ensureGroup(projectID, groupName string) error {
	path := filepath.Join(s.store.ProjectDir(projectID), "variables", "variables.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest stepworkflow.VariablesManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	for _, group := range manifest.Groups {
		if group.Name == groupName {
			return nil
		}
	}
	manifest.Groups = append(manifest.Groups, stepworkflow.VariableGroup{Name: groupName, Values: map[string]string{}, Enabled: true})
	manifest.ExtractionDate = time.Now().Format(time.RFC3339)
	updated, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(updated, '\n'), 0600)
}

type workflowLaunch struct{ runID, mode, group, userID string }

// workflowAutoNotification is the product-local equivalent of AgentWorks'
// synthetic [AUTO-NOTIFICATION] turn. It deliberately stays out of the visible
// message store: the resumed main agent consumes it and writes the user-facing
// continuation as its normal assistant reply.
type workflowAutoNotification struct {
	ProjectID   string
	UserID      string
	RunID       string
	FinalStatus string
	Message     string
}

type videoWorkflowNotifier struct {
	store      *Store
	projectID  string
	pipeline   *Pipeline
	mu         sync.Mutex
	pending    *workflowLaunch
	execRuns   map[string]string
	runModes   map[string]string
	runGroups  map[string]string
	runUsers   map[string]string
	completed  map[string]bool
	autoNotify func(workflowAutoNotification)
}

func newVideoWorkflowNotifier(store *Store, projectID string, pipeline *Pipeline, autoNotify func(workflowAutoNotification)) *videoWorkflowNotifier {
	return &videoWorkflowNotifier{store: store, projectID: projectID, pipeline: pipeline, execRuns: map[string]string{}, runModes: map[string]string{}, runGroups: map[string]string{}, runUsers: map[string]string{}, completed: map[string]bool{}, autoNotify: autoNotify}
}

func (n *videoWorkflowNotifier) setAutoNotificationHandler(handler func(workflowAutoNotification)) {
	n.mu.Lock()
	n.autoNotify = handler
	n.mu.Unlock()
}

func (n *videoWorkflowNotifier) Prepare(runID, mode, group, userID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending = &workflowLaunch{runID: runID, mode: mode, group: group, userID: userID}
	n.runModes[runID] = mode
	n.runGroups[runID] = group
	n.runUsers[runID] = userID
}

// stageIDFromName resolves a step name across EVERY pipeline, not just the
// default one. A run's branch is chosen by the routing step at execution time,
// so the stage reporting status may belong to a pipeline this run was not
// labelled with; matching only the default pipeline silently dropped every
// status update for the other branch.
//
// Titles are matched only within the pipeline whose stage ids also match, since
// branches deliberately reuse titles ("Research", "Quality check"). Ids are
// unique across the plan, so they are tried first.
func stageIDFromName(_ *Pipeline, name string) string {
	base := strings.ToLower(strings.TrimSpace(strings.Split(name, "[")[0]))
	base = strings.TrimPrefix(base, "step: ")
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			if base == strings.ToLower(stage.ID) {
				return stage.ID
			}
		}
	}
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			if base == strings.ToLower(stage.Title) {
				return stage.ID
			}
		}
	}
	return ""
}

func (n *videoWorkflowNotifier) OnExecutionStart(start stepworkflow.WorkshopExecutionStart) {
	n.mu.Lock()
	runID := n.execRuns[start.ParentExecutionID]
	if runID == "" && n.pending != nil {
		runID = n.pending.runID
		n.pending = nil
	}
	if runID != "" {
		n.execRuns[start.ID] = runID
	}
	n.mu.Unlock()
	if runID != "" {
		_ = n.store.SetWorkflowExecution(runID, start.ID)
		if stepID := stageIDFromName(n.pipeline, start.Name); stepID != "" {
			_ = n.store.SetWorkflowStep(runID, stepID, "running")
		}
	}
}

func (n *videoWorkflowNotifier) OnExecutionComplete(execID, name, result string, meta map[string]string, execErr error) {
	n.mu.Lock()
	runID := n.execRuns[execID]
	mode := n.runModes[runID]
	group := n.runGroups[runID]
	userID := n.runUsers[runID]
	autoNotify := n.autoNotify
	alreadyCompleted := n.completed[execID]
	if !alreadyCompleted {
		n.completed[execID] = true
	}
	n.mu.Unlock()
	if runID == "" || alreadyCompleted {
		return
	}
	status := "completed"
	if execErr != nil {
		status = "failed"
	}
	stepID := ""
	if meta != nil {
		stepID = stageIDFromName(n.pipeline, meta["step_name"])
	}
	if stepID == "" {
		stepID = stageIDFromName(n.pipeline, name)
	}
	if stepID != "" {
		_ = n.store.SetWorkflowStep(runID, stepID, status)
	}
	isFullCompletion := meta != nil && meta["execution_type"] == "full-workflow"
	shouldResumeAgent := (mode == "execute_step" && stepID != "") || (mode == "run_full_workflow" && isFullCompletion)
	if !shouldResumeAgent {
		return
	}
	finalStatus := status
	if mode == "execute_step" && status == "completed" {
		finalStatus = "ready"
	}
	label := n.pipeline.Name + " workflow"
	if stepID != "" {
		label = stepID
		for _, stage := range n.pipeline.Stages {
			if stage.ID == stepID {
				label = stage.Title
				break
			}
		}
	}
	synthetic := buildWorkflowNotification(workflowNotificationFacts{
		Label: label, Group: group, Status: status, StepID: stepID,
		RunRoot: workflowRunRoot(n.projectID, meta), Result: result,
	})
	if autoNotify != nil {
		autoNotify(workflowAutoNotification{ProjectID: n.projectID, UserID: userID, RunID: runID, FinalStatus: finalStatus, Message: synthetic})
		return
	}

	// Compatibility fallback for hosts that have not connected a persistent
	// main-agent continuation handler.
	_ = n.store.FinishWorkflowRun(runID, finalStatus)
	message := fmt.Sprintf("%s finished for %s.", label, group)
	if status == "failed" {
		message = fmt.Sprintf("%s needs attention for %s.", label, group)
	}
	_, _ = n.store.AddMessage(n.projectID, "", "assistant", "Studio agent", message)
}

type workflowNotificationFacts struct {
	Label   string // user-facing stage or workflow name
	Group   string
	Status  string // "completed" | "failed" — authoritative terminal status
	StepID  string
	RunRoot string // workspace-relative run root, may be empty
	Result  string // free-text summary from the workflow; NOT authoritative
}

// workflowRunRoot returns the workspace-relative folder that actually holds a
// run's artifacts. Steps write to <project>/runs/<run_folder>/execution/<step>/;
// the legacy work/ and outputs/ folders are scaffolded but never written by the
// workflow, so a reply sourced from them reports "nothing was produced" even on
// a successful run.
func workflowRunRoot(projectID string, meta map[string]string) string {
	if meta == nil {
		return ""
	}
	runFolder := strings.TrimSpace(meta["run_folder"])
	if runFolder == "" {
		return ""
	}
	return workflowRelativePath(projectID) + "/runs/" + runFolder
}

// buildWorkflowNotification renders the synthetic turn that resumes the main
// agent after a workflow finishes.
//
// The terminal status is stated as a fact and the agent is told not to re-derive
// it. Previously the status was only a word in a sentence followed by an
// unstructured Result blob, and when a run failed after some steps succeeded the
// blob could still read "completed successfully" — leaving the agent to
// reconcile "failed" against "completed successfully" and produce replies that
// asserted both. The free-text summary is kept, but explicitly demoted to
// context so it can never override the real outcome.
func buildWorkflowNotification(facts workflowNotificationFacts) string {
	var b strings.Builder
	failed := facts.Status == "failed"
	outcome := "COMPLETED"
	if failed {
		outcome = "DID NOT COMPLETE"
	}
	fmt.Fprintf(&b, "[AUTO-NOTIFICATION] These are the authoritative facts about the run that just ended. Trust them over any wording in the summary below.\n")
	fmt.Fprintf(&b, "- Stage: %s\n- Project group: %s\n- Outcome: %s\n", facts.Label, facts.Group, outcome)
	if failed && facts.StepID != "" {
		fmt.Fprintf(&b, "- Failed at stage: %s\n", facts.StepID)
	}
	if facts.RunRoot != "" {
		fmt.Fprintf(&b, "- Artifacts for this run live ONLY under: %s/execution/<stage>/\n", facts.RunRoot)
	}
	if summary := strings.TrimSpace(facts.Result); summary != "" {
		fmt.Fprintf(&b, "\nWorkflow summary (context only — it may be incomplete or contradict the outcome above; if it disagrees, the outcome above wins):\n%s\n", summary)
	}
	b.WriteString("\nContinue the same project conversation now. ")
	if failed {
		b.WriteString("Tell the user plainly that this did not finish, name the stage it stopped at, and give one concrete next action. Do not describe the run as successful, do not claim any video or media file exists, and do not invent partial success. ")
	} else {
		b.WriteString("Report what is now ready and what the natural next step is. ")
	}
	if facts.RunRoot != "" {
		b.WriteString("If you need evidence, read files under the run folder above. Never treat work/ or outputs/ as the source of truth for workflow artifacts — they are always empty. ")
	}
	b.WriteString("Reply in user-friendly product language; do not mention notification internals, AgentWorks, learnings, knowledge-base maintenance, locks, or other framework details. Do not repeat query_step for this completed execution. Do not start an expensive or media-generating stage unless the user's existing approval clearly authorizes it.")
	return b.String()
}

func (n *videoWorkflowNotifier) OnExecutionTerminated(execID, name string) {
	n.mu.Lock()
	runID := n.execRuns[execID]
	n.mu.Unlock()
	if runID == "" {
		return
	}
	if stepID := stageIDFromName(n.pipeline, name); stepID != "" {
		_ = n.store.SetWorkflowStep(runID, stepID, "cancelled")
	}
	_ = n.store.FinishWorkflowRun(runID, "cancelled")
}
