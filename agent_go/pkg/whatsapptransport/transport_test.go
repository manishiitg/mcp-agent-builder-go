package whatsapptransport

import (
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
)

func TestExtractTextCoversCaptionsAndWrappers(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		msg  *waProto.Message
		want string
	}{
		{"nil", nil, ""},
		{"plain", &waProto.Message{Conversation: str(" hello ")}, "hello"},
		{"extended", &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: str("with a link")}}, "with a link"},
		{"image caption", &waProto.Message{ImageMessage: &waProto.ImageMessage{Caption: str("what do you think?")}}, "what do you think?"},
		{"image without caption", &waProto.Message{ImageMessage: &waProto.ImageMessage{}}, ""},
		{"document caption", &waProto.Message{DocumentMessage: &waProto.DocumentMessage{Caption: str("her homework")}}, "her homework"},
		{"device sent wrapper", &waProto.Message{DeviceSentMessage: &waProto.DeviceSentMessage{Message: &waProto.Message{Conversation: str("from my laptop")}}}, "from my laptop"},
		{"edited wrapper", &waProto.Message{EditedMessage: &waProto.FutureProofMessage{Message: &waProto.Message{Conversation: str("fixed typo")}}}, "fixed typo"},
	}
	for _, c := range cases {
		if got := ExtractText(c.msg); got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestExtensionForMime(t *testing.T) {
	for mime, want := range map[string]string{"image/jpeg": ".jpg", "image/PNG": ".png", "audio/ogg; codecs=opus": ".ogg", "application/pdf": ".pdf", "video/mp4": ".mp4", "weird/thing": ".bin"} {
		if got := ExtensionForMime(mime, ".bin"); got != want {
			t.Fatalf("%s: got %s want %s", mime, got, want)
		}
	}
}

func TestDedupeWindow(t *testing.T) {
	d := NewDedupe(50 * time.Millisecond)
	if d.Seen("") {
		t.Fatal("empty id must never count as a duplicate")
	}
	if d.Seen("m1") {
		t.Fatal("first sighting is not a duplicate")
	}
	if !d.Seen("m1") {
		t.Fatal("second sighting within the window is a duplicate")
	}
	time.Sleep(80 * time.Millisecond)
	if d.Seen("m1") {
		t.Fatal("after the window the id is fresh again")
	}
}

func TestHasMediaAndVoiceNote(t *testing.T) {
	if HasMedia(&waProto.Message{}) || HasMedia(nil) {
		t.Fatal("no media expected")
	}
	audio := &waProto.Message{AudioMessage: &waProto.AudioMessage{}}
	if !HasMedia(audio) || !IsVoiceNote(audio) {
		t.Fatal("audio is downloadable media and a voice note")
	}
	if IsVoiceNote(&waProto.Message{ImageMessage: &waProto.ImageMessage{}}) {
		t.Fatal("an image is not a voice note")
	}
}
