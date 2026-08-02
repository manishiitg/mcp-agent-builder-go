package main

import (
	"os"
	"strings"

	"github.com/manishiitg/mcpagent/llm"
)

// experimentCodingAgentTransport lets a one-off run A/B the two coding-agent
// process transports (see agentsession.Config.Transport /
// mcpagent.RuntimeConfig.Coding.Transport) without a code change on either side:
// set FAMILY_CODING_TRANSPORT=structured before starting family-server to run
// every turn (parent and child both) over the CLI's one-shot JSON mode
// instead of the default tmux pane. Empty/unset/anything else keeps the
// default (tmux). Not read anywhere else — this is a manual test knob, not a
// persisted setting.
func experimentCodingAgentTransport() llm.CodingAgentTransport {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FAMILY_CODING_TRANSPORT"))) {
	case "structured", "json":
		return llm.CodingAgentTransportStructured
	default:
		return ""
	}
}

// experimentSteeringDisabled lets a one-off run test parent/child behavior
// with live mid-turn steering turned off entirely: set
// FAMILY_DISABLE_STEER=1 before starting family-server. trySteer checks this
// first and refuses every steer attempt (falling back to the frontend's
// existing queue-until-next-turn behavior), regardless of whether a turn is
// actually in flight. Not read anywhere else — a manual test knob, not a
// persisted setting.
func experimentSteeringDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FAMILY_DISABLE_STEER")))
	return v == "1" || v == "true" || v == "yes"
}
