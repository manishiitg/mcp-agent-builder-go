package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
)

// childShowSceneTool lets the child tutor render a small, freshly-generated
// HTML visual INLINE in its reply — a story beat, a diagram, a "guess before
// you peek" moment with real choices — instead of only ever pointing at the
// activity's one original static file. Unlike that file, a scene is generated
// fresh every time it's called, so it can match whatever's actually being
// discussed right now (a tangent the child took the conversation into, not
// just what was anticipated when the activity was first created).
func childShowSceneTool(recordScene func(html string)) agentsession.Tool {
	return agentsession.Tool{
		Name: "show_scene",
		Description: "Show a small, self-contained HTML visual INLINE in this reply — a story beat, a diagram, a 'guess before you peek' " +
			"moment, a mini interactive scene, a genuinely playable moment. Generate it fresh to match exactly what's happening in the " +
			"conversation right now — not limited to whatever is in the activity's original file, so it can follow the child's own " +
			"tangents naturally. Full CSS animation AND real JavaScript are available — <script> runs, so build actual interactivity " +
			"(drag, click-and-respond, a running score, a tiny simulation or game), not just something that plays on its own. Use " +
			"whatever the moment genuinely calls for; don't hold back on capability you actually have. Keep it SMALL (a few lines/cards " +
			"— not a full page) and self-contained (inline CSS/JS only, no external assets or network calls, follow " +
			"skills/_shared/html-design.md's visual style). One real constraint: this iframe stays mounted in her chat history forever " +
			"— it is never torn down when the conversation moves on — so any setInterval/requestAnimationFrame loop must have a natural " +
			"stopping point (finish the animation, end the game, clear the timer) rather than run forever in the background. " +
			"To offer a real choice you need to see and respond to, use a button that calls `SQ.choose(text, this)` — never a `<details>` " +
			"reveal or a button that does nothing further. It also disables itself the instant it's tapped, so a slow reply can't be " +
			"mistaken for a missed tap and answered twice: `<button onclick=\"SQ.choose('Investigate Saturn', this)\">" +
			"Investigate Saturn</button>`. Call this when a visual moment genuinely helps, not every single turn — plain conversation is " +
			"fine most of the time.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"html": map[string]interface{}{"type": "string", "description": "the small, self-contained HTML snippet to show inline"},
			},
			"required": []string{"html"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			html := strings.TrimSpace(fmt.Sprint(args["html"]))
			if html == "" {
				return "", fmt.Errorf("html is required")
			}
			if recordScene != nil {
				recordScene(html)
			}
			return `{"status":"ok"}`, nil
		},
	}
}
