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
		routes = append(routes, map[string]interface{}{"route_id": p.ID, "route_name": p.Name, "condition": p.WhenToUse, "next_step_id": p.Stages[0].ID})
	}
	steps = append(steps, map[string]interface{}{"type": "routing", "id": routeStepID, "title": "Route", "routing_question": "Which pipeline does this brief call for?", "routes": routes, "default_route_id": pipelines[0].ID})
	for _, p := range pipelines {
		for i, stage := range p.Stages {
			deps := []string{}
			for prev := 0; prev < i; prev++ {
				deps = append(deps, p.Stages[prev].Output)
				deps = append(deps, p.Stages[prev].Artifacts...)
			}
			required := []map[string]interface{}{{"file_name": stage.Output, "must_exist": true}}
			for _, artifact := range stage.Artifacts {
				required = append(required, map[string]interface{}{"file_name": artifact, "must_exist": true})
			}
			step := map[string]interface{}{"type": "regular", "id": stage.ID, "title": stage.Title, "description": stage.Description, "context_dependencies": deps, "context_output": stage.Output, "has_loop": false, "validation_schema": map[string]interface{}{"files": required}}
			if i == len(p.Stages)-1 {
				step["next_step_id"] = "end"
			}
			steps = append(steps, step)
		}
	}
	return map[string]interface{}{"steps": steps}
}
func baseStageAgentConfig() map[string]interface{} {
	return map[string]interface{}{"execution_llm": videoAgentLLMConfig(), "execution_max_turns": 100, "use_code_execution_mode": true, "declared_execution_mode": "agentic", "additional_read_paths": []string{"uploads"}, "learnings_access": "none", "knowledgebase_access": "none", "db_access": "none"}
}
func stepConfigForAll(pipelines []*Pipeline) map[string]interface{} {
	steps := make([]map[string]interface{}, 0, len(pipelines)*8)
	for _, p := range pipelines {
		for _, stage := range p.Stages {
			config := baseStageAgentConfig()
			if len(stage.Skills) > 0 {
				config["enabled_skills"] = append([]string{}, stage.Skills...)
			}
			steps = append(steps, map[string]interface{}{"id": stage.ID, "title": stage.Title, "agent_configs": config})
		}
	}
	return map[string]interface{}{"steps": steps}
}
