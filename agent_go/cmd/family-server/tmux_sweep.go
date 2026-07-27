package main

import (
	"context"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// codingAgentTmuxPrefixes are family-server's OWN naming prefixes — set via
// the *_SESSION_PREFIX env vars in main.go, deliberately NOT
// multi-llm-provider-go's shared "mlp-*" default, which AgentWorks also uses
// unmodified. Matching "mlp-*" here would let this sweep kill a live
// AgentWorks session on the same machine; matching only "sq-*" makes that
// impossible regardless of what else is running.
var codingAgentTmuxPrefixes = []string{
	"sq-cursor-cli-int",
	"sq-claude-code",
	"sq-codex-cli-int",
	"sq-pi-cli-int",
}

func isCodingAgentTmuxSessionName(name string) bool {
	for _, p := range codingAgentTmuxPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// sweepOrphanedCodingAgentTmuxSessions kills leftover tmux sessions from a
// PAST family-server process — confirmed live: 32 accumulated after a day of
// restarts during development. Every warm session is tracked only in this
// process's own memory (agentsession.warmOwners, plus each adapter's own
// registry); when the process restarts, whatever was warm at that moment is
// orphaned forever — nothing left running remembers it exists to ever close
// it, even though the tmux pane and its cursor-agent/claude/codex/pi process
// keep running indefinitely.
//
// family-server is single-instance and single-family (unlike AgentWorks,
// which tags ownership by PID in tmux's own session options because several
// of its instances can run at once) — there is never a second SAME-process
// instance that could legitimately still own one of these sessions, so one
// signal is enough: how long has the pane been completely silent.
//
//   - minIdleAge=0 (the STARTUP call, before this process has served a single
//     request) kills every matching session unconditionally — nothing could
//     possibly be legitimate yet, the in-memory registry is always empty at
//     boot.
//   - A non-zero minIdleAge (the PERIODIC call, while this same process keeps
//     running for days) only kills sessions far quieter than any real turn or
//     the adapter's own idle-timeout should ever allow — catching the rarer
//     case where a close attempt's own `tmux kill-session` call itself failed
//     silently (the adapter discards that error) and left a real process
//     behind even without a restart.
func sweepOrphanedCodingAgentTmuxSessions(minIdleAge time.Duration) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_activity}").Output()
	if err != nil {
		// Most common case: no tmux server running at all (nothing to sweep) —
		// tmux exits non-zero then. Not worth logging as a failure.
		return
	}
	now := time.Now()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || !isCodingAgentTmuxSessionName(parts[0]) {
			continue
		}
		name := parts[0]
		activityUnix, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			continue
		}
		idleFor := now.Sub(time.Unix(activityUnix, 0))
		if idleFor < minIdleAge {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		killErr := exec.CommandContext(ctx, "tmux", "kill-session", "-t", name).Run()
		cancel()
		if killErr != nil {
			log.Printf("[tmux-sweep] failed to kill orphaned session %q: %v", name, killErr)
			continue
		}
		log.Printf("[tmux-sweep] killed orphaned coding-agent tmux session %q (idle %s)", name, idleFor.Round(time.Second))
	}
}

// startTmuxSweepLoop runs the startup sweep once immediately, then repeats
// hourly for the lifetime of the process — see sweepOrphanedCodingAgentTmuxSessions's
// own comment for why continuous operation still needs the periodic half.
// The 45-minute idle threshold sits comfortably above any real turn (minutes,
// not tens of minutes) and above the adapter's own idle-timeout, so this
// never races a session that's still legitimately alive.
func startTmuxSweepLoop() {
	sweepOrphanedCodingAgentTmuxSessions(0)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			sweepOrphanedCodingAgentTmuxSessions(45 * time.Minute)
		}
	}()
}
