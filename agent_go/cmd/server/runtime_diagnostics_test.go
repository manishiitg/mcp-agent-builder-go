package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeDiagnosticsHandlerIsOffByDefault(t *testing.T) {
	t.Setenv("AGENTWORKS_RUNTIME_DEBUG", "")
	called := false
	handler := (&StreamingAPI{}).runtimeDiagnosticsHandler(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/terminals", nil))

	if called {
		t.Fatal("diagnostic handler ran while runtime debugging was disabled")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestRuntimeDiagnosticsHandlerAllowsExplicitOptIn(t *testing.T) {
	t.Setenv("AGENTWORKS_RUNTIME_DEBUG", "1")
	called := false
	handler := (&StreamingAPI{}).runtimeDiagnosticsHandler(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/terminals", nil))

	if !called {
		t.Fatal("diagnostic handler did not run after explicit opt-in")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
