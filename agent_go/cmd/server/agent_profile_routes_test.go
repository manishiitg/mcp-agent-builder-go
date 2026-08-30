package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func profileRouteRequest(method, target string, body []byte, userID string) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	claims := &UserClaims{UserID: userID, Username: userID}
	return req.WithContext(context.WithValue(req.Context(), UserContextKey, claims))
}

func routeTestProfile(id string, builtIn bool, ownerID string) agentprofiles.Profile {
	return agentprofiles.Profile{
		ID:                   id,
		Name:                 id,
		Version:              1,
		SystemPromptTemplate: "Work on {{.ProjectTitle}}.",
		Runtime:              agentprofiles.RuntimePolicy{Transport: "auto"},
		BuiltIn:              builtIn,
		OwnerID:              ownerID,
	}
}

func TestQueryRequestForAgentProfileChatUsesOnlyServerOwnedProfileConfiguration(t *testing.T) {
	profile := routeTestProfile("dominion", true, "")
	profile.Name = "Dominion"

	query, err := queryRequestForAgentProfileChat(profile, AgentProfileChatRequest{
		Message: "What changed in the portfolio today?",
	}, ProductConversationRecord{
		ConversationID:  "conversation-1",
		ConversationKey: "main",
		SessionID:       "session-1",
		WorkspacePath:   "Chats",
		Title:           "Dominion",
	})
	if err != nil {
		t.Fatal(err)
	}

	if query.Query != "What changed in the portfolio today?" {
		t.Fatalf("query=%q", query.Query)
	}
	if query.AgentProfileID != "dominion" || query.AgentProfileVersion != 1 {
		t.Fatalf("unexpected profile binding: id=%q version=%d", query.AgentProfileID, query.AgentProfileVersion)
	}
	if query.SelectedFolder != "Chats" || query.AgentProfileContext.ProjectTitle != "Dominion" {
		t.Fatalf("unexpected server-owned workspace binding: folder=%q context=%+v", query.SelectedFolder, query.AgentProfileContext)
	}
	if query.RestoredConversationPath != "" || query.RestoredConversationSessionID != "" {
		t.Fatalf("browser-controlled restore leaked into query: path=%q session=%q", query.RestoredConversationPath, query.RestoredConversationSessionID)
	}
	if query.AgentMode != "multi-agent" || !query.DisableLiveInputDelivery {
		t.Fatalf("unexpected runner configuration: mode=%q disable_live_input=%v", query.AgentMode, query.DisableLiveInputDelivery)
	}
}

func TestQueryRequestForAgentProfileChatRequiresServerOwnedWorkspace(t *testing.T) {
	_, err := queryRequestForAgentProfileChat(
		routeTestProfile("project-product", true, ""),
		AgentProfileChatRequest{Message: "hello"},
		ProductConversationRecord{SessionID: "session-1"},
	)
	if err == nil {
		t.Fatal("expected a conversation without runtime workspace to be rejected")
	}
}

func TestAgentProfileChatEndpointRejectsBroadAgentWorksFields(t *testing.T) {
	api := &StreamingAPI{}
	req := profileRouteRequest(
		http.MethodPost,
		"/api/agent-profiles/dominion/query",
		[]byte(`{"message":"hello","provider":"codex-cli"}`),
		"user-1",
	)
	req = mux.SetURLVars(req, map[string]string{"id": "dominion"})
	recorder := httptest.NewRecorder()

	api.handleAgentProfileChatQuery(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("unknown field")) {
		t.Fatalf("expected unknown-field rejection, body=%s", recorder.Body.String())
	}
}

func TestListAgentProfilesFiltersOtherOwners(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	for _, profile := range []agentprofiles.Profile{
		routeTestProfile("built-in", true, ""),
		routeTestProfile("mine", false, "user-1"),
		routeTestProfile("theirs", false, "user-2"),
	} {
		if err := registry.RegisterProfile(profile); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	listAgentProfilesHandler(registry)(recorder, profileRouteRequest(http.MethodGet, "/api/agent-profiles", nil, "user-1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Profiles []agentprofiles.Profile `json:"profiles"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Profiles) != 2 || response.Profiles[0].ID != "built-in" || response.Profiles[1].ID != "mine" {
		t.Fatalf("unexpected visible profiles: %+v", response.Profiles)
	}
}

func TestGetAgentProfileDoesNotLeakAnotherOwner(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	if err := registry.RegisterProfile(routeTestProfile("mine", false, "user-1")); err != nil {
		t.Fatal(err)
	}
	req := profileRouteRequest(http.MethodGet, "/api/agent-profiles/mine", nil, "user-2")
	req = mux.SetURLVars(req, map[string]string{"id": "mine"})
	recorder := httptest.NewRecorder()
	getAgentProfileHandler(registry)(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestValidateAgentProfileCannotClaimBuiltInAuthority(t *testing.T) {
	profile := routeTestProfile("custom-agent", true, "server-owner")
	body, _ := json.Marshal(profile)
	recorder := httptest.NewRecorder()
	validateAgentProfileHandler()(recorder, profileRouteRequest(http.MethodPost, "/api/agent-profiles/validate", body, "user-1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Valid   bool                  `json:"valid"`
		Profile agentprofiles.Profile `json:"profile"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Valid || response.Profile.BuiltIn || response.Profile.OwnerID != "user-1" {
		t.Fatalf("validation did not enforce user ownership: %+v", response)
	}
}
