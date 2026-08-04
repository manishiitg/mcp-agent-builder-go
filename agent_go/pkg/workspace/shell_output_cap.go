package workspace

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// defaultAgentShellOutputBytes bounds what a single shell result shows an agent.
//
// Nothing on this path capped anything: workspace/handlers/shell.go returns
// stdoutBuf.String() whole, and mcpagent's 100KB maxOutputBytes guards a
// different executor the builder never calls. So a coding CLI received the full
// payload and rejected it against its own token cap — 12 times in 5h37m on
// 2026-08-04, at 67,930 to 130,046 characters.
//
// The number is chosen against the smallest observed rejection (67,930
// characters, "across 1 line"). Single-line output tokenizes at roughly 2.7
// characters per token, so 48,000 characters is about 18k tokens — under a
// 25k-token result cap even in that worst case, and about 12k tokens for
// ordinary prose.
const defaultAgentShellOutputBytes = 48000

// agentShellStderrReserve keeps room for stderr even when stdout is enormous.
// stderr is where the reason for a failure lives, and it is the part an agent
// most needs; a big stdout must not push it out of the result.
const agentShellStderrReserve = 8000

// agentShellOutputBytes reads the cap, allowing an operator to tune it for a
// consumer with a different limit without a rebuild. A non-positive value
// disables capping.
func agentShellOutputBytes() int {
	raw := strings.TrimSpace(os.Getenv("SHELL_MAX_AGENT_OUTPUT_BYTES"))
	if raw == "" {
		return defaultAgentShellOutputBytes
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultAgentShellOutputBytes
	}
	return parsed
}

// capShellResultForAgent bounds a shell result that an LLM will read.
//
// It is applied in the tool executor, not in Client.ExecuteShellCommand, because
// scripted steps call the client directly and parse stdout as schema-validated
// JSON (controller_scripted.go). Truncating there would corrupt output that is
// correct.
//
// Truncation keeps both ends. An agent usually needs either the beginning (what
// ran) or the end (how it finished), and dropping the tail turns "the command
// worked" into "the command produced nothing conclusive". The marker between
// them states the real size and names the ways to produce less, because an agent
// told only that output was truncated re-runs the identical command — three of
// the 2026-08-04 rejections were byte-identical repeats.
func capShellResultForAgent(result ShellCommandResult) ShellCommandResult {
	limit := agentShellOutputBytes()
	if limit <= 0 {
		return result
	}
	if len(result.Stdout)+len(result.Stderr) <= limit {
		return result
	}

	stderrBudget := agentShellStderrReserve
	if stderrBudget > limit/2 {
		stderrBudget = limit / 2
	}
	if len(result.Stderr) < stderrBudget {
		stderrBudget = len(result.Stderr)
	}
	result.Stderr = truncateShellStream(result.Stderr, stderrBudget, "stderr")
	result.Stdout = truncateShellStream(result.Stdout, limit-len(result.Stderr), "stdout")
	return result
}

// truncateShellStream keeps the head and tail of s within budget, replacing the
// middle with a marker that says how much was dropped and what to do about it.
func truncateShellStream(s string, budget int, stream string) string {
	if budget < 0 {
		budget = 0
	}
	if len(s) <= budget {
		return s
	}

	marker := fmt.Sprintf(
		"\n\n... [%s truncated: %d of %d characters omitted. Do NOT re-run this command unchanged — it will be truncated identically. "+
			"Produce less: filter with grep, slice with head/tail or sed -n '<start>,<end>p', select fields with jq/awk, "+
			"or write the full output to a file under your working directory and read it back in pieces.] ...\n\n",
		stream, len(s)-budget, len(s),
	)
	// A budget too small to hold the marker plus useful text on both sides is
	// better spent on the marker alone: the agent still learns why it is short
	// and what to do, which is the part it acts on.
	if budget <= len(marker) {
		return marker
	}

	remaining := budget - len(marker)
	head := remaining * 2 / 3
	tail := remaining - head
	return s[:head] + marker + s[len(s)-tail:]
}
