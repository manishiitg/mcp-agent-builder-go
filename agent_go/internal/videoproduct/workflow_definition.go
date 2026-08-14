package videoproduct

import "time"

const routeStepID = "route"

func videoStudioWorkflowManifest(projectID, title string) map[string]interface{} {
	product := mustVideoStudioManifest()
	return map[string]interface{}{"schema_version": 1, "id": "video-" + projectID, "version": "1.0.0", "label": title, "capabilities": map[string]interface{}{"selected_servers": []string{}, "selected_tools": []string{}, "selected_skills": append([]string(nil), product.Workflows.SelectedSkills...), "selected_secrets": []string{}, "browser_mode": product.Workflows.BrowserMode, "use_code_execution_mode": true, "llm_config": videoWorkflowLLMConfig()}, "execution_defaults": map[string]interface{}{"always_use_same_run": true, "workshop_mode": "run"}, "schedules": []interface{}{}, "created_at": time.Now().UTC().Format(time.RFC3339), "updated_at": time.Now().UTC().Format(time.RFC3339)}
}
func videoAgentLLMConfig() map[string]interface{} {
	return map[string]interface{}{"provider": "claude-code", "model_id": DefaultClaudeModel}
}
func videoWorkflowLLMConfig() map[string]interface{} {
	return map[string]interface{}{"schema_version": 2, "mode": "explicit", "builder_llm": videoAgentLLMConfig(), "maintenance_llm": videoAgentLLMConfig(), "pulse_llm": videoAgentLLMConfig(), "tiered_config": map[string]interface{}{"tier_1": videoAgentLLMConfig(), "tier_2": videoAgentLLMConfig(), "tier_3": videoAgentLLMConfig()}}
}
func planForAll(pipelines []*Pipeline) map[string]interface{} {
	steps := make([]map[string]interface{}, 0, 1+len(pipelines)*8)
	routes := make([]map[string]interface{}, 0, len(pipelines))
	for _, p := range pipelines {
		routes = append(routes, map[string]interface{}{"route_id": p.ID, "route_name": p.Name, "condition": p.WhenToUse, "next_step_id": pipelineEntryStepID(p)})
	}
	steps = append(steps, map[string]interface{}{"type": "routing", "id": routeStepID, "title": "Route", "routing_question": "Which pipeline does this brief call for?", "routes": routes, "default_route_id": pipelines[0].ID})
	for _, p := range pipelines {
		block := orchestratedStageIDs(p)
		blockRoutes := make([]map[string]interface{}, 0, len(block))
		blockDeps := []string{}
		blockEmitted := false
		for i, stage := range p.Stages {
			deps := []string{}
			for prev := 0; prev < i; prev++ {
				deps = append(deps, p.Stages[prev].Output)
				deps = append(deps, p.Stages[prev].Artifacts...)
			}
			step := stageStep(p, stage, deps, i == len(p.Stages)-1)

			if block[stage.ID] {
				// Orchestrated stages become routes rather than plan steps. The
				// orchestrator is emitted at the position of the first one so the
				// plan order still reads as the production order.
				if !blockEmitted {
					blockDeps = append([]string{}, deps...)
					blockEmitted = true
				}
				delete(step, "next_step_id")
				blockRoutes = append(blockRoutes, map[string]interface{}{
					"route_id":       stage.ID,
					"route_name":     stage.Title,
					"condition":      stage.Summary,
					"sub_agent_step": step,
				})
				if stage.ID == p.Orchestrated.StageIDs[len(p.Orchestrated.StageIDs)-1] {
					// A todo_task must name its successor. The runtime would fall
					// through to the next step in array order anyway, but the plan
					// graph refuses to draw a sequential edge out of a todo_task —
					// it only follows next_step_id — so without this the
					// orchestrator renders as a dead end and the plan stops
					// describing what actually runs.
					next := "end"
					if i+1 < len(p.Stages) {
						next = p.Stages[i+1].ID
					}
					steps = append(steps, orchestratorStep(p, blockRoutes, blockDeps, next))
				}
				continue
			}
			steps = append(steps, step)
		}
	}
	return map[string]interface{}{"steps": steps}
}

// pipelineEntryStepID is what the router points at: the orchestrator when the
// pipeline opens with an orchestrated block, otherwise its first stage.
func pipelineEntryStepID(p *Pipeline) string {
	if p.Orchestrated != nil && len(p.Orchestrated.StageIDs) > 0 && p.Stages[0].ID == p.Orchestrated.StageIDs[0] {
		return p.Orchestrated.ID
	}
	return p.Stages[0].ID
}

func orchestratedStageIDs(p *Pipeline) map[string]bool {
	ids := map[string]bool{}
	if p.Orchestrated == nil {
		return ids
	}
	for _, id := range p.Orchestrated.StageIDs {
		ids[id] = true
	}
	return ids
}

// stageStep is one production stage. Always a message_sequence — every stage
// here is conversational and judgment-heavy, which is what the plan-authoring
// guidance reserves that type for.
func stageStep(p *Pipeline, stage PipelineStage, deps []string, last bool) map[string]interface{} {
	required := []map[string]interface{}{{"file_name": stage.Output, "must_exist": true}}
	for _, artifact := range stage.Artifacts {
		required = append(required, map[string]interface{}{"file_name": artifact, "must_exist": true})
	}
	step := map[string]interface{}{
		"type": "message_sequence", "id": stage.ID, "title": stage.Title,
		"description": stage.Description, "context_dependencies": deps,
		"context_output":    stage.Output,
		"items":             []map[string]interface{}{stageExecuteItem()},
		"validation_schema": map[string]interface{}{"files": required},
	}
	if last {
		step["next_step_id"] = "end"
	}
	return step
}

// orchestratorStep runs an OrchestratedBlock's stages as sub-agent routes. Its
// own validation asserts the block's output so the step cannot pass by talking
// about work its routes never did.
func orchestratorStep(p *Pipeline, blockRoutes []map[string]interface{}, deps []string, nextStepID string) map[string]interface{} {
	step := map[string]interface{}{
		"type": "todo_task", "id": p.Orchestrated.ID, "title": p.Orchestrated.Title,
		"description": p.Orchestrated.Description, "context_dependencies": deps,
		"context_output":    p.Orchestrated.Output,
		"predefined_routes": blockRoutes,
		"validation_schema": map[string]interface{}{"files": []map[string]interface{}{
			{"file_name": p.Orchestrated.Output, "must_exist": true},
		}},
		"next_step_id": nextStepID,
	}
	return step
}

// stageExecuteItem is the one turn a production stage runs. Every stage here is
// conversational and judgment-heavy — writing a brief, a storyboard, a design,
// a critique — which is exactly what the plan-authoring guidance means by
// "use message_sequence for every conversational, judgment-heavy, browser-driven
// or adaptive step, even when it needs only one message. Non-scripted regular
// steps are unsupported." (frontend/src/commands/builtin-commands.tsx).
//
// These stages were authored as `regular` and only ran because the runtime
// rewrites every non-scripted regular step into a sequence exactly like this one
// (normalizeRegularStepToMessageSequence). Declaring it makes the stored plan
// say what actually executes, so the plan UI no longer has to reconstruct it and
// a stage that later wants a second turn — a critique pass, a repair gate — can
// simply add an item instead of needing a different step type.
//
// The wording is the runtime's own synthesized message, kept verbatim so this
// change alters what the plan DECLARES, not how a stage behaves.
func stageExecuteItem() map[string]interface{} {
	return map[string]interface{}{
		"id":   "execute-and-verify",
		"type": "user_message",
		"kind": "execution",
		"message": "Complete the step described above. Re-open the produced evidence, verify the result against every stated requirement, " +
			"repair any gap you find, and finish only when the step is complete.",
	}
}

// managedSkillPolicy splits a stage's declared skills by how the product says
// each one reaches the agent. product.yaml installs twelve HyperFrames skills
// but declares `attach: [hyperframes]`; the rest are, in productdeps' own
// words, "ordinary files for progressive disclosure" — product-infographic
// routes the agent to read them by path (`skills/<name>/SKILL.md`).
//
// One field was doing both jobs. Every stage asked to ATTACH all of them,
// which failed silently before the loader became workspace-aware and would now
// succeed and load eleven specialist skills nobody asked for — the exact thing
// the product prompt says not to do. So attachment gets the skills declared
// attachable, and the remainder get a read path instead.
func managedSkillPolicy() (installed map[string]bool, attachable map[string]bool) {
	installed = map[string]bool{}
	attachable = map[string]bool{}
	for _, source := range mustVideoStudioManifest().Dependencies.Skills {
		for _, name := range source.Install {
			installed[name] = true
		}
		for _, name := range source.Attach {
			attachable[name] = true
		}
	}
	return installed, attachable
}

// splitStageSkills returns the skills a stage should attach and the workspace
// read paths it needs for the ones it must read from disk instead.
func splitStageSkills(stageSkills []string) (attach []string, readPaths []string) {
	installed, attachable := managedSkillPolicy()
	for _, name := range stageSkills {
		if installed[name] && !attachable[name] {
			readPaths = append(readPaths, "skills/"+name)
			continue
		}
		attach = append(attach, name)
	}
	return attach, readPaths
}

func baseStageAgentConfig() map[string]interface{} {
	return map[string]interface{}{"execution_llm": videoAgentLLMConfig(), "execution_max_turns": 100, "use_code_execution_mode": true, "declared_execution_mode": "agentic", "additional_read_paths": []string{"uploads"}, "learnings_access": "none", "knowledgebase_access": "none", "db_access": "none"}
}
func stepConfigForAll(pipelines []*Pipeline) map[string]interface{} {
	steps := make([]map[string]interface{}, 0, len(pipelines)*8)
	for _, p := range pipelines {
		// The orchestrator is an agent too and needs its own entry; without one
		// it falls back to platform defaults rather than this product's LLM.
		// It carries no stage skills: it sequences routes and relays findings,
		// and never writes an artifact itself.
		if p.Orchestrated != nil {
			steps = append(steps, map[string]interface{}{"id": p.Orchestrated.ID, "title": p.Orchestrated.Title, "agent_configs": baseStageAgentConfig()})
		}
		for _, stage := range p.Stages {
			config := baseStageAgentConfig()
			if len(stage.Skills) > 0 {
				attach, diskPaths := splitStageSkills(stage.Skills)
				if len(attach) > 0 {
					config["enabled_skills"] = attach
				}
				if len(diskPaths) > 0 {
					// enabled_skills also drives the folder guard, so skills that
					// move out of it must get their read path back explicitly or
					// the agent cannot open the file it is told to read.
					config["additional_read_paths"] = append(config["additional_read_paths"].([]string), diskPaths...)
				}
			}
			steps = append(steps, map[string]interface{}{"id": stage.ID, "title": stage.Title, "agent_configs": config})
		}
	}
	return map[string]interface{}{"steps": steps}
}
