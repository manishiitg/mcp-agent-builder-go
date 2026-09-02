package main

import (
	"context"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
)

// turnSession is what a parent, child, Pulse or WhatsApp turn needs from a
// coding-agent session. agentsession.Session satisfies it; tests substitute
// a fake through newAgentSession so a whole turn — prompt assembly, tool
// wiring, activity scoping, streaming, persistence — runs without a model.
type turnSession interface {
	Ask(ctx context.Context, history []agentsession.Message) (string, error)
	Send(ctx context.Context, input string) error
	Resumed() bool
	Handle() *agentsession.Handle
	Close()
}

// newAgentSession is the one place a turn obtains its session. Swapped in
// characterization tests; production always returns the real thing.
var newAgentSession = func(ctx context.Context, cfg agentsession.Config) (turnSession, error) {
	return agentsession.New(ctx, cfg)
}
