package main

import (
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// turnTrace measures where a single agent turn's wall-clock time actually goes,
// from the moment family-server starts handling the message to the moment the
// reply is complete.
//
// Why this exists separately from mcpagent's own [LATENCY_DEBUG] lines: those
// start at AskWithHistory and report one `llm_duration` for the whole provider
// call. For a coding-CLI provider that single number silently bundles several
// very different costs — spawning tmux and booting the CLI on a cold start, the
// CLI reconstructing its own session, actual model generation, and (for
// transcript-tailing providers) the poll interval before text is even visible to
// us. A two-minute turn looks identical whether the model thought for two
// minutes or the CLI took ninety seconds to come up.
//
// So this records the boundaries family-server itself can see, and is
// deliberately provider-agnostic: every surface (web chat, child chat, WhatsApp,
// Pulse) and every engine (codex/claude/cursor/pi) goes through the same helper,
// so the numbers are comparable across engines rather than only existing for
// whichever one someone last debugged.
//
// Emits ONE summary line per turn, e.g.:
//
//	[TURN] parent engine=cursor-cli session=cold total=124310ms setup=8102ms ttft=none first_tool=41208ms(execute_shell_command) tools=3 reply=980ch err=<nil>
//
// Reading it: a large `setup` means session/CLI startup, not the model. `ttft`
// (time to first streamed character) separates "the model is slow" from "we
// can't see output until the end" — `ttft=none` means no streaming arrived at
// all this turn, which is itself the finding (see the pi-cli note in
// docs/backlog.md). `first_tool` shows how long before the agent did anything
// observable.
type turnTrace struct {
	surface string // parent | child | pulse — which surface drove this turn
	engine  string
	start   time.Time

	mu            sync.Mutex
	setup         time.Duration // until the session was ready to Ask
	resumed       bool
	ttft          time.Duration // until the first streamed delta
	sawDelta      bool
	firstTool     time.Duration
	firstToolName string
	tools         int
	// byTool counts calls per tool name. A turn spending minutes on 20+ calls
	// looks the same in a bare total whether it did 20 different useful things
	// or retried one tool 20 times — and those need opposite fixes.
	byTool map[string]int
}

func newTurnTrace(surface, engine string) *turnTrace {
	return &turnTrace{
		surface: surface,
		engine:  strings.TrimSpace(engine),
		start:   time.Now(),
		byTool:  map[string]int{},
	}
}

// sessionReady marks agentsession.New having returned, and whether it resumed an
// existing coding-agent session or cold-started one.
func (t *turnTrace) sessionReady(resumed bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setup = time.Since(t.start)
	t.resumed = resumed
}

// delta marks streamed reply text arriving. Only the first one is timed; called
// on every chunk, so it must stay cheap.
func (t *turnTrace) delta() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.sawDelta {
		t.sawDelta = true
		t.ttft = time.Since(t.start)
	}
}

// tool marks a tool call starting. Every call counts; only the first is timed.
func (t *turnTrace) tool(name string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tools++
	t.byTool[name]++
	if t.firstToolName == "" {
		t.firstToolName = name
		t.firstTool = time.Since(t.start)
	}
}

// toolBreakdown renders the per-tool counts, busiest first, e.g.
// "execute_shell_command x18,set_secret x4,list_secrets x1".
func (t *turnTrace) toolBreakdown() string {
	if len(t.byTool) == 0 {
		return "-"
	}
	names := make([]string, 0, len(t.byTool))
	for name := range t.byTool {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if t.byTool[names[i]] != t.byTool[names[j]] {
			return t.byTool[names[i]] > t.byTool[names[j]]
		}
		return names[i] < names[j] // stable for equal counts
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" x"+strconv.Itoa(t.byTool[name]))
	}
	return strings.Join(parts, ",")
}

// finish logs the one summary line for this turn.
func (t *turnTrace) finish(reply string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	session := "cold"
	if t.resumed {
		session = "warm"
	}
	ttft := "none" // no streamed text ever arrived — a real finding, not zero
	if t.sawDelta {
		ttft = ms(t.ttft)
	}
	firstTool := "none"
	if t.firstToolName != "" {
		firstTool = ms(t.firstTool) + "(" + t.firstToolName + ")"
	}
	log.Printf("[TURN] %s engine=%s session=%s total=%s setup=%s ttft=%s first_tool=%s tools=%d [%s] reply=%dch err=%v",
		t.surface, t.engine, session, ms(time.Since(t.start)), ms(t.setup),
		ttft, firstTool, t.tools, t.toolBreakdown(), len([]rune(reply)), err)
}

func ms(d time.Duration) string {
	return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
}
