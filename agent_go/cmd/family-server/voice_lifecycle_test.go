package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/voicestt"
)

func TestVoiceLifecycleRejectsGET(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"warm":   handleVoiceWarm,
		"unload": handleVoiceUnload,
		"stream": handleVoiceStream,
	} {
		method := http.MethodGet
		if name == "stream" {
			method = http.MethodPost
		}
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(method, "/", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 405, got %d", name, rec.Code)
		}
	}
}

// The Settings card, the WhatsApp toggle and the mic must all read the SAME
// engine state. This pins the tier's shape to the shared voicestt.Status so
// nobody reintroduces a second, disconnected notion of "installed".
func TestVoiceStatusReportsSharedEngine(t *testing.T) {
	rec := httptest.NewRecorder()
	handleVoiceStatus(rec, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out voiceStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(out.STTTiers) != 1 || out.STTTiers[0].ID != voiceTierID {
		t.Fatalf("expected exactly the %q tier, got %+v", voiceTierID, out.STTTiers)
	}
	tier := out.STTTiers[0]
	st := familyVoice.Status()
	if tier.Available != st.Available || tier.Available != voicestt.Available {
		t.Fatalf("tier.available=%v, engine available=%v, build available=%v", tier.Available, st.Available, voicestt.Available)
	}
	if tier.SizeMB != voicestt.ModelSizeMB {
		t.Fatalf("tier.size_mb=%d, want %d", tier.SizeMB, voicestt.ModelSizeMB)
	}
	if tier.Warm && !tier.Installed {
		t.Fatal("a tier cannot be warm without being installed")
	}
}

func TestVoiceModelEndpointsRejectUnknownTier(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"install": handleVoiceModelInstall,
		"remove":  handleVoiceModelRemove,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", jsonBody(`{"id":"parakeet"}`))
		h(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400 for the retired tier id, got %d", name, rec.Code)
		}
	}
}

func jsonBody(s string) io.Reader { return strings.NewReader(s) }
