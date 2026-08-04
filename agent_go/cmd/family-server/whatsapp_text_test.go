package main

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
)

// Captions are message text. Attaching photos and typing underneath is the
// normal way a parent asks about them, and WhatsApp delivers that as an
// ImageMessage carrying Caption rather than a separate text message. Reading
// only Conversation/ExtendedTextMessage dropped it silently: empty text made
// the handler file the media and return without running a turn.
func TestExtractWhatsAppMessageText(t *testing.T) {
	str := func(s string) *string { return &s }

	cases := []struct {
		name string
		msg  *waProto.Message
		want string
	}{
		{"nil", nil, ""},
		{"plain text", &waProto.Message{Conversation: str("hello")}, "hello"},
		{"extended text", &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: str("with a link")},
		}, "with a link"},
		{"image caption", &waProto.Message{
			ImageMessage: &waProto.ImageMessage{Caption: str("what do you think?")},
		}, "what do you think?"},
		{"image without caption", &waProto.Message{
			ImageMessage: &waProto.ImageMessage{},
		}, ""},
		{"video caption", &waProto.Message{
			VideoMessage: &waProto.VideoMessage{Caption: str("her working")},
		}, "her working"},
		{"document caption", &waProto.Message{
			DocumentMessage: &waProto.DocumentMessage{Caption: str("the worksheet")},
		}, "the worksheet"},
		{"caption is trimmed", &waProto.Message{
			ImageMessage: &waProto.ImageMessage{Caption: str("  spaced  ")},
		}, "spaced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractWhatsAppMessageText(tc.msg); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
