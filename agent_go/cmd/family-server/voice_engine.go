package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/voicestt"
)

// familyVoice is SparkQuill's handle on the ONE AgentWorks speech-to-text
// engine (pkg/voicestt: sherpa-onnx running NVIDIA's Nemotron streaming
// model, in-process, on-device). The same package, model, and WebSocket
// protocol serve AgentWorks' composer, Video Studio, and this app — on any
// Mac and on Linux — so there is exactly one voice stack to keep working.
//
// This replaced two earlier SparkQuill-only engines: the Python/MLX Parakeet
// worker (~3.1GB venv, Apple Silicon only) and the Swift/CoreML voice helper.
// Both are gone; the model directory is shared with every other AgentWorks
// binary on the machine, so one ~690MB download serves all of them.
var familyVoice = voicestt.NewManager(voicestt.DefaultModelDir())

// voiceTierID is the one speech tier this app offers. The Settings card, the
// install/remove endpoints and the mic test all name it.
const voiceTierID = "standard"

// voiceUpgrader admits the same origins the JSON API does (withCORS) plus the
// app's own origin when family-server serves the built frontend itself, which
// is what the packaged desktop app does. gorilla's default rejects any
// cross-port Origin outright, which is precisely the Vite dev setup.
var voiceUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 4 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" || allowedOrigins[origin] {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host := strings.ToLower(u.Hostname())
		return host == "127.0.0.1" || host == "localhost"
	},
}
