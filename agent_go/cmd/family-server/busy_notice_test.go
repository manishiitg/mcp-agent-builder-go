package main

import (
	"testing"
	"time"
)

func TestWhatsappBusyNotice(t *testing.T) {
	lastBusyNotice.at = time.Time{}
	if got := whatsappBusyNotice("", 0, false); got != "" {
		t.Fatalf("idle should stay quiet, got %q", got)
	}
	lastBusyNotice.at = time.Time{}
	if got := whatsappBusyNotice("pulse", 2*time.Second, true); got != "" {
		t.Fatalf("a just-started turn should stay quiet, got %q", got)
	}
	lastBusyNotice.at = time.Time{}
	first := whatsappBusyNotice("pulse", 30*time.Second, true)
	if first == "" {
		t.Fatal("a running pulse should produce a notice")
	}
	if second := whatsappBusyNotice("pulse", 40*time.Second, true); second != "" {
		t.Fatalf("burst should be debounced, got %q", second)
	}
	lastBusyNotice.at = time.Now().Add(-6 * time.Minute)
	if got := whatsappBusyNotice("parent", 30*time.Second, true); got == "" {
		t.Fatal("should notify again after the gap")
	}
}
