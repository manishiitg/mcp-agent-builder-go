package server

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	internalevents "github.com/manishiitg/coding-agent-loop/agent_go/internal/events"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	unifiedevents "github.com/manishiitg/mcpagent/events"
)

type resolvedAgentProfile struct {
	Definition agentprofiles.Profile
	Prompt     string
}

func cleanAgentProfileWorkspace(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("selected_folder is required when agent_profile_id is set")
	}
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("selected_folder must be workspace-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("selected_folder must stay inside the workspace")
	}
	return clean, nil
}

func appendUniqueStrings(current []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(current)+len(additions))
	out := make([]string, 0, len(current)+len(additions))
	for _, values := range [][]string{current, additions} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func agentProfileRuntimeWorkspace(userID, workspacePath string) string {
	workspacePath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(workspacePath)))
	if workspacePath == "Chats" || strings.HasPrefix(workspacePath, "Chats/") {
		suffix := strings.TrimPrefix(strings.TrimPrefix(workspacePath, "Chats"), "/")
		return filepath.ToSlash(filepath.Join(perUserChatsFolderFor(userID), suffix))
	}
	return workspacePath
}

func resolveProfileRuntimeModel(runtime agentprofiles.RuntimePolicy, requestedProvider, requestedModelID string) (string, string) {
	provider, modelID := strings.TrimSpace(runtime.Provider), strings.TrimSpace(runtime.ModelID)
	for _, option := range runtime.ProviderOptions {
		if option.Default {
			provider, modelID = strings.TrimSpace(option.Provider), strings.TrimSpace(option.ModelID)
			break
		}
	}
	requestedProvider = strings.TrimSpace(requestedProvider)
	requestedModelID = strings.TrimSpace(requestedModelID)
	for _, option := range runtime.ProviderOptions {
		if strings.EqualFold(requestedProvider, strings.TrimSpace(option.Provider)) &&
			strings.EqualFold(requestedModelID, strings.TrimSpace(option.ModelID)) {
			return strings.TrimSpace(option.Provider), strings.TrimSpace(option.ModelID)
		}
	}
	return provider, modelID
}

func (api *StreamingAPI) resolveAgentProfileForQuery(ctx context.Context, req *QueryRequest, userID, sessionID string) (*resolvedAgentProfile, error) {
	profileID := strings.TrimSpace(req.AgentProfileID)
	if profileID == "" {
		if req.AgentProfileVersion != 0 || strings.TrimSpace(req.AgentProfileContext.ProjectTitle) != "" || strings.TrimSpace(req.AgentProfileContext.WorkspaceDescription) != "" {
			return nil, fmt.Errorf("agent_profile_id is required when agent profile fields are provided")
		}
		return nil, nil
	}
	if api.agentProfiles == nil {
		return nil, fmt.Errorf("agent profiles are unavailable")
	}
	if req.AgentMode != "multi-agent" {
		return nil, fmt.Errorf("agent profiles currently require agent_mode=multi-agent")
	}
	workspacePath, err := cleanAgentProfileWorkspace(req.SelectedFolder)
	if err != nil {
		return nil, err
	}
	req.SelectedFolder = workspacePath

	profile, err := api.agentProfiles.Resolve(profileID, req.AgentProfileVersion, userID)
	if err != nil {
		return nil, err
	}
	promptContext := req.AgentProfileContext
	promptContext.ProjectTitle = strings.TrimSpace(promptContext.ProjectTitle)
	if promptContext.ProjectTitle == "" {
		return nil, fmt.Errorf("agent_profile_context.project_title is required")
	}
	if strings.TrimSpace(promptContext.LocalDateTime) == "" {
		now := time.Now()
		_, offsetSeconds := now.Zone()
		offsetSign := "+"
		if offsetSeconds < 0 {
			offsetSign = "-"
			offsetSeconds = -offsetSeconds
		}
		promptContext.LocalDateTime = fmt.Sprintf("%s (UTC%s%02d:%02d)", now.Format("Monday, 2 January 2006 at 3:04 PM MST"), offsetSign, offsetSeconds/3600, (offsetSeconds%3600)/60)
	}
	rendered, err := agentprofiles.RenderPrompt(profile, promptContext)
	if err != nil {
		return nil, err
	}
	if err := api.agentProfiles.Initialize(ctx, profile.ID, agentprofiles.RuntimeContext{
		UserID: userID, SessionID: sessionID, WorkspacePath: workspacePath,
	}); err != nil {
		return nil, fmt.Errorf("initialize agent profile %q: %w", profile.ID, err)
	}
	req.AgentProfileID = profile.ID
	req.AgentProfileVersion = profile.Version
	req.AgentProfileContext = promptContext
	req.SelectedSkills = appendUniqueStrings(req.SelectedSkills, profile.Skills...)
	if profile.Runtime.Capabilities.Secrets == agentprofiles.CapabilityDisabled {
		// Product profiles may explicitly opt out of the shared secret runtime.
		// Clear both user and global selections after saved chat configuration has
		// been applied, so a profile cannot inherit credentials accidentally.
		req.DecryptedSecrets = nil
		noGlobalSecrets := []string{}
		req.SelectedGlobalSecrets = &noGlobalSecrets
	} else if api.chatStore != nil && userID != "" {
		// A product project owns its workflow-scoped secrets. Attach their names
		// automatically for every direct-chat turn so native coding-agent tools
		// receive SECRET_<NAME> without the model ever seeing a value. User-wide
		// secrets remain opt-in through the existing selected-secret mechanism;
		// a project secret with the same name deliberately resolves to the
		// project value.
		stored, secretErr := api.chatStore.ListWorkflowSecrets(ctx, userID, workspacePath)
		if secretErr != nil {
			log.Printf("[SECRETS] Failed to list product workspace secrets for %s (%s): %v", userID, workspacePath, secretErr)
		} else {
			selectedNames := make([]string, 0, len(req.DecryptedSecrets)+len(stored))
			for _, secret := range req.DecryptedSecrets {
				selectedNames = appendUniqueStrings(selectedNames, secret.Name)
			}
			for _, secret := range stored {
				selectedNames = appendUniqueStrings(selectedNames, secret.Name)
			}
			if len(selectedNames) > 0 {
				req.DecryptedSecrets = api.loadSelectedSecrets(ctx, userID, workspacePath, selectedNames)
			}
		}
	}
	browserRequirement := profile.Runtime.Capabilities.Browser
	if browserRequirement == agentprofiles.CapabilityRequired || browserRequirement == agentprofiles.CapabilityPreferred || browserRequirement == agentprofiles.CapabilityOptional {
		// Agent profiles declare browser capability once. The generic chat
		// runtime then registers AgentWorks' managed agent_browser tool and
		// attaches its shared built-in skill; product code must not duplicate
		// either implementation.
		browserEnabled := true
		req.EnableBrowserAccess = &browserEnabled
		if strings.TrimSpace(req.BrowserMode) == "" || strings.EqualFold(strings.TrimSpace(req.BrowserMode), "none") {
			req.BrowserMode = "auto"
		}
	}
	if provider, modelID := resolveProfileRuntimeModel(profile.Runtime, req.Provider, req.ModelID); provider != "" && modelID != "" {
		// A profile-owned model binding is authoritative over the user's global
		// AgentWorks chat selection, while still using the shared provider adapter,
		// credentials, session registry, and streaming lifecycle.
		req.Provider = provider
		req.ModelID = modelID
		req.LLMConfig = &orchestrator.LLMConfig{Primary: orchestrator.LLMModel{Provider: provider, ModelID: modelID}}
		req.LLMConfigSource = llmConfigSourceAgentProfile
		if strings.EqualFold(provider, claudeCodeProviderID) && api.chatStore != nil {
			// Product workspaces use the same encrypted Claude setup-token store as
			// AgentWorks workflows. It stays scoped to this user/workspace and is
			// injected only into the provider runtime, never into a prompt or tool.
			keys, credentialErr := api.workflowProviderAPIKeys(ctx, userID, workspacePath, MergedProviderAPIKeys(ctx))
			if credentialErr != nil {
				return nil, fmt.Errorf("load agent profile Claude Code credential: %w", credentialErr)
			}
			req.LLMConfig.APIKeys = keys
		}
	}
	if strings.TrimSpace(req.SessionTitle) == "" {
		req.SessionTitle = promptContext.ProjectTitle
	}
	return &resolvedAgentProfile{Definition: profile, Prompt: rendered}, nil
}

func profileRuntimeEventType(event any) string {
	if typed, ok := event.(unifiedevents.EventData); ok {
		if eventType := strings.TrimSpace(string(typed.GetEventType())); eventType != "" {
			return eventType
		}
	}
	if payload, ok := event.(map[string]interface{}); ok {
		if eventType, _ := payload["type"].(string); strings.TrimSpace(eventType) != "" {
			return strings.TrimSpace(eventType)
		}
	}
	return "agent_profile_event"
}

// emitAgentProfileEvent records an agent-profile-emitted event for this
// session's stream.
//
// event should be a value implementing unifiedevents.EventData -- a real
// struct from a schema-gen-registered event package (e.g.
// orchestrator_events.PresentationUpdatedEvent), not a hand-built
// map[string]interface{}. A typed value is used directly as the
// AgentEvent.Data payload, so it serializes at the same nesting depth as
// every other typed event (tool_call_end, llm_generation_end, ...) and gets
// a real generated TypeScript interface via cmd/schema-gen instead of an
// `unknown`-typed blob the frontend has to defensively unwrap.
//
// The map[string]interface{} path still exists underneath for a caller that
// has no registered event type yet, but it comes at a real cost: schema-gen
// has no way to generate a shape for it, so consumers get no compile-time
// guarantee about what is inside, and it wraps one JSON level deeper
// (GenericEventData's own "data" field) than a typed event does. Prefer
// registering a real type (see docs/design/agent_tool_surface_single_source.md
// for why "declared once, consumed everywhere" beats "reconstructed per
// consumer").
func (api *StreamingAPI) emitAgentProfileEvent(sessionID string, event any) {
	if api.eventStore == nil {
		return
	}
	eventType := profileRuntimeEventType(event)
	now := time.Now()

	var data unifiedevents.EventData
	if typed, ok := event.(unifiedevents.EventData); ok {
		data = typed
	} else {
		payload := map[string]interface{}{"event": event}
		if untyped, ok := event.(map[string]interface{}); ok {
			payload = untyped
		}
		data = &unifiedevents.GenericEventData{Data: payload}
	}

	api.eventStore.AddEvent(sessionID, internalevents.Event{
		ID:        fmt.Sprintf("profile_%s_%d", strings.ReplaceAll(eventType, ".", "_"), now.UnixNano()),
		Type:      eventType,
		Timestamp: now,
		SessionID: sessionID,
		Data: &unifiedevents.AgentEvent{
			Type: unifiedevents.EventType(eventType), Timestamp: now,
			Data: data,
		},
	})
}

func (api *StreamingAPI) registerAgentProfileTools(registrar definitionToolRegistrar, resolved *resolvedAgentProfile, userID, sessionID, workspacePath string) error {
	if resolved == nil {
		return nil
	}
	for _, binding := range resolved.Definition.Tools {
		tool, err := api.agentProfiles.BuildTool(binding, agentprofiles.ToolRuntimeContext{
			UserID: userID, SessionID: sessionID, WorkspacePath: workspacePath,
			Emit:         func(event any) { api.emitAgentProfileEvent(sessionID, event) },
			Presentation: binding.Presentation,
		})
		if err != nil {
			return err
		}
		category := strings.TrimSpace(tool.Category)
		if category == "" {
			category = "agent_profile_tools"
		}
		if err := registrar.RegisterCustomTool(tool.Name, tool.Description, tool.Parameters, tool.Execute, category); err != nil {
			return fmt.Errorf("register profile tool %q: %w", tool.Name, err)
		}
	}
	return nil
}

func profileDisablesVirtualTool(profile *resolvedAgentProfile, toolName string) bool {
	if profile == nil {
		return false
	}
	for _, disabled := range profile.Definition.ToolPolicy.Disabled {
		if strings.EqualFold(strings.TrimSpace(disabled), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}
