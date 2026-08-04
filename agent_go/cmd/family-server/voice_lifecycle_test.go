package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The unload endpoint is driven by a window event, so it can fire at any
// moment — including while someone is mid-sentence. Killing the helper then
// would discard the utterance being spoken, so an active dictation must veto
// it.
func TestVoiceNativeUnloadRefusesDuringDictation(t *testing.T) {
	voiceStreamActive.Store(true)
	defer voiceStreamActive.Store(false)

	rec := httptest.NewRecorder()
	handleVoiceNativeUnload(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out["unloaded"] != false {
		t.Fatalf("expected the unload to be refused, got %v", out)
	}
}

func TestVoiceNativeLifecycleRejectsGET(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"warm":   handleVoiceNativeWarm,
		"unload": handleVoiceNativeUnload,
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 405, got %d", name, rec.Code)
		}
	}
}
