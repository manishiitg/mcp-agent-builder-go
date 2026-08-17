package server

import "testing"

func TestProductEnabled(t *testing.T) {
	t.Setenv("AGENT_PRODUCTS", "")
	if !productEnabled("video-studio") || !productEnabled("chief-of-staff") {
		t.Fatal("an unset product allowlist must preserve shared-server behavior")
	}

	t.Setenv("AGENT_PRODUCTS", " video-studio, Finance ")
	if !productEnabled("video-studio") || !productEnabled("finance") {
		t.Fatal("configured products must be enabled case-insensitively")
	}
	if productEnabled("chief-of-staff") {
		t.Fatal("unlisted product must not be registered")
	}
}
