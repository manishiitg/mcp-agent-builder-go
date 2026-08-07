package main

import (
	"context"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// codingAgentTmuxPrefixes are Video Studio's OWN naming prefixes — set via the
// *_SESSION_PREFIX env vars in main.go, deliberately NOT multi-llm-provider-go's
// shared "mlp-*" default that AgentWorks uses unmodified, nor family-server's
// "sq-*". Matching another product's prefix here would let this sweep kill a
// live session belonging to it on the same machine; matching only "video-*"
// makes that impossible regardless of what else is running.
var codingAgentTmuxPrefixes = []string{
	"video-claude-code",
	"video-cursor-cli-int",
	"video-codex-cli-int",
	"video-pi-cli-int",
}

func isCodingAgentTmuxSessionName(name string) bool {
	for _, p := range codingAgentTmuxPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// sweepOrphanedCodingAgentTmuxSessions kills leftover tmux sessions from a PAST
// video-server process. Every warm session is tracked only in that process's own
// memory; when it restarts, whatever was warm at that moment is orphaned
// forever — nothing left running remembers it exists to ever close it, while
// the pane and its claude process keep running indefinitely.
//
// Video Studio is single-instance and local, so one signal is enough: how long
// the pane has been completely silent.
//
//   - minIdleAge=0 (the STARTUP call, before this process has served a request)
//     kills every matching session unconditionally — nothing could be
//     legitimate yet, the in-memory registry is always empty at boot.
//   - A non-zero minIdleAge (the PERIODIC call) only kills sessions far quieter
//     than any real turn should allow, catching the rarer case where a close
//     attempt's own kill-session failed silently and left a process behind
//     without a restart.
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
// hourly for the lifetime of the process. The 45-minute idle threshold sits
// comfortably above any real turn and above the adapter's own idle-timeout, so
// it never races a session that is still legitimately alive.
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
