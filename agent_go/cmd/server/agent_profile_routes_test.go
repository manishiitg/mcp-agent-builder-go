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
