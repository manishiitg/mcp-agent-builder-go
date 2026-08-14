package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
)

// parent_tools.go owns the ONE canonical Parent-Mode tool manifest.
//
// Why this file exists: web chat (chat.go), the WhatsApp bot (whatsapp_bot.go),
// direct WhatsApp (whatsapp.go), and Pulse (pulse.go) all run against the SAME
// persistent parent session — every one of them passes
// SessionID = parentConversationID ("parent"), which is deliberate (one
// continuous parent↔Quill conversation, one warm tmux session, resumable via
// the stored session handle). But each surface used to hand that shared session
// a DIFFERENT tool list: 16 tools from web chat, 6 from the WhatsApp bot, 4
// from direct WhatsApp, and 2-7 from Pulse depending on which check ran.
//
// That is a real bug, not just inconsistency. The coding-agent CLI is launched
// once for the warm session and learns its tool set at launch, so WHICHEVER
// SURFACE HAPPENED TO START THE SESSION silently decided what tools the other
// surfaces got for the rest of that session's life. A parent asking the web UI
// to make an activity could hit a session started by a Pulse browser check,
// where create_learning_activity was never registered — and the failure mode is
// the model claiming it can't do something it normally can, which reads as a
// model problem rather than a wiring one. Handler registration is also
// session-scoped, so a surface's handlers could stay registered after its turn.
//
// The fix is that every parent-scope surface asks for the same manifest here.
// Per-surface DIFFERENCES IN BEHAVIOR stay where they belong — in the system
// prompt / per-check instructions (e.g. a Pulse browser check is told to focus
// only on the saved sites), which is the agentic control this codebase prefers
// over hard tool gating that the shared session can't honor anyway.
//
// Child Mode (child.go) is deliberately NOT part of this: it is a genuinely
// different session (SessionID = the activity dir, not "parent"), a different
// prompt, and a legitimately different, narrower tool set.

// parentToolSinks collects the per-turn recorders parent tools write into.
// Every field is optional — a nil sink is a no-op. This is what lets a surface
// that has nowhere to render suggestions (WhatsApp, Pulse) still expose the
// SAME tool manifest as the web UI: it registers the identical tool and simply
// discards that particular signal, instead of omitting the tool and corrupting
// the shared session's capabilities for everyone else.
type parentToolSinks struct {
	onEvent       func(toolEvent)
	onSuggestions func([]suggestion)
	onSentFile    func(path string)
	onSecretSet   func(name, value string)
}

func (s parentToolSinks) event(ev toolEvent) {
	if s.onEvent != nil {
		s.onEvent(ev)
	}
}

func (s parentToolSinks) suggestions(v []suggestion) {
	if s.onSuggestions != nil {
		s.onSuggestions(v)
	}
}

func (s parentToolSinks) sentFile(path string) {
	if s.onSentFile != nil {
		s.onSentFile(path)
	}
}

func (s parentToolSinks) secretSet(name, value string) {
	if s.onSecretSet != nil {
		s.onSecretSet(name, value)
	}
}

// parentChildLabel is how tool descriptions refer to the child. Shared so every
// surface builds the identical manifest — a differing label would otherwise
// make the "same" tool differ by description text between surfaces.
func parentChildLabel(child *Child) string {
	if child != nil && strings.TrimSpace(child.Name) != "" {
		return child.Name
	}
	return "the child"
}

// parentTools returns the canonical Parent-Mode manifest. Every parent-scope
// surface MUST use this, unmodified, so the shared warm session has one stable
// set of capabilities regardless of which surface started it.
func parentTools(engine, childLabel string, sinks parentToolSinks) []agentsession.Tool {
	return []agentsession.Tool{
		setChildProfileTool(sinks),
		setChildScheduleTool(sinks),
		setParentLabelTool(sinks),
		openFileTool(sinks),
		openActivityTool(sinks),
		createLearningActivityTool(childLabel, sinks.event),
		suggestActionsTool(sinks),
		webSearchTool(),
		readImageTool(engine),
		// The parent authors pages anywhere in the workspace, so it names the
		// folder the picture belongs beside; the path is still validated
		// against the workspace root.
		findImageTool(func(requested string) (string, bool) {
			rel := strings.Trim(strings.TrimSpace(requested), "/")
			if rel == "" {
				return "", false
			}
			return resolveWorkspacePath(rel)
		}),
		notifyTool(),
		shellTool(),
		agentBrowserTool(),
		sendWhatsAppFileTool(sinks.sentFile),
		listSecretsTool(),
		setSecretTool(sinks.secretSet),
		deleteSecretTool(),
	}
}

func setChildProfileTool(sinks parentToolSinks) agentsession.Tool {
	return agentsession.Tool{
		Name: "set_child_profile",
		Description: "Save or update the child's profile — name, grade, and school board — once the parent tells you. " +
			"Call this whenever you learn any of these so future sessions and material are tailored to the right level.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":  map[string]interface{}{"type": "string", "description": "the child's name"},
				"grade": map[string]interface{}{"type": "string", "description": "the child's grade/class, e.g. 10"},
				"board": map[string]interface{}{"type": "string", "description": "the school board, e.g. CBSE, ICSE, State Board"},
			},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			nm, _ := args["name"].(string)
			gr, _ := args["grade"].(string)
			bd, _ := args["board"].(string)
			nm, gr, bd = strings.TrimSpace(nm), strings.TrimSpace(gr), strings.TrimSpace(bd)
			if nm == "" && gr == "" && bd == "" {
				return "", fmt.Errorf("provide at least one of name, grade, board")
			}
			stateMu.Lock()
			cur := loadState()
			if cur.Child == nil {
				cur.Child = &Child{Language: "en", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
			}
			if nm != "" {
				cur.Child.Name = nm
			}
			if gr != "" {
				cur.Child.Grade = gr
			}
			if bd != "" {
				cur.Child.Board = bd
			}
			err := saveState(cur)
			saved := cur.Child
			stateMu.Unlock()
			if err != nil {
				return "", fmt.Errorf("failed to save child profile: %w", err)
			}
			seedWorkspace(saved) // keep memory/child-profile.json (read by skills) in sync
			sinks.event(toolEvent{Tool: "set_child_profile", Name: saved.Name, Grade: saved.Grade, Board: saved.Board})
			return fmt.Sprintf(`{"status":"ok","name":%q,"grade":%q,"board":%q}`, saved.Name, saved.Grade, saved.Board), nil
		},
	}
}

// setChildScheduleTool lets Quill capture the child's recurring weekly
// commitments (school hours, tuition, sports practice — anything on the same
// day/time every week) as the parent mentions them in conversation. ADDS to
// the existing schedule rather than replacing it, so the parent can state
// facts incrementally across separate conversations without the model
// needing to reconstruct and resend the whole week each time (the direct
// "This Week" schedule editor handles wholesale replace/remove instead).
func setChildScheduleTool(sinks parentToolSinks) agentsession.Tool {
	return agentsession.Tool{
		Name: "set_child_schedule",
		Description: "Save one or more recurring weekly commitments to the child's schedule — school hours, tuition, sports " +
			"practice, anything that happens on the same day/time every week — once the parent tells you, or you notice one " +
			"from context (not only while the schedule is still empty). ADDS to the existing schedule (never replaces it), " +
			"so call this again for each new fact as it comes up rather than trying to restate the whole week at once — an " +
			"entry that exactly matches one already saved is silently skipped, so it's safe to call even if you're not " +
			"fully sure it's new. Powers the parent's \"This Week\" view, showing her free study time around these " +
			"commitments.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"entries": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"day":   map[string]interface{}{"type": "string", "description": "e.g. Monday"},
							"start": map[string]interface{}{"type": "string", "description": "24h local time, e.g. 08:00"},
							"end":   map[string]interface{}{"type": "string", "description": "24h local time, e.g. 14:30"},
							"label": map[string]interface{}{"type": "string", "description": "e.g. School, Football practice"},
						},
						"required": []string{"day", "start", "end", "label"},
					},
				},
			},
			"required": []string{"entries"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			raw, _ := args["entries"].([]interface{})
			var added []ScheduleEntry
			for _, item := range raw {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				day := strings.TrimSpace(fmt.Sprint(m["day"]))
				start := strings.TrimSpace(fmt.Sprint(m["start"]))
				end := strings.TrimSpace(fmt.Sprint(m["end"]))
				label := strings.TrimSpace(fmt.Sprint(m["label"]))
				if day == "" || start == "" || end == "" || label == "" {
					continue
				}
				added = append(added, ScheduleEntry{Day: day, Start: start, End: end, Label: label})
			}
			if len(added) == 0 {
				return "", fmt.Errorf("no valid entries provided — each needs day, start, end, and label")
			}
			stateMu.Lock()
			cur := loadState()
			// Skip anything that exactly matches an existing entry. This tool is
			// additive with no other dedup, which was tolerable while the only
			// caller was a parent reading the conversation live — a duplicate
			// Pulse can now add unprompted (see pulse.go's schedule check) is
			// easy to miss, sitting as one line inside an automated summary
			// rather than something said to the parent directly.
			newEntries := make([]ScheduleEntry, 0, len(added))
			for _, e := range added {
				dup := false
				for _, ex := range cur.Schedule.Entries {
					if ex.Day == e.Day && ex.Start == e.Start && ex.End == e.End && ex.Label == e.Label {
						dup = true
						break
					}
				}
				if !dup {
					newEntries = append(newEntries, e)
				}
			}
			cur.Schedule.Entries = append(cur.Schedule.Entries, newEntries...)
			err := saveState(cur)
			sched := cur.Schedule
			stateMu.Unlock()
			if err != nil {
				return "", fmt.Errorf("failed to save schedule: %w", err)
			}
			mirrorChildSchedule(sched) // keep memory/child-schedule.json (read by skills) in sync
			sinks.event(toolEvent{Tool: "set_child_schedule"})
			return fmt.Sprintf(`{"status":"ok","added":%d,"total_entries":%d}`, len(newEntries), len(sched.Entries)), nil
		},
	}
}

func setParentLabelTool(sinks parentToolSinks) agentsession.Tool {
	return agentsession.Tool{
		Name: "set_parent_label",
		Description: "Save how the parent wants to be referred to when you talk ABOUT them to the child — e.g. \"mom\", \"dad\", " +
			"\"grandma\", or their first name. Call this once you learn it, whether the parent states it directly or you asked them.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"label": map[string]interface{}{"type": "string", "description": "e.g. mom, dad, grandma, or a first name"},
			},
			"required": []string{"label"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			label, _ := args["label"].(string)
			label = strings.TrimSpace(label)
			if label == "" {
				return "", fmt.Errorf("label is required")
			}
			stateMu.Lock()
			cur := loadState()
			cur.ParentLabel = label
			err := saveState(cur)
			stateMu.Unlock()
			if err != nil {
				return "", fmt.Errorf("failed to save parent label: %w", err)
			}
			sinks.event(toolEvent{Tool: "set_parent_label", ParentLabel: label})
			return fmt.Sprintf(`{"status":"ok","label":%q}`, label), nil
		},
	}
}

func openFileTool(sinks parentToolSinks) agentsession.Tool {
	return agentsession.Tool{
		Name: "open_file",
		Description: "Show a workspace file to the parent on the right side of the screen. Call this right after you " +
			"create or update a file the parent should see (study material, a test, a progress report, the academic map) " +
			"so it opens for them immediately. Pass the workspace-relative path.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{"type": "string", "description": "workspace-relative path to the file to display"},
			},
			"required": []string{"path"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			p, _ := args["path"].(string)
			p = strings.TrimSpace(p)
			if _, ok := resolveWorkspacePath(p); !ok {
				return "", fmt.Errorf("invalid path")
			}
			sinks.event(toolEvent{Tool: "open_file", Path: p})
			return fmt.Sprintf(`{"status":"ok","opened":%q}`, p), nil
		},
	}
}

func openActivityTool(sinks parentToolSinks) agentsession.Tool {
	return agentsession.Tool{
		Name: "open_activity",
		Description: "Show a whole activity (its title, instructions, and item list) to the parent on the right side of the " +
			"screen — a dedicated overview, not a single file. Call this right after create_learning_activity finishes (so the " +
			"parent immediately sees it, with its own 'Give to <child>' button) and whenever the parent asks to see/review/open " +
			"an EXISTING activity as a whole (\"show me that activity\", \"what's in the coding mission\"), as opposed to open_file " +
			"for one specific file inside it. Pass the activity folder (dir), e.g. <Subject>/<Topic>/<slug>.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"dir": map[string]interface{}{"type": "string", "description": "the activity folder, workspace-relative: <Subject>/<Topic>/<slug>"},
			},
			"required": []string{"dir"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			dir := strings.Trim(strings.TrimSpace(fmt.Sprint(args["dir"])), "/")
			if _, ok := loadActivity(dir); !ok {
				return "", fmt.Errorf("no activity found at %q (create it first)", dir)
			}
			sinks.event(toolEvent{Tool: "open_activity", Path: dir})
			return fmt.Sprintf(`{"status":"ok","opened":%q}`, dir), nil
		},
	}
}

func suggestActionsTool(sinks parentToolSinks) agentsession.Tool {
	return agentsession.Tool{
		Name: "suggest_actions",
		Description: "Call this at the END of EVERY turn, without exception — a turn that ends without it leaves the parent " +
			"with nothing to tap, which is a bug, not restraint. Offer 2–4 clickable buttons; each has a short label and " +
			"the exact message sent as if the parent typed it when clicked. Prefer things they probably AREN'T already " +
			"thinking about — a best practice or technique for this topic/board (use web_search), a way to personalize " +
			"further for this specific child's actual pattern (from recent activity, not generic advice), or a genuine " +
			"improvement to what already exists — but if nothing non-obvious comes to mind, offer the two most useful " +
			"obvious things rather than skipping the call. Do NOT use this for \"give/send/hand X to the child\" — " +
			"create_learning_activity + open_activity already put that real button on the right automatically.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"actions": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"label":   map[string]interface{}{"type": "string", "description": "short button text, 2–4 words"},
							"message": map[string]interface{}{"type": "string", "description": "the message sent as the parent when clicked"},
						},
						"required": []string{"label", "message"},
					},
				},
			},
			"required": []string{"actions"},
		},
		Handler: func(_ context.Context, args map[string]interface{}) (string, error) {
			raw, _ := args["actions"].([]interface{})
			out := []suggestion{}
			for _, it := range raw {
				m, ok := it.(map[string]interface{})
				if !ok {
					continue
				}
				label, _ := m["label"].(string)
				msg, _ := m["message"].(string)
				label, msg = strings.TrimSpace(label), strings.TrimSpace(msg)
				if label == "" || msg == "" {
					continue
				}
				// Every action the model sent is kept. "2–4" is stated in the
				// tool description and the system prompt, so the count is the
				// model's call — silently truncating here would drop a button
				// it deliberately chose and hide that it had done so.
				out = append(out, suggestion{Label: label, Message: msg})
			}
			sinks.suggestions(out)
			return fmt.Sprintf(`{"status":"ok","count":%d}`, len(out)), nil
		},
	}
}
