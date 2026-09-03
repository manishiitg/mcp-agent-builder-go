package whatsappbot

import (
	"context"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/whatsapptransport"
)

// Message is one accepted inbound WhatsApp message, with the helpers a
// product needs to acknowledge and answer it.
type Message struct {
	conn *Connector

	Account   *whatsapptransport.Account
	Event     *events.Message
	Chat      types.JID
	Sender    types.JID
	ID        types.MessageID
	PushName  string
	Timestamp time.Time
	FromMe    bool
	SelfChat  bool
	HasMedia  bool

	// RawText is the message body as received; Text has the routing mention
	// removed (they are equal when no mention was found).
	RawText string
	Text    string

	// Route is the resolved destination: the mention in this message, else
	// the chat's remembered route, else nil.
	Route *Route
	// Switched is true when this very message carried the mention that
	// activated Route.
	Switched bool
}

// Proto returns the underlying protobuf message (nil-safe).
func (m *Message) Proto() *waProto.Message {
	if m == nil || m.Event == nil {
		return nil
	}
	return m.Event.Message
}

// IsVoiceNote reports whether the message is an audio message.
func (m *Message) IsVoiceNote() bool { return whatsapptransport.IsVoiceNote(m.Proto()) }

// Caption returns the media caption, if any.
func (m *Message) Caption() string { return whatsapptransport.ExtractCaption(m.Proto()) }

// React sets the bot's reaction on this message; failures are logged only.
func (m *Message) React(emoji string) {
	if m == nil || m.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.conn.React(ctx, m.Chat, m.Sender, m.ID, emoji); err != nil {
		m.conn.logf("reaction %q failed: %v", emoji, err)
	}
}

// ClearReaction removes the bot's reaction.
func (m *Message) ClearReaction() { m.React("") }

// Pending shows the "still working" reaction once delay has elapsed. The
// returned stop func cancels the timer; call it as soon as the answer is
// ready (it does not clear a reaction that was already shown).
func (m *Message) Pending(emoji string, delay time.Duration) (stop func()) {
	t := time.AfterFunc(delay, func() { m.React(emoji) })
	return func() { t.Stop() }
}

// Reply sends text into this message's chat, retrying a few times.
func (m *Message) Reply(text string) error {
	if m == nil || m.conn == nil {
		return nil
	}
	return m.conn.SendTextWithRetry(m.Chat, text, 3, 30*time.Second)
}

// ReplyOnce sends text into this message's chat with a single attempt.
func (m *Message) ReplyOnce(ctx context.Context, text string) error {
	if m == nil || m.conn == nil {
		return nil
	}
	return m.conn.SendText(ctx, m.Chat, text)
}

// Download fetches the attachment; maxBytes <= 0 means unlimited.
func (m *Message) Download(ctx context.Context, maxBytes int) (*whatsapptransport.Media, error) {
	if m == nil || m.Account == nil {
		return nil, whatsapptransport.ErrNoMedia
	}
	return m.Account.Download(ctx, m.Proto(), maxBytes)
}
