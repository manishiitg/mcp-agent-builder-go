package main

import (
	"encoding/json"
	"os"
	"strings"
)

// WhatsApp is one thread with two possible targets for an incoming photo:
// the parent conversation (default) or whichever activity the child is
// currently bound to. "@child" / "@parent" (see extractModeSwitch in
// whatsapp_bot.go) flip a PERSISTENT mode — once switched, every image
// message goes to that target until switched back, so a parent snapping
// several photos of a notebook mid-activity doesn't have to retype the tag
// each time. Plain text messages are unaffected by this mode; only image/
// document attachments route differently — see handleIncomingMessage.
const (
	waRoutingModeChild  = "child"
	waRoutingModeParent = "parent"
)

func waRoutingModePath() (string, bool) { return resolveWorkspacePath("whatsapp-routing.json") }

// loadWaRoutingMode defaults to "parent" whenever the file is missing or
// unreadable — the safe default is the one that behaves exactly like before
// this feature existed.
func loadWaRoutingMode() string {
	abs, ok := waRoutingModePath()
	if !ok {
		return waRoutingModeParent
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return waRoutingModeParent
	}
	var v struct {
		Mode string `json:"mode"`
	}
	if json.Unmarshal(b, &v) != nil || strings.TrimSpace(v.Mode) != waRoutingModeChild {
		return waRoutingModeParent
	}
	return waRoutingModeChild
}

func setWaRoutingMode(mode string) {
	abs, ok := waRoutingModePath()
	if !ok {
		return
	}
	b, _ := json.Marshal(struct {
		Mode string `json:"mode"`
	}{Mode: mode})
	_ = os.WriteFile(abs, b, 0o600)
}
