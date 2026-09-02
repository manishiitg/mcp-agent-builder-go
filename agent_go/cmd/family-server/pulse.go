package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/enginedetect"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/productschedule"
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
		trigger: "Automated check-in — reviewing the weekly schedule",
		instruction: "This is an automated Pulse check-in, focused ONLY on " + who + "'s recurring weekly schedule and whether it's " +
			"actually being followed — two things, in one check, since they share the same two facts below. " +
			"CURRENT SAVED SCHEDULE: " + formatScheduleForPulse(s.Schedule.Entries) + ". " +
			"WHAT ACTUALLY HAPPENED THE LAST 7 DAYS (activity log): " + recentActivityLogForPulse(7) + ". " +
			"FIRST — maintain: look back over recent conversations (conversations/parent.json, and any child activity " +
			"conversations) for a recurring weekly commitment mentioned that ISN'T already in the saved schedule above " +
			"— a new class, a season starting, tuition added, a practice time that changed. If you find one, save it " +
			"with set_child_schedule; it silently skips anything that exactly matches an existing entry, so it's fine " +
			"to call even if you're not fully sure it's new. Do NOT invent a commitment that was never actually " +
			"mentioned. SECOND — cross-check: compare the saved schedule against the last 7 days above. If a " +
			"recurring slot clearly meant for study/schoolwork (not something like a sports practice — those aren't " +
			"expected to show activity) had NOTHING logged against it this week, that's worth a gentle, specific " +
			"mention (which slot, which day) — not an accusation, just useful visibility. If nothing stands out on " +
			"either front, say so briefly rather than manufacturing a finding." + pulseReplyRules,
	})

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

// formatScheduleForPulse renders the saved recurring schedule as plain text
// for a Pulse check's instruction, so the model can see what's already there
// without having to go read+parse memory/child-schedule.json itself.
func formatScheduleForPulse(entries []ScheduleEntry) string {
	if len(entries) == 0 {
		return "(none saved yet)"
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, e.Day+" "+e.Start+"-"+e.End+" "+e.Label)
	}
	return strings.Join(parts, "; ")
}

// recentActivityLogForPulse renders activity-log.json entries from the last
// `days` days as plain text, for cross-checking against the schedule.
func recentActivityLogForPulse(days int) string {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	var parts []string
	for _, e := range loadActivityLog() {
		if e.Date >= cutoff {
			parts = append(parts, e.Date+" "+e.Title)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("(nothing logged in the last %d days)", days)
	}
	return strings.Join(parts, "; ")
}

// upcomingDeadlinesForPulse filters+sorts deadlines due within the next
// `days` days (inclusive of today), soonest first.
func upcomingDeadlinesForPulse(deadlines []SchoolDeadline, days int) []SchoolDeadline {
	today := time.Now().Format("2006-01-02")
	until := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	var out []SchoolDeadline
	for _, d := range deadlines {
		if d.DueDate >= today && d.DueDate <= until {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueDate < out[j].DueDate })
	return out
}

// formatDeadlinesForPulse renders deadlines as plain text for a Pulse check's
// instruction.
func formatDeadlinesForPulse(deadlines []SchoolDeadline) string {
	parts := make([]string, 0, len(deadlines))
	for _, d := range deadlines {
		kind := d.Kind
		if kind == "" {
			kind = "assignment"
		}
		subj := ""
		if d.Subject != "" {
			subj = " (" + d.Subject + ")"
		}
		parts = append(parts, d.DueDate+" "+kind+": "+d.Title+subj)
	}
	return strings.Join(parts, "; ")
}

// recentActivityBySubjectForPulse renders activity-log.json entries from the
// last `days` days as plain text, each tagged with its inferred subject (the
// activity dir's first path segment, e.g. "Mathematics/Lines and
// Angles/..." -> "Mathematics") — an approximate match, good enough for the
// model to reason over rather than a precise join, since deadlines and
// activities come from two entirely separate, loosely-related sources.
func recentActivityBySubjectForPulse(days int) string {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	var parts []string
	for _, e := range loadActivityLog() {
		if e.Date < cutoff {
			continue
		}
		subject := e.ActivityDir
		if i := strings.Index(subject, "/"); i >= 0 {
			subject = subject[:i]
		}
		parts = append(parts, e.Date+" "+subject+": "+e.Title)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("(nothing logged in the last %d days)", days)
	}
	return strings.Join(parts, "; ")
}

// pulseSchedule is Pulse expressed as a product schedule: the parent's
// settings (enabled, cadence, preferred hour) plus the fixed quiet rule, with
// this cycle's check-ins as its messages. The same definition shape a
// product declares in product.yaml, so moving Pulse onto the platform
// scheduler later is a data move, not a rewrite.
func pulseSchedule(s familyState) productschedule.Schedule {
	hours := s.Pulse.CadenceHours
	if hours <= 0 {
		hours = 24
	}
	sched := productschedule.Schedule{
		ID:               "pulse",
		Name:             "Check-in",
		Description:      "Quill reviews recent learning, watched sites and upcoming deadlines, then sends the parent a summary.",
		Enabled:          s.Pulse.Enabled,
		CadenceHours:     hours,
		QuietMinutes:     int(pulseQuietPeriod / time.Minute),
		MaxDeferralHours: int(pulseMaxDeferral / time.Hour),
	}
	if s.Pulse.PreferredHourSet {
		hour := s.Pulse.PreferredHour
		sched.PreferredHour = &hour
	}
	for _, c := range pulseChecks(s) {
		sched.Messages = append(sched.Messages, c.instruction)
	}
	return sched
}

// pulseQuietPeriod is how long the family must have been idle before a
// scheduled Pulse may start. Pulse holds the agent for 25-250s per check and
// runs several back to back, so starting one while someone is mid-conversation
// pushes their reply minutes into the future.
const pulseQuietPeriod = 10 * time.Minute

// pulseMaxDeferral stops "wait for quiet" becoming "never run". A family using
// the app all evening would otherwise defer Pulse indefinitely and silently
// lose their check-ins; past this much overdue it runs regardless.
const pulseMaxDeferral = 4 * time.Hour

func pulseLastRun() time.Time {
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if s.Pulse.LastRunAt == "" {
		return time.Time{}
	}
	last, err := time.Parse(time.RFC3339, s.Pulse.LastRunAt)
	if err != nil {
		return time.Time{}
	}
	return last
}

// pulseRunner drives Pulse on the shared schedule runtime: the ticker, the
// due/quiet decision, single-flight and per-check status all live there.
var pulseRunner = func() *productschedule.Runner {
	r, err := productschedule.NewRunner(productschedule.Host{
		Name: "pulse",
		Schedule: func() productschedule.Schedule {
			stateMu.Lock()
			s := loadState()
			stateMu.Unlock()
			return pulseSchedule(s)
		},
		LastRun:          pulseLastRun,
		SinceInteractive: sinceInteractiveTurn,
		Begin:            beginPulseRun,
		Timeout:          turnTimeout,
		Tick:             5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}
	return r
}()

// pulseRun is one Pulse cycle: every check runs as its OWN sequential agent
// turn on the single parent conversation, persisting each as its own visible
// message before moving to the next — so the parent sees distinct check-ins,
// and if the process dies mid-cycle the checks that already completed are
// still saved.
type pulseRun struct {
	state    familyState
	provider llm.Provider
	turns    []productschedule.Turn
	checks   []pulseCheck
	messages []enginedetect.ChatMessage
	release  func()
}

// beginPulseRun prepares a cycle. Manual runs ignore the enabled toggle (the
// runner only consults it for scheduled ticks), so testing Pulse does not
// require turning the recurring schedule on first.
func beginPulseRun(_ context.Context, _ productschedule.Source) (productschedule.Run, error) {
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if s.Engine == "" {
		return nil, fmt.Errorf("no learning engine selected")
	}
	if s.Child == nil {
		return nil, fmt.Errorf("no child profile set up yet")
	}
	provider, ok := engineToProvider(s.Engine)
	if !ok {
		return nil, fmt.Errorf("engine %q has no provider mapping", s.Engine)
	}

	// Mechanical housekeeping, not an agent turn — see archive.go.
	archiveStaleActivities()

	// Pulse checks in on the SINGLE parent conversation (same file + warm tmux
	// session as the web chat and WhatsApp) — one unified thread, not a separate
	// Pulse channel. Parent and every child activity share the same physical
	// workspace, so starting a session here would force-close whatever OTHER
	// session is currently warm; skip the cycle instead.
	convID := parentConversationID
	if agentsession.HasOtherWarmInteractiveSession(convID) {
		return nil, fmt.Errorf("%w: another session is currently active on the shared workspace", productschedule.ErrSkip)
	}

	existing, _ := loadStoredConversation("parent", convID)

	checks := pulseChecks(s)
	if deadlines := loadSchoolDeadlines(); len(deadlines) > 0 {
		if upcoming := upcomingDeadlinesForPulse(deadlines, 14); len(upcoming) > 0 {
			checks = append(checks, pulseDeadlineCheck(s, upcoming))
		}
	}
	checks = append(checks, pulseNotifyCheck)

	run := &pulseRun{state: s, provider: provider, checks: checks, messages: existing.Messages}
	for _, c := range checks {
		run.turns = append(run.turns, productschedule.Turn{Label: c.trigger, Message: c.instruction})
	}
	agentTurnMu.Lock()
	clearHolder := markAgentTurnStart("pulse")
	run.release = func() {
		clearHolder()
		agentTurnMu.Unlock()
	}
	return run, nil
}

func (r *pulseRun) Turns() []productschedule.Turn { return r.turns }

func (r *pulseRun) Send(ctx context.Context, i int, _ productschedule.Turn) (string, error) {
	reply, err := runPulseCheckTurn(ctx, r.provider, r.state, parentConversationID, r.messages, r.checks[i])
	if err != nil {
		return "", err
	}
	r.messages = appendPulseTurn(r.messages, r.checks[i].trigger, reply)
	persistConversation("parent", parentConversationID, r.messages)
	return reply, nil
}

func (r *pulseRun) Finish(_ context.Context, err error) {
	r.release()
	if err != nil {
		return
	}
	stateMu.Lock()
	cur := loadState()
	cur.Pulse.LastRunAt = time.Now().UTC().Format(time.RFC3339)
	_ = saveState(cur)
	stateMu.Unlock()
}

func pulseDeadlineCheck(s familyState, upcoming []SchoolDeadline) pulseCheck {
	who := "the child"
	if s.Child != nil && strings.TrimSpace(s.Child.Name) != "" {
		who = s.Child.Name
	}
	return pulseCheck{
		trigger: "Automated check-in — checking deadline readiness",
		instruction: "This is an automated Pulse check-in, focused ONLY on whether " + who + " is actually ready for what's " +
			"coming up soon — not just whether it's on the calendar. UPCOMING (next 14 days): " + formatDeadlinesForPulse(upcoming) + ". " +
			"RECENT PRACTICE BY SUBJECT (last 14 days, from the activity log): " + recentActivityBySubjectForPulse(14) + ". " +
			"For each upcoming item, judge whether there's been recent practice in that SAME subject — if a test or " +
			"assignment is close and you see little or no matching recent activity, that's worth mentioning specifically " +
			"(which one, when it's due, what subject). You MAY prepare study material or a practice test for a genuine " +
			"gap (skills/create-study-material/SKILL.md, skills/create-test/SKILL.md) — but do NOT create or hand off an " +
			"activity for it; nothing gets handed to " + who + " without the parent explicitly asking, so just mention " +
			"what you made. If everything upcoming already has reasonable recent practice, say so briefly rather than " +
			"manufacturing a gap." + pulseReplyRules,
	}
}

var pulseNotifyCheck = pulseCheck{
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

	sess, err := newAgentSession(ctx, agentsession.Config{
		Provider:                  provider,
		ModelID:                   selectedModelID(s, provider),
		ReasoningEffort:           selectedReasoningEffort(s.FastMode, provider),
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
func handlePulseRunNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if pulseRunner.Status().Running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a Pulse check-in is already running"})
		return
	}
	go func() {
		if err := pulseRunner.RunNow(context.Background(), productschedule.SourceManual); err != nil && !errors.Is(err, productschedule.ErrAlreadyRunning) {
			log.Printf("[pulse] manual run failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// handlePulseStatus reports the running/last cycle with per-check status.
func handlePulseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, pulseRunner.Status())
}

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
