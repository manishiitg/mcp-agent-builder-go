package main

import (
	"context"
	"testing"
)

type recordingSender struct{ got []string }

func (r *recordingSender) Send(_ context.Context, msg string) error {
	r.got = append(r.got, msg)
	return nil
}

// The single-slot registry had a real defect independent of parallelism: any
// finishing turn cleared whatever was registered, so with two turns live the
// first to finish silently disabled steering for the second.
func TestClearActiveTurnOnlyClearsItsOwnConversation(t *testing.T) {
	activeTurns = map[string]activeTurnSender{}
	parent, child := &recordingSender{}, &recordingSender{}

	registerActiveTurn("parent", parent)
	registerActiveTurn("activities/maths", child)

	clearActiveTurn("parent")

	if _, ok := activeTurns["parent"]; ok {
		t.Fatal("parent should be deregistered")
	}
	if _, ok := activeTurns["activities/maths"]; !ok {
		t.Fatal("the child's turn must survive the parent's completion")
	}
}

// Steering must reach the turn for THAT conversation, never a bystander.
func TestSteerRoutesToTheRightConversation(t *testing.T) {
	activeTurns = map[string]activeTurnSender{}
	parent, child := &recordingSender{}, &recordingSender{}
	registerActiveTurn("parent", parent)
	registerActiveTurn("activities/maths", child)

	if !trySteer(context.Background(), "activities/maths", "hello") {
		t.Fatal("expected the steer to be delivered")
	}
	if len(child.got) != 1 || child.got[0] != "hello" {
		t.Fatalf("child should have received it, got %v", child.got)
	}
	if len(parent.got) != 0 {
		t.Fatalf("parent must not receive another conversation's steer, got %v", parent.got)
	}
}

func TestSteerWithNoTurnForThatConversation(t *testing.T) {
	activeTurns = map[string]activeTurnSender{}
	registerActiveTurn("parent", &recordingSender{})
	if trySteer(context.Background(), "activities/science", "hi") {
		t.Fatal("must not steer into a conversation that has no turn running")
	}
}
