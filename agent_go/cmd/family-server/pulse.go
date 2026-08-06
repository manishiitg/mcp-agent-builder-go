package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/enginedetect"
	"github.com/manishiitg/mcpagent/llm"
)

// appendPulseTurn appends a Pulse check-in as a real, visible two-message
// turn — a parent-facing trigger line (what the check-in looked at) followed
// by Quill's reply — both tagged Source:"pulse" so the UI marks them as an
// automated check-in. Nothing about Pulse is hidden: the parent sees the
// check-in happen and its result, exactly like a normal chat turn. The
// trigger persisted here is the CLEAN parent-facing line (pulseCheck.trigger),
// NOT the raw technical instruction sent to the model — showing the parent
// file paths/skill names would break the "parent is not technical, hide the
// machinery" rule the whole app follows.
func appendPulseTurn(messages []enginedetect.ChatMessage, trigger, reply string) []enginedetect.ChatMessage {
	full := append([]enginedetect.ChatMessage(nil), messages...)
	full = append(full, enginedetect.ChatMessage{Role: "user", Text: trigger, Source: "pulse"})
	full = append(full, enginedetect.ChatMessage{Role: "assistant", Text: reply, Source: "pulse"})
	return full
}

// pulseCheck is one focused Pulse sub-check. Pulse runs each enabled check as
// its OWN agent turn producing its own visible message (a parent-facing
// trigger divider + Quill's reply), rather than one combined message doing
// everything at once — so the parent sees distinct, legible check-ins
// (learning, school portal, school email, memory) instead of one dense blob.
type pulseCheck struct {
	trigger     string // clean parent-facing divider line (no file paths)
	instruction string // full technical instruction sent to the model
}

// NOTE: a pulseCheck deliberately has NO per-check tool list. Pulse runs on
// the SAME shared warm "parent" session as web chat and WhatsApp, so a
// narrower per-check manifest could not be honored anyway (the session's tool
// set is fixed by whichever surface launched it) and actively risked defining
// a crippled session for the other surfaces. Every check gets the one
// canonical parentTools(...) manifest; what a check should FOCUS on stays in
// its instruction text, which is the control that actually works here.

// pulseReplyRules is the shared tail every check's instruction ends with, so
// each focused turn produces one short, honest, parent-appropriate message.
const pulseReplyRules = " If nothing here has meaningfully changed since your last check-in visible earlier in this conversation, " +
	"say so briefly and warmly — do not manufacture busywork or repeat something you already told the parent. " +
	"Write ONE short, warm message as your reply, following your usual rules (parent is not technical, no files/paths/JSON, plain language)."

// pulseChecks returns the ordered set of check-ins to run this cycle — the
// learning review and the memory update always run; the school portal and
// school email checks only run when the parent has configured them.
func pulseChecks(s familyState) []pulseCheck {
	who := "the child"
	if s.Child != nil && strings.TrimSpace(s.Child.Name) != "" {
		who = s.Child.Name
	}
	engine := s.Engine

	checks := []pulseCheck{{
		trigger: "Automated check-in — reviewing recent learning activity",
		instruction: "This is an automated Pulse check-in — the parent did not just ask you anything; you're reviewing on your own " +
			"initiative, focused ONLY on " + who + "'s recent learning activity this turn (ignore email and the school portal — those are " +
			"separate check-ins). Look at what's actually changed since your last check (recent conversations, activity attempts/conversation.json across activities, test results, " +
			"uploaded materials). Rebuild BOTH reports/academic-map.html and reports/progress.html per their skill files " +
			"(skills/create-academic-map/SKILL.md, skills/create-progress-report/SKILL.md) every time this check runs, even if one " +
			"side has less new evidence than the other — both are fully-regenerated current-state snapshots, never a dated log, so " +
			"each should always reflect today's real picture, not the picture from whenever it last happened to change. If a clear gap " +
			"or opportunity stands out (a weak topic, something " + who + " hasn't practiced in a while, a natural next step), you may prepare " +
			"study material or a test (skills/create-study-material/SKILL.md, skills/create-test/SKILL.md) — but do NOT create or hand off an " +
			"activity for it; nothing gets handed to " + who + " without the parent explicitly asking, so just mention what you made." + pulseReplyRules,
	}}

	if sites := s.Pulse.Sites(); len(sites) > 0 {
		list := strings.Join(sites, ", ")
		checks = append(checks, pulseCheck{
			trigger: "Automated check-in — checking your saved websites",
			instruction: "This is an automated Pulse check-in, focused ONLY on the website(s) the parent asked you to keep an eye on: " + list + ". " +
				"Use agent_browser (it reuses the parent's own signed-in browser). FIRST run tab list to see all open tabs — the parent's browser " +
				"usually has many, and the active one is often unrelated (a work site, etc.); find the tab(s) that match the site(s) above and switch " +
				"to each, or open the URL yourself if it isn't already a tab. NEVER read or act on an unrelated tab just because it's in front. " +
				"BEFORE navigating each site, check memory/browser-notes.md for anything you've already saved about it — the menu path to " +
				"assignments, where grades actually live, a login quirk, a dead end to skip. Go straight there instead of re-discovering it from " +
				"scratch every single check-in; a recurring check should get FASTER over time, the way a person who's used a site before navigates " +
				"it quicker than a first-timer. Then explore each of the parent's site(s) THOROUGHLY. These can be a school portal, a class website, or any third-party site — a school portal for instance " +
				"usually has a lot: assignments/homework, due dates, uploaded books/materials/handouts, grades or graded work, teacher " +
				"announcements/notices, timetable or calendar changes, messages, attendance. Don't stop at the first page: navigate into the main " +
				"sections (snapshot the page, follow the obvious links) and gather as much concrete detail as you can — specific item names, due " +
				"dates, topics, anything new or relevant to " + who + ". When a site has actual resource FILES worth keeping — worksheets, notes, " +
				"PDFs, images, handouts, question papers — download them: clicking a download link puts the file in the parent's Downloads folder on " +
				"this computer, so then use execute_shell_command to copy it into materials/<subject>/<topic>/ (e.g. " +
				"`cp ~/Downloads/<file> materials/...`) so it becomes part of " + who + "'s workspace. Then INGEST what you saved the same way an " +
				"uploaded file is handled: for an image call read_image to see what it actually is; for a PDF or document, read/convert it with your " +
				"shell tools to pull out the real content; follow skills/process-file/SKILL.md to file it properly (right subject/topic, a short " +
				"summary). The goal is that a useful resource on a site ends up usable INSIDE SparkQuill, not just noticed. Then tell the parent " +
				"plainly what's actually new across the site(s) and what matters for " + who + " — be specific (names, dates, what you pulled in), not " +
				"vague. If a site needs a login you can't get past, say it needs them to sign in first (via the Browser connector) rather than " +
				"guessing. Before finishing, update memory/browser-notes.md with anything you learned this run that will save time next check-in — " +
				"a menu path, a section that turned out to be a dead end, a login quirk. Keep it SHORT (a few lines per site, not a transcript of the " +
				"visit) and edit your own existing entry for that site rather than appending a new one each time, so the file stays a compact, " +
				"current cheat sheet instead of growing forever. " +
				"Also rewrite memory/school-deadlines.json with the CURRENT real picture of assignments/tests and their due dates you just saw across " +
				"the site(s) — this powers the parent's \"This Week\" view, so it needs real structured data, not prose. Fully rewrite the whole file " +
				"each time (same convention as browser-notes.md/preferences.md: the current real picture, not an accumulating log) — drop anything no " +
				"longer listed on the site (submitted, past its window, removed) rather than leaving it sitting there stale. Shape: " +
				"{\"deadlines\":[{\"title\":\"...\",\"subject\":\"...\",\"due_date\":\"YYYY-MM-DD\",\"kind\":\"assignment|test\"}],\"updated_at\":\"...\"}. " +
				"Only include items with a real, known due date — skip anything where the site didn't give you one rather than guessing." + pulseReplyRules,
		})
	}

	checks = append(checks, pulseCheck{
		trigger: "Automated check-in — updating what I remember about your preferences",
		instruction: "This is an automated Pulse check-in, focused ONLY on your working memory of the parent's preferences. Read " +
			"skills/update-preferences/SKILL.md and follow it: check memory/preferences.md against what the parent has actually said across " +
			"conversations/parent.json, and update it in place if there's something durable worth remembering (exam dates, scheduling/behavioral " +
			"preferences, content preferences) that isn't already captured. Tell the parent in one short line what (if anything) you noted." + pulseReplyRules,
	})

	checks = append(checks, pulseCheck{
		trigger: "Automated check-in — learning " + who + "'s interests",
		instruction: "This is an automated Pulse check-in, focused ONLY on what " + who + " genuinely enjoys, learned from her own " +
			"conversations (not what the parent has said — that's the preferences check above). Read " +
			"skills/update-child-interests/SKILL.md and follow it: check memory/interests.md against " + who + "'s actual engagement across " +
			"her own activity conversations, and update it in place if a genuine interest (or clear disinterest) signal isn't already captured. This " +
			"powers skills/discover-something-new/SKILL.md, which the parent can ask for anytime (\"make her something fun this weekend\"). " +
			"Tell the parent in one short line what (if anything) you noted." + pulseReplyRules,
	})

	checks = append(checks, pulseCheck{
		trigger: "Automated check-in — backing up the workspace",
		instruction: "This is an automated Pulse check-in, focused ONLY on backup. Read skills/backup/SKILL.md and follow it exactly, " +
			"including its rule that the FIRST (repo-creating) backup must be attended — this is an automated cycle with no parent " +
			"present, so if it isn't set up yet (or backup/status.json has never shown a verified success), just note ONCE that " +
			"it isn't set up, don't repeat that note if you already said it recently, and stop. If it IS already set up and " +
			"verified, run a real backup per the skill (check the token, skip-if-unchanged, upload excluding archive/, write " +
			"status.json). Tell the parent in one short line what happened, or say nothing new-worthy if it was skipped as " +
			"unchanged." + pulseReplyRules,
	})

	_ = engine
	return checks
}

// Pulse is SparkQuill's version of AgentWorks' Pulse feature (see the design
// discussion this was built from): a periodic, opt-in check-in that reviews
// recent learning activity and keeps reports/academic-map.html and
// reports/progress.html current, proposing new study material where a
// gap shows up. Deliberately much simpler than AgentWorks' multi-module
// Gate/Reviewer/Fixer machinery (that exists because ONE workflow run can
// touch ten disparate concern types with no natural home in its own output
// files) — here there's exactly one output that matters and it already has a
// durable, dated, human-readable home. No separate Pulse log file either
// (see the "why do we need improve.html" conversation): findings are written
// straight into the existing academic map/progress report, and the
// parent-facing narrative goes into their own ongoing chat — nothing needs a
// second log that just restates what's already visible elsewhere.
//
// Crucially, Pulse does NOT run in its own session/thread: it checks in on the
// single parent conversation (parentConversationID) that web chat, WhatsApp, and
// Pulse all share — so a check-in reads like Quill following up in the same
// conversation the parent already has open, not a separate channel they'd have
// to remember to check.
const pulseTickInterval = 5 * time.Minute

// pulseQuietPeriod is how long the family must have been idle before a
// scheduled Pulse may start. Pulse holds the agent for 25-250s per check and
// runs several back to back, so starting one while someone is mid-conversation
// pushes their reply minutes into the future.
const pulseQuietPeriod = 10 * time.Minute

// pulseMaxDeferral stops "wait for quiet" becoming "never run". A family using
// the app all evening would otherwise defer Pulse indefinitely and silently
// lose their check-ins; past this much overdue it runs regardless.
const pulseMaxDeferral = 4 * time.Hour

// runPulseOnce runs one Pulse cycle. When force is false (the periodic
// ticker's normal call), it's a no-op unless Pulse is actually enabled —
// when force is true (a manual "run now" trigger), it runs regardless of the
// enabled toggle, since testing it shouldn't require turning on the
// recurring schedule first.
//
// A cycle runs each check in pulseChecks(s) as its OWN sequential agent turn,
// persisting each as its own visible message before moving to the next — so
// the parent sees distinct check-ins, and if the process dies mid-cycle the
// checks that already completed are still saved.
func runPulseOnce(ctx context.Context, force bool) error {
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if !force && !s.Pulse.Enabled {
		return nil
	}
	if s.Engine == "" {
		return fmt.Errorf("no learning engine selected")
	}
	if s.Child == nil {
		return fmt.Errorf("no child profile set up yet")
	}
	provider, ok := engineToProvider(s.Engine)
	if !ok {
		return fmt.Errorf("engine %q has no provider mapping", s.Engine)
	}

	// Mechanical housekeeping, not an agent turn — see archive.go.
	archiveStaleActivities()

	// Pulse checks in on the SINGLE parent conversation (same file + warm tmux
	// session as the web chat and WhatsApp) — one unified thread, not a separate
	// Pulse channel.
	convID := parentConversationID

	// Parent and every child activity share the same physical workspace, so
	// starting a new session here would force-close whatever OTHER session is
	// currently warm (see agentsession.closeOtherInteractiveSessions) — fine
	// for a direct user action, but Pulse fires on its own schedule with no
	// idea whether the child is mid-conversation right now. Defer to the next
	// cadence tick rather than silently evicting her live session the moment
	// she pauses between messages.
	if agentsession.HasOtherWarmInteractiveSession(convID) {
		log.Printf("[pulse] deferring — another session is currently active on the shared workspace")
		return nil
	}

	existing, _ := loadStoredConversation("parent", convID)

	agentTurnMu.Lock()
	defer agentTurnMu.Unlock()
	defer markAgentTurnStart("pulse")()

	messages := existing.Messages
	for _, c := range pulseChecks(s) {
		if err := ctx.Err(); err != nil {
			return err
		}
		reply, err := runPulseCheckTurn(ctx, provider, s, convID, messages, c)
		if err != nil {
			return fmt.Errorf("%q check failed: %w", c.trigger, err)
		}
		// Persist each check as its own visible turn immediately, so a later
		// check failing (or the process dying) doesn't lose the ones already done.
		messages = appendPulseTurn(messages, c.trigger, reply)
		persistConversation("parent", convID, messages)
	}

	// The agent decides what's worth telling the parent and sends it — Go's
	// job stops at asking. An earlier version of this function mechanically
	// joined every check's raw reply into one string and rendered the email
	// in Go, which meant the actual judgment call (what matters, what's too
	// trivial to mention, how to weigh it against the parent's own known
	// preferences in memory/preferences.md) never happened — Go doesn't have
	// that judgment. The agent does, and it already has notify_user
	// (parentTools) for exactly this, so this closing turn just asks it to
	// use it: review this cycle (already in `messages`, no separate context
	// needed) and call notify_user itself, once, with its own title, message,
	// and a real email_html it composes — not a template Go fills in.
	notifyCheck := pulseCheck{
		trigger: "Automated check-in — sending you a summary",
		instruction: "This automated Pulse cycle is done — you've just gone through everything above yourself, in this same " +
			"conversation. Now decide what's ACTUALLY worth telling the parent: skip anything trivial or unchanged, weigh it " +
			"against anything you know of their preferences (memory/preferences.md), and lead with whatever matters most. " +
			"MORE THAN ONE PARENT MAY USE THIS FAMILY — one by chat, another only by WhatsApp — and notify_user is the ONLY " +
			"thing that reaches WhatsApp, so it must not be scoped to Pulse's own checks alone. Also look back over this SAME " +
			"conversation for anything a parent directly asked, decided, or had you do since your last check-in (a new activity, " +
			"a real decision, a setting changed) that a parent who ONLY sees WhatsApp would otherwise never learn about. Fold " +
			"anything genuinely worth surfacing into this summary too, so both parents stay in sync regardless of which one of " +
			"them you actually spoke with. " +
			"Then call notify_user EXACTLY ONCE to send it — a short title, a brief plain message (1-3 sentences, your usual " +
			"voice; this is what appears on desktop/WhatsApp), and a well-structured, EMAIL-SAFE inline-styled email_html with " +
			"its own heading per topic that actually has news (skip a topic entirely rather than write \"nothing new\" for " +
			"it) so the email reads as organized sections, not one dense paragraph. If truly nothing meaningful happened " +
			"across every check this cycle, still call notify_user with one short, honest line saying so — never skip the " +
			"call itself; it's the parent's only signal this cycle ran at all. Afterward, report the outcome honestly in " +
			"your reply (notify_user tells you what actually got delivered) — never claim it reached them if it didn't. " +
			"If (and only if) the progress report actually gained genuinely new content THIS cycle — not just a re-save of " +
			"the same picture — a WhatsApp-only parent still can't just click it open the way a parent using the app can, " +
			"so also hand them the real document: export reports/progress.html to a PDF via agent_browser's \"pdf\" command " +
			"(into reports/progress.pdf) and send it with send_whatsapp_file, a short caption naming what's new and what " +
			"she should prepare for next. Skip this entirely on a cycle where nothing genuinely changed — a PDF resent every " +
			"five minutes with no new content is noise, not help.",
	}
	reply, err := runPulseCheckTurn(ctx, provider, s, convID, messages, notifyCheck)
	if err != nil {
		return fmt.Errorf("summary notification failed: %w", err)
	}
	messages = appendPulseTurn(messages, notifyCheck.trigger, reply)
	persistConversation("parent", convID, messages)
	// Which of these fired is otherwise unrecoverable from the log: the
	// ticker (startPulseTicker) and the manual "Run Pulse Now" button
	// (handlePulseRunNow) both land here with no other distinguishing trace,
	// which made a real investigation (why did Pulse run 4 times in 4
	// minutes when cadence_hours=12?) unable to rule out a scheduling bug
	// versus repeated manual clicks — this is the only place that
	// certainty could have come from.
	source := "scheduled"
	if force {
		source = "manual"
	}
	log.Printf("[pulse] cycle complete (%s): %s", source, strings.TrimSpace(reply))

	stateMu.Lock()
	cur := loadState()
	cur.Pulse.LastRunAt = time.Now().UTC().Format(time.RFC3339)
	_ = saveState(cur)
	stateMu.Unlock()
	return nil
}

// runPulseCheckTurn runs one focused check as a single agent turn over the
// accumulated conversation so far (so it sees prior checks this cycle and
// won't repeat them), returning Quill's reply. The technical instruction is
// sent to the model but never persisted — only the clean trigger + reply are
// (see appendPulseTurn).
func runPulseCheckTurn(ctx context.Context, provider llm.Provider, s familyState, convID string, messages []enginedetect.ChatMessage, c pulseCheck) (string, error) {
	history := make([]agentsession.Message, 0, len(messages)+1)
	for _, m := range messages {
		history = append(history, agentsession.Message{Role: m.Role, Text: m.Text})
	}
	history = append(history, agentsession.Message{Role: "user", Text: c.instruction})

	trace := newTurnTrace("pulse", s.Engine)

	sess, err := agentsession.New(ctx, agentsession.Config{
		Provider:                  provider,
		ModelID:                   selectedModelID(s.FastMode, provider),
		ReasoningEffort:           "high",
		WorkingDir:                filepath.Join(familyDataDir(), "workspace"),
		SystemPrompt:              parentSystemPrompt(s.Child, s.ParentLabel, s.Pulse, s.Schedule),
		SessionID:                 convID,
		SessionHandle:             loadSessionHandle("parent", convID, provider),
		BridgeRoutingInstructions: bridgeRoutingInstructions(),
		Tools:                     withLiveStatus("pulse:"+convID, parentTools(s.Engine, parentChildLabel(s.Child), parentToolSinks{})),
	})
	if err != nil {
		trace.finish("", err)
		return "", fmt.Errorf("session setup failed: %w", err)
	}
	trace.sessionReady(sess.Resumed())
	defer sess.Close()

	reply, err := sess.Ask(ctx, history)
	trace.finish(reply, err)
	if err != nil {
		return "", err
	}
	saveSessionHandle("parent", convID, sess.Handle())
	return reply, nil
}

// startPulseTicker runs the periodic check forever until ctx is canceled.
// A plain wall-clock ticker is enough at this scale — no cron parser needed,
// since the only knob is "every N hours," configured via /api/pulse/config.
func startPulseTicker(ctx context.Context) {
	ticker := time.NewTicker(pulseTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stateMu.Lock()
			s := loadState()
			stateMu.Unlock()
			if !s.Pulse.Enabled {
				continue
			}
			due := true
			if s.Pulse.LastRunAt != "" {
				if last, err := time.Parse(time.RFC3339, s.Pulse.LastRunAt); err == nil {
					due = time.Since(last) >= s.Pulse.cadence()
				}
			}
			// The cadence alone says "enough time has passed"; PreferredHour
			// additionally holds off until the local clock has actually
			// reached that hour, so a daily cadence lands around the same
			// time each day instead of drifting to whenever the cadence
			// window happens to elapse (a restart, a deferred run, etc.).
			if due && s.Pulse.PreferredHourSet {
				due = time.Now().Hour() >= s.Pulse.PreferredHour
			}
			if !due {
				continue
			}
			// Stay out of the way of a conversation in progress — see
			// lastInteractiveTurn in chat.go for the measured reason.
			if quiet := sinceInteractiveTurn(); quiet < pulseQuietPeriod {
				overdue := time.Duration(0)
				if s.Pulse.LastRunAt != "" {
					if last, err := time.Parse(time.RFC3339, s.Pulse.LastRunAt); err == nil {
						overdue = time.Since(last) - s.Pulse.cadence()
					}
				}
				if overdue < pulseMaxDeferral {
					log.Printf("[pulse] deferring: family active %s ago (overdue by %s, forcing after %s)",
						quiet.Round(time.Second), overdue.Round(time.Minute), pulseMaxDeferral)
					continue
				}
				log.Printf("[pulse] running despite recent activity — overdue by %s", overdue.Round(time.Minute))
			}
			runCtx, cancel := context.WithTimeout(context.Background(), turnTimeout)
			if err := runPulseOnce(runCtx, false); err != nil {
				log.Printf("[pulse] scheduled run failed: %v", err)
			}
			cancel()
		}
	}
}

// --- HTTP routes ---------------------------------------------------------

type pulseConfigResponse struct {
	Enabled          bool     `json:"enabled"`
	CadenceHours     int      `json:"cadence_hours"`
	LastRunAt        string   `json:"last_run_at,omitempty"`
	WatchSites       []string `json:"watch_sites,omitempty"`
	PreferredHour    int      `json:"preferred_hour"`
	PreferredHourSet bool     `json:"preferred_hour_set"`
}

func pulseConfigResponseFrom(p PulseConfig) pulseConfigResponse {
	hours := p.CadenceHours
	if hours <= 0 {
		hours = 24
	}
	return pulseConfigResponse{
		Enabled:          p.Enabled,
		CadenceHours:     hours,
		LastRunAt:        p.LastRunAt,
		WatchSites:       p.Sites(),
		PreferredHour:    p.PreferredHour,
		PreferredHourSet: p.PreferredHourSet,
	}
}

// pulseRunMu prevents two manual "run now" triggers overlapping — a real
// Pulse turn already serializes on agentTurnMu once it starts, but this stops
// a second HTTP call from spawning a redundant goroutine that would just
// block, and lets the handler tell the parent plainly "already running"
// instead of silently queuing.
var pulseRunMu sync.Mutex
var pulseRunning bool

// POST /api/pulse/run — manual "run now" trigger (e.g. from the Pulse
// popover, to test it without waiting for the ticker or turning on the
// recurring schedule). Runs in the background; the caller polls
// GET /api/pulse/config and watches last_run_at change to know it's done.
func handlePulseRunNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pulseRunMu.Lock()
	if pulseRunning {
		pulseRunMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a Pulse check-in is already running"})
		return
	}
	pulseRunning = true
	pulseRunMu.Unlock()

	go func() {
		defer func() {
			pulseRunMu.Lock()
			pulseRunning = false
			pulseRunMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
		defer cancel()
		if err := runPulseOnce(ctx, true); err != nil {
			log.Printf("[pulse] manual run failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// GET /api/pulse/config
func handleGetPulseConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	writeJSON(w, http.StatusOK, pulseConfigResponseFrom(s.Pulse))
}

type setPulseConfigRequest struct {
	Enabled      *bool     `json:"enabled,omitempty"`
	CadenceHours *int      `json:"cadence_hours,omitempty"`
	WatchSites   *[]string `json:"watch_sites,omitempty"`
	// PreferredHour (0-23) and PreferredHourSet are independent so the
	// frontend can toggle "use a specific time" off without losing the hour
	// value the parent had picked, and back on without them re-entering it.
	PreferredHour    *int  `json:"preferred_hour,omitempty"`
	PreferredHourSet *bool `json:"preferred_hour_set,omitempty"`
}

// POST /api/pulse/config — partial update; only provided fields change.
func handleSetPulseConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req setPulseConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	stateMu.Lock()
	s := loadState()
	if req.Enabled != nil {
		s.Pulse.Enabled = *req.Enabled
	}
	if req.CadenceHours != nil && *req.CadenceHours > 0 {
		s.Pulse.CadenceHours = *req.CadenceHours
	}
	if req.WatchSites != nil {
		cleaned := make([]string, 0, len(*req.WatchSites))
		for _, u := range *req.WatchSites {
			if u = strings.TrimSpace(u); u != "" {
				cleaned = append(cleaned, u)
			}
		}
		s.Pulse.WatchSites = cleaned
		s.Pulse.SchoolPortalURL = "" // fully replaced by the generic list; drop the legacy single value
	}
	if req.PreferredHour != nil && *req.PreferredHour >= 0 && *req.PreferredHour <= 23 {
		s.Pulse.PreferredHour = *req.PreferredHour
	}
	if req.PreferredHourSet != nil {
		s.Pulse.PreferredHourSet = *req.PreferredHourSet
	}
	err := saveState(s)
	stateMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, pulseConfigResponseFrom(s.Pulse))
}
