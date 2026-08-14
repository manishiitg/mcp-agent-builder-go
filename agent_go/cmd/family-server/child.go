package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/enginedetect"
	mcpagent "github.com/manishiitg/mcpagent/agent"
)

// POST /api/child/message — run one turn of Child Mode tutoring through the
// selected engine. Same agentic runtime as the parent, but with the child tutor
// prompt and the sandboxed child shell — scoped to exactly the current
// activity folder (see child_workspace.go / shell_tool.go).
func handleChildMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req parentMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, parentMessageResponse{Error: "messages are required"})
		return
	}

	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if s.Engine == "" {
		writeJSON(w, http.StatusBadRequest, parentMessageResponse{Error: "no learning engine is selected"})
		return
	}
	if s.Child == nil {
		writeJSON(w, http.StatusBadRequest, parentMessageResponse{Error: "setup is not complete"})
		return
	}
	// The child's conversation lives INSIDE the current activity folder
	// (conversation.json) — one per activity, resumed whenever that same
	// activity is reopened. There is no child session without one.
	activityDir := currentActivityDir()
	if activityDir == "" {
		writeJSON(w, http.StatusBadRequest, parentMessageResponse{Error: "no activity has been handed off yet"})
		return
	}

	resp := runChildTurn(r.Context(), s, activityDir, req.Messages)
	// Every in-turn failure (session construction, the Ask call itself, the
	// no-tools fallback) already reports 200 with the JSON Error field set —
	// same as the parent handler, and what the frontend already expects.
	// workDirPrepFailed is the one exception, needing its original 500.
	status := http.StatusOK
	if resp.Error == errWorkDirPrepFailed {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, resp)
}

// errWorkDirPrepFailed is the Error value ONLY runChildTurn's workDir-mkdir
// failure sets — doubles as both the sentinel handleChildMessage checks for
// and the actual human-readable message sent to the client, the one
// internal-error case that gets a real 500 instead of the 200-with-Error
// every other turn failure uses.
const errWorkDirPrepFailed = "could not prepare the activity folder"

// runChildTurn runs one turn of Child Mode tutoring for the activity dir
// already resolved by the caller — the full tool set, sandbox, prompt, and
// persistence, identical regardless of what triggered it. Shared by the
// HTTP handler above and the WhatsApp "@child mode" text path (see
// whatsapp_bot.go) so a message typed on the phone gets EXACTLY the same
// child experience as one typed in the app, not a separate, drifting copy of
// the sandbox.
func runChildTurn(ctx context.Context, s familyState, activityDir string, messages []enginedetect.ChatMessage) parentMessageResponse {
	// The coding-agent CLI's own launch directory — where IT (not our tools)
	// discovers its project config and drops its own session-scoped files
	// (Cursor's .cursor/hooks.json + hooks/mlp-deny-builtin.sh, a git marker,
	// etc.). Scoped to THIS activity's own folder, not the shared workspace
	// root: every custom tool (execute_shell_command,
	// open_file, read_image) resolves its paths independently via
	// resolveWorkspacePath/workspaceRoot() regardless of this value, so
	// nothing about what the child can read/write changes — but the
	// underlying CLI process itself no longer shares a physical .cursor/
	// folder with the parent's session or any other activity's. Confirmed
	// live: two different sessions sharing that folder raced on cleanup,
	// deleting a hook script a still-running turn depended on ("I can't
	// reach the workspace tools" — see agentsession.closeOtherInteractiveSessions,
	// which stays as defense in depth for parent-vs-child, but this removes
	// the collision at its root for child-vs-child too).
	workDir := filepath.Join(familyDataDir(), "workspace", activityDir)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return parentMessageResponse{Error: errWorkDirPrepFailed}
	}

	// Persist the message(s) that kick off this turn right away, mirroring the
	// parent flow (chat.go) — see persistConversationReplyWithExtras's own
	// comment for the bug this and the completion-time calls below fix
	// together: this used to save the turn's whole transcript straight from
	// req.Messages, which the frontend sends with every past tool message
	// (show_scene, celebrate) already stripped out. That silently deleted a
	// scene shown or a star earned in an EARLIER turn the moment the child
	// sent her next message — persistConversationReplyWithExtras reloads disk
	// as the base instead, so only this turn's own new messages are added.
	persistNewMessages("child", activityDir, messages)

	provider, ok := engineToProvider(s.Engine)
	if !ok {
		// Plain-completion fallback (no tools) for engines not yet mapped.
		reply, err := enginedetect.Chat(ctx, s.Engine, "", workDir, childSystemPrompt(s.Child, s.ParentLabel, activityDir), messages)
		if err != nil {
			return parentMessageResponse{Error: friendlyTurnError(err)}
		}
		return parentMessageResponse{Reply: reply}
	}

	// Recorder captures open_file invocations so the child UI can show the file
	// on the right, mirroring the parent flow (chat.go).
	var evMu sync.Mutex
	var events []toolEvent
	// Recorder for show_scene — at most one scene per turn is shown (the
	// latest call wins if the model calls it more than once).
	var sceneMu sync.Mutex
	var scene string
	childOpenFile := agentsession.Tool{
		Name: "open_file",
		Description: "Show a lesson, worksheet, or one of your own saved pages to " + childDisplayName(s.Child) +
			" on the right side of their screen. Call this when you want them to look at a specific study sheet, " +
			"practice test, or their own work while you talk about it. Pass the workspace-relative path. " +
			"PASS focus WHENEVER you are talking about one specific question or section — that is what actually scrolls " +
			"the page to it. Without focus the page keeps wherever she had scrolled to (or opens at the top the first " +
			"time), so a reply about question 4 next to a page still showing question 1 leaves her hunting for it. " +
			"Omit focus only when you genuinely mean \"here is the page, carry on where you were\" — for example right " +
			"after recording an answer, where holding her place is the point.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{"type": "string", "description": "workspace-relative path to the file to display"},
				"focus": map[string]interface{}{
					"type": "string",
					"description": "id of the element to scroll to — a question (\"q4\"), a section (\"s2\"), a sub-section or worked example " +
						"(\"s2-1\"), or a figure (\"fig1\"); see skills/_shared/html-design.md for the id scheme every generated page follows. " +
						"Set this whenever your reply is clearly about one specific part of the page. Omitting it keeps her current scroll " +
						"position instead. Ignored if no such id exists on the page.",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			p, _ := args["path"].(string)
			p = strings.TrimSpace(p)
			if !childCanSee(p) {
				return "", fmt.Errorf("that file isn't available on the child's screen")
			}
			// Accept "q4", "#q4" or "4" — the model reaches for all three, and
			// normalizing here beats an id that silently doesn't match.
			focus := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(args["focus"])), "#"))
			if focus == "<nil>" {
				focus = ""
			}
			if focus != "" && focus[0] >= '0' && focus[0] <= '9' {
				focus = "q" + focus
			}
			evMu.Lock()
			events = append(events, toolEvent{Tool: "open_file", Path: p, Focus: focus})
			evMu.Unlock()
			return fmt.Sprintf(`{"status":"ok","opened":%q}`, p), nil
		},
	}

	celebrate := agentsession.Tool{
		Name: "celebrate",
		Description: "Award " + childDisplayName(s.Child) + " 1-3 stars for genuine effort or progress, right now, in the moment — " +
			"finishing a test, working through something hard, a nice improvement, real persistence. This is shown live in the chat " +
			"as it happens; it is not tracked as a running total anywhere. Call this in the moment it happens, not routinely. Never " +
			"for just showing up or a single easy answer — save it for effort that actually deserves it, or it stops meaning anything.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"stars":  map[string]interface{}{"type": "integer", "description": "how many stars, 1 to 3"},
				"reason": map[string]interface{}{"type": "string", "description": "one short, warm sentence about what earned it"},
			},
			"required": []string{"stars", "reason"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			starsF, _ := args["stars"].(float64)
			stars := int(starsF)
			if stars < 1 {
				stars = 1
			}
			if stars > 3 {
				stars = 3
			}
			reason, _ := args["reason"].(string)
			reason = strings.TrimSpace(reason)
			if reason == "" {
				return "", fmt.Errorf("reason is required")
			}
			evMu.Lock()
			events = append(events, toolEvent{Tool: "celebrate", Stars: stars, Reason: reason})
			evMu.Unlock()
			return fmt.Sprintf(`{"status":"ok","stars_awarded":%d}`, stars), nil
		},
	}

	// Created BEFORE the mutex so trace.locked() below can see how long this
	// turn actually waited behind another one — see turntrace.go's own comment.
	trace := newTurnTrace("child", s.Engine)
	toolCalls := newToolCallCollector("child:"+activityDir, trace)
	// Serialize on the shared agent-turn lock (parent + child share global MCP env).
	agentTurnMu.Lock()
	defer agentTurnMu.Unlock()
	defer markAgentTurnStart("child")()
	trace.locked()

	ctx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()

	sess, err := agentsession.New(ctx, agentsession.Config{
		Provider: provider,
		// Child Mode runs the same MODEL as Parent Mode, but defaults to lower
		// reasoning effort (see familyState.ChildFastMode) because a child
		// waiting reads as breakage rather than thinking. The model choice
		// below is deliberately unchanged — see why:
		//
		// It used to take a deliberately cheaper tier (haiku for Claude Code,
		// composer-2.5 for Cursor) with "low" effort, on the theory
		// that short one-at-a-time tutoring turns want latency over depth. That
		// traded away quality on exactly the judgment that has to be right: how
		// much to reveal under teaching_mode, whether her answer is actually
		// correct, and which specific step to point at when it isn't. A weaker
		// model being vague about right-vs-wrong is a worse experience than a
		// slightly slower one being clear.
		//
		// This is ONLY the model/effort. The child's sandbox, narrow tool set and
		// separate prompt are unchanged and deliberately stay that way — she must
		// never reach the *-KEY.md answer keys, other activities, or the parent's
		// connectors. See parent_tools.go on why Child Mode is excluded from the
		// shared parent manifest.
		ModelID:         selectedModelID(s, provider),
		ReasoningEffort: selectedReasoningEffort(s.childFastMode(), provider),
		WorkingDir:      workDir,
		SystemPrompt:    childSystemPrompt(s.Child, s.ParentLabel, activityDir),
		// Stable SessionID reuses the warm tmux within this process; SessionHandle
		// restores the coding agent's `--resume` state across restarts (loaded from
		// disk) so context survives a restart without replaying the transcript.
		SessionID:                 activityDir,
		SessionHandle:             loadSessionHandle("child", activityDir, provider),
		BridgeRoutingInstructions: bridgeRoutingInstructions(),
		Transport:                 experimentCodingAgentTransport(),
		StreamCallback: func(text string) {
			trace.delta()
			statusHubs.publishDelta("child:"+activityDir, text)
		},
		Observers:                 []mcpagent.AgentEventListener{toolCalls},
		DirectToolExecutionEvents: true,
		Tools: []agentsession.Tool{
			childShellTool(), childOpenFile, celebrate, notifyTool(), childReadImageTool(s.Engine),
			// Illustrations mid-lesson. The requested dir is IGNORED: whatever the
			// tutor passes, a picture can only ever land in this activity's own
			// folder, matching the child sandbox everywhere else.
			findImageTool(func(string) (string, bool) {
				return resolveWorkspacePath(activityDir)
			}),
			childShowSceneTool(func(html string) {
				sceneMu.Lock()
				scene = html
				sceneMu.Unlock()
			}),
		},
	})
	if err != nil {
		trace.finish("", err)
		msg := friendlyTurnError(err)
		persistConversationReply("child", activityDir, messages, msg)
		return parentMessageResponse{Error: msg}
	}
	trace.sessionReady(sess.Resumed())
	defer sess.Close()

	history := make([]agentsession.Message, 0, len(messages))
	for _, m := range messages {
		history = append(history, agentsession.Message{Role: m.Role, Text: m.Text})
	}
	if suffix := pendingChildUploadSuffix(); suffix != "" && len(history) > 0 {
		history[len(history)-1].Text += suffix
	}

	// Register this turn as steerable for its whole duration, so a follow-up
	// message the child sends while it's still running can be injected live
	// (see steer.go) instead of only ever being queued for afterward.
	registerActiveTurn(activityDir, sess)
	defer clearActiveTurn(activityDir)

	reply, err := sess.Ask(ctx, history)
	trace.finish(reply, err)
	if err != nil {
		// Persist the turn even on failure — see chat.go's parent handler for why.
		msg := friendlyTurnError(err)
		persistConversationReply("child", activityDir, messages, msg)
		return parentMessageResponse{Error: msg, ToolCalls: toolCalls.Snapshot()}
	}
	saveSessionHandle("child", activityDir, sess.Handle())
	evMu.Lock()
	evs := events
	evMu.Unlock()
	sceneMu.Lock()
	sceneOut := scene
	sceneMu.Unlock()

	// Base is disk, not req.Messages — see persistConversationReplyWithExtras.
	// extra carries ONLY this turn's own new tool messages; anything from a
	// past turn is already sitting on disk and reloaded as part of the base.
	var extra []enginedetect.ChatMessage
	if cel := findCelebrateEvent(evs); cel != nil {
		// Persisted alongside the reply so a reloaded transcript replays the
		// star moment exactly where it happened, not just the text.
		extra = append(extra, enginedetect.ChatMessage{Role: "tool", Tool: "celebrate", Stars: cel.Stars, Reason: cel.Reason})
	}
	if sceneOut != "" {
		// Persisted alongside the reply so reloading mid-conversation replays
		// it exactly where it was shown, not just the reply text.
		extra = append(extra, enginedetect.ChatMessage{Role: "tool", Tool: "scene", HTML: sceneOut})
	}
	persistConversationReplyWithExtras("child", activityDir, messages, reply, extra...)
	// powers the "This Week" view (week.go) — accumulates onto today's entry
	// for this activity; trace.duration() excludes queue wait, not just raw
	// elapsed time (see its own comment).
	recordActivityLogEntry(activityDir, trace.duration())

	return parentMessageResponse{Reply: reply, ToolEvents: evs, ToolCalls: toolCalls.Snapshot(), Scene: sceneOut}
}

func findCelebrateEvent(evs []toolEvent) *toolEvent {
	for i := range evs {
		if evs[i].Tool == "celebrate" {
			return &evs[i]
		}
	}
	return nil
}
