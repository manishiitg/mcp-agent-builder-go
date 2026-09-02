package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/enginedetect"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/whatsappbot"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/whatsapptransport"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
)

// whatsappPairingTimeout bounds how long a pairing attempt waits for WhatsApp
// to issue its first QR code before the next status poll starts a fresh one.
const whatsappPairingTimeout = 30 * time.Second

// waBot is SparkQuill's side of the shared WhatsApp connector
// (pkg/whatsappbot). The connector owns pairing, reconnects, dedupe, the
// "@child"/"@parent" mention routing and the acknowledgement plumbing; what
// lives here is only this product's policy: two routes (the parent
// conversation and the child's current activity), attachments into the inbox
// or that activity, voice notes transcribed locally, plain-text replies.
type waBot struct {
	mu   sync.RWMutex
	conn *whatsappbot.Connector

	// pendingUploads names inbox files that arrived with no caption and are
	// therefore still unfiled; the next real turn is told about them
	// explicitly (see runTurn) instead of relying on the agent to notice.
	pendingUploadsMu sync.Mutex
	pendingUploads   []string
}

var whatsAppBot = &waBot{}

func (w *waBot) connector() *whatsappbot.Connector {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.conn
}

func (w *waBot) noteUpload(relPath string) {
	w.pendingUploadsMu.Lock()
	w.pendingUploads = append(w.pendingUploads, relPath)
	w.pendingUploadsMu.Unlock()
}

func (w *waBot) drainPendingUploads() []string {
	w.pendingUploadsMu.Lock()
	defer w.pendingUploadsMu.Unlock()
	if len(w.pendingUploads) == 0 {
		return nil
	}
	out := w.pendingUploads
	w.pendingUploads = nil
	return out
}

func whatsAppSessionPath() string {
	return filepath.Join(familyDataDir(), "whatsapp", "session.db")
}

func initWhatsAppBot(ctx context.Context) error {
	conn := whatsappbot.New(whatsappbot.Config{
		DBPath:         whatsAppSessionPath(),
		AppName:        "SparkQuill",
		AppVersion:     [3]uint32{1, 0, 0},
		Debug:          os.Getenv("WHATSAPP_DEBUG") == "true",
		LogPrefix:      "[whatsapp]",
		MultiAccount:   true, // every parent links their own phone
		PairingTimeout: whatsappPairingTimeout,
		OutboundText:   stripHTMLForWhatsApp,
		Handler:        whatsAppBot,
		Access:         whatsappbot.SelfChatOnly{}, // only the parent's own "message yourself" chat
		Router:         waModeRouter{},
		Routes:         waModeStore{},
	})
	whatsAppBot.mu.Lock()
	whatsAppBot.conn = conn
	whatsAppBot.mu.Unlock()
	return conn.Start(ctx)
}

// ---- lifecycle passthroughs used by main.go, notify.go and the file tool ---

func (w *waBot) EnsureConnecting(ctx context.Context) {
	if c := w.connector(); c != nil {
		c.EnsureConnecting(ctx)
	}
}

func (w *waBot) IsPaired() bool {
	c := w.connector()
	return c != nil && c.IsPaired()
}

func (w *waBot) IsConnected() bool {
	c := w.connector()
	return c != nil && c.IsConnected()
}

func (w *waBot) GetQR() (code string, expires time.Time) {
	if c := w.connector(); c != nil {
		return c.GetQR()
	}
	return "", time.Time{}
}

func (w *waBot) GetQRImagePNG(size int) ([]byte, error) {
	if c := w.connector(); c != nil {
		return c.GetQRImagePNG(size)
	}
	return nil, nil
}

func (w *waBot) Unpair(ctx context.Context, phone string) error {
	c := w.connector()
	if c == nil {
		return fmt.Errorf("whatsapp is not initialized")
	}
	return c.Unpair(ctx, phone)
}

func (w *waBot) SendToAllSelf(ctx context.Context, text string) (sent int, lastErr error) {
	c := w.connector()
	if c == nil {
		return 0, fmt.Errorf("whatsapp is not initialized")
	}
	return c.SendToAllSelf(ctx, text)
}

func (w *waBot) SendDocumentToAllSelf(ctx context.Context, data []byte, filename, mimetype, caption string) (sent int, lastErr error) {
	c := w.connector()
	if c == nil {
		return 0, fmt.Errorf("whatsapp is not initialized")
	}
	return c.SendDocumentToAllSelf(ctx, data, filename, mimetype, caption)
}

func sendWhatsAppFileTool(onSent func(path string)) agentsession.Tool {
	return agentsession.Tool{
		Name: "send_whatsapp_file",
		Description: "Send a test, study material file, academic map, or progress report to the parent as a real PDF " +
			"attachment on their own WhatsApp (their linked \"message yourself\" chat) — only call this when the parent " +
			"explicitly asks for a file/PDF over WhatsApp, or (Pulse only) as part of an automated check-in that has " +
			"genuinely new progress to show. The file must already exist as a PDF (use agent_browser: open the file, then " +
			"run its \"pdf\" command to export a PDF into the same folder — e.g. <Subject>/<Topic>/<activity>/<name>.pdf for " +
			"an activity, or reports/<name>.pdf for the academic map or progress report — before calling this). Requires " +
			"WhatsApp to be linked (Connectors → WhatsApp) — if it's not, tell the parent to link it there first.",
		Category: "family_tools",
		Params: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":    map[string]interface{}{"type": "string", "description": "workspace-relative path to the PDF, e.g. Math/Fractions/2026-07-23-quick-check/quick-check.pdf"},
				"caption": map[string]interface{}{"type": "string", "description": "optional short caption to send with the file"},
			},
			"required": []string{"path"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			if !whatsAppBot.IsConnected() {
				return "", fmt.Errorf("whatsapp is not linked")
			}
			rel := strings.TrimSpace(fmt.Sprint(args["path"]))
			caption, _ := args["caption"].(string)
			if !strings.HasSuffix(strings.ToLower(rel), ".pdf") {
				return "", fmt.Errorf("path must be a .pdf file")
			}
			if findActivityForPath(rel) == "" && !strings.HasPrefix(strings.Trim(rel, "/"), "reports/") {
				return "", fmt.Errorf("send_whatsapp_file only sends files inside an activity folder or reports/")
			}
			abs, ok := resolveWorkspacePath(rel)
			if !ok {
				return "", fmt.Errorf("invalid path")
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", rel, err)
			}
			sendCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			sent, sendErr := whatsAppBot.SendDocumentToAllSelf(sendCtx, data, filepath.Base(rel), "application/pdf", caption)
			if sent == 0 {
				if sendErr != nil {
					return "", fmt.Errorf("send whatsapp document: %w", sendErr)
				}
				return "", fmt.Errorf("send whatsapp document: no connected accounts")
			}
			if onSent != nil {
				onSent(rel)
			}
			return `{"status":"sent"}`, nil
		},
	}
}

// ---- routes: "@child" / "@parent" -----------------------------------------

// waModeRouter is SparkQuill's route table: two fixed routes, recognized
// anywhere in the text ("@child she got stuck on Q5"), not only as a prefix.
type waModeRouter struct{}

func (waModeRouter) Resolve(_ context.Context, _ string, token string) *whatsappbot.Route {
	switch token {
	case waRoutingModeChild, waRoutingModeParent:
		return &whatsappbot.Route{Key: token}
	}
	return nil
}

func (waModeRouter) MatchMention(text string) (token, rest string, ok bool) {
	rest, mode := extractModeSwitch(text)
	return mode, rest, mode != ""
}

// waModeStore persists the active route as the routing mode file
// (whatsapp_routing.go): the mode is a sticky switch shared by every linked
// phone, and "parent" is the default rather than "no route".
type waModeStore struct{}

func (waModeStore) Active(string) string   { return loadWaRoutingMode() }
func (waModeStore) Activate(_, key string) { setWaRoutingMode(key) }
func (waModeStore) Deactivate(string)      { setWaRoutingMode(waRoutingModeParent) }

func extForMime(mime, def string) string { return whatsapptransport.ExtensionForMime(mime, def) }

func sanitizeInboxName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.NewReplacer("/", "_", "\\", "_", "..", "_", " ", "_").Replace(name)
	if name == "" || name == "." {
		name = "wa-attachment"
	}
	return name
}

// childMentionRe / parentMentionRe match "@child"/"@parent" as whole words,
// case-insensitive, so "@childhood" or "email@childcare.com" don't
// false-positive. These are MODE SWITCHES (see whatsapp_routing.go), not
// per-message tags: sending "@child" once puts every later image message
// into the child's current activity until "@parent" switches back.
var childMentionRe = regexp.MustCompile(`(?i)@child\b`)
var parentMentionRe = regexp.MustCompile(`(?i)@parent\b`)

// extractModeSwitch checks for an "@child"/"@parent" mode-switch keyword
// anywhere in the text and returns the caption with it stripped out — e.g.
// "@child she got stuck on Q5" -> ("she got stuck on Q5", "child"). "" for
// mode means no switch keyword was present.
func extractModeSwitch(text string) (rest string, mode string) {
	switch {
	case childMentionRe.MatchString(text):
		return strings.Join(strings.Fields(childMentionRe.ReplaceAllString(text, " ")), " "), waRoutingModeChild
	case parentMentionRe.MatchString(text):
		return strings.Join(strings.Fields(parentMentionRe.ReplaceAllString(text, " ")), " "), waRoutingModeParent
	default:
		return text, ""
	}
}

func extractWhatsAppMessageText(m *waProto.Message) string { return whatsapptransport.ExtractText(m) }

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTMLForWhatsApp removes HTML tags (keeping their text content) from
// everything that goes out over WhatsApp: the system prompts encourage
// `<span style="color:...">` for the desktop's rendered bubbles, and WhatsApp
// is plain text with nowhere for that markup to go.
func stripHTMLForWhatsApp(text string) string {
	return htmlTagRe.ReplaceAllString(text, "")
}

// ---- route acknowledgements -----------------------------------------------

func (w *waBot) RouteActivated(_ context.Context, msg *whatsappbot.Message, route *whatsappbot.Route, _ bool) {
	w.confirmMode(msg, route.Key)
}

// RouteDeactivated handles "@child off": there is no "no route" in this
// product, so it is just the parent route again.
func (w *waBot) RouteDeactivated(_ context.Context, msg *whatsappbot.Message, _ string) {
	w.confirmMode(msg, waRoutingModeParent)
}

func (w *waBot) RouteUnknown(context.Context, *whatsappbot.Message, string) {}

func (w *waBot) confirmMode(msg *whatsappbot.Message, mode string) {
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	label := parentChildLabel(s.Child)
	var confirmation string
	if mode == waRoutingModeChild {
		title := ""
		if dir := currentActivityDir(); dir != "" {
			if act, ok := loadActivity(dir); ok && act.Title != "" {
				title = act.Title
			}
		}
		if title != "" {
			confirmation = fmt.Sprintf("Switched to child mode — photos now go to %q. Send @parent to switch back.", title)
		} else {
			confirmation = "Switched to child mode — photos will go to " + label + "'s activity once one is open. Send @parent to switch back."
		}
	} else {
		confirmation = "Back to our conversation — photos come to me again."
	}
	msg.React("🔀")
	w.send(msg, confirmation, 2, 15*time.Second, "mode switch")
}

// send is a logged, retrying reply.
func (w *waBot) send(msg *whatsappbot.Message, text string, attempts int, attemptTimeout time.Duration, logPrefix string) error {
	c := w.connector()
	if c == nil {
		return fmt.Errorf("whatsapp not paired")
	}
	if err := c.SendTextWithRetry(msg.Chat, text, attempts, attemptTimeout); err != nil {
		log.Printf("[whatsapp] %s: send failed after %d attempts: %v", logPrefix, attempts, err)
		return err
	}
	return nil
}

// ---- attachments ----------------------------------------------------------

// ingestWhatsAppMedia saves the message's attachment under the workspace
// (destDir, or the inbox) and, for a voice note with transcription on,
// returns its local transcript instead of keeping the audio file.
func ingestWhatsAppMedia(msg *whatsappbot.Message, destDir string) (saved bool, voiceText string, savedPath string) {
	if !msg.HasMedia {
		return false, "", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	media, err := msg.Download(ctx, 0)
	if err != nil {
		log.Printf("[whatsapp] media download failed: %v", err)
		return false, "", ""
	}
	data := media.Data
	isVoice := msg.IsVoiceNote()
	var name string
	switch {
	case media.Kind == "document" && media.FileName != "":
		name = media.FileName
	case isVoice:
		name = "wa-voice-" + msg.ID + media.Ext
	case media.Kind == "document":
		name = "wa-" + msg.ID + extForMime(media.MimeType, ".bin")
	default:
		name = "wa-" + msg.ID + media.Ext
	}
	name = sanitizeInboxName(name)
	relDir := strings.TrimSpace(destDir)
	if relDir == "" {
		relDir = "inbox"
	}
	absDir := filepath.Join(familyDataDir(), "workspace", relDir)
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		log.Printf("[whatsapp] inbox mkdir failed: %v", err)
		return false, "", ""
	}
	absPath := filepath.Join(absDir, name)
	if err := os.WriteFile(absPath, data, 0o600); err != nil {
		log.Printf("[whatsapp] media save failed: %v", err)
		return false, "", ""
	}
	relPath := filepath.ToSlash(filepath.Join(relDir, name))
	log.Printf("[whatsapp] saved attachment to %s (%d bytes)", relPath, len(data))
	savedPath = relPath
	if isVoice {
		stateMu.Lock()
		voiceOn := whatsAppVoiceEnabled(loadState())
		stateMu.Unlock()
		if !voiceOn {
			log.Printf("[whatsapp] voice transcription disabled by parent — saved audio only")
		} else {
			transcript, err := transcribeAudioFile(ctx, absPath)
			if err != nil {
				log.Printf("[whatsapp] voice transcription unavailable: %v", err)
			} else if strings.TrimSpace(transcript) != "" {
				voiceText = strings.TrimSpace(transcript)
				log.Printf("[whatsapp] transcribed voice note (%d chars)", len(voiceText))
				if err := os.Remove(absPath); err != nil {
					log.Printf("[whatsapp] could not remove transcribed voice note %s: %v", relPath, err)
				} else {
					log.Printf("[whatsapp] removed transcribed voice note %s (transcript kept in the conversation)", relPath)
					savedPath = ""
				}
			}
		}
	}
	return true, voiceText, savedPath
}

// ---- the message handler --------------------------------------------------

// HandleMessage runs once per accepted self-chat message. The connector has
// already applied the mode switch (msg.Route is "child" or "parent") and
// stripped the "@child"/"@parent" keyword from msg.Text.
func (w *waBot) HandleMessage(_ context.Context, msg *whatsappbot.Message) {
	text := msg.Text
	inChildMode := msg.Route != nil && msg.Route.Key == waRoutingModeChild
	proto := msg.Proto()

	// Where an attachment in THIS message lands: the child's current
	// activity in child mode, otherwise the shared parent inbox.
	destDir := ""
	var childActivityDir string
	if inChildMode {
		childActivityDir = currentActivityDir()
		if childActivityDir != "" {
			destDir = childActivityDir
		}
	}

	// An image or document is saved and stops there — only text ever reaches
	// the agent. A bare attachment is acknowledged (👀) and remembered in
	// pendingUploads for the next real turn. A voice note is the exception:
	// its transcript stands in for typed text below and always stays on the
	// parent path, even in child mode.
	gotMedia, voiceText, savedPath := ingestWhatsAppMedia(msg, destDir)

	// A photo or video gets a visible card in the desktop transcript the
	// instant it is saved, in whichever conversation the file landed in.
	if gotMedia && savedPath != "" && proto != nil && (proto.ImageMessage != nil || proto.VideoMessage != nil) {
		mediaTool := "photo"
		if proto.VideoMessage != nil {
			mediaTool = "video"
		}
		if inChildMode && childActivityDir != "" {
			appendToolMessageToConversation("child", childActivityDir, enginedetect.ChatMessage{Role: "tool", Tool: mediaTool, Path: savedPath})
		} else {
			appendToolMessageToConversation("parent", parentConversationID, enginedetect.ChatMessage{Role: "tool", Tool: mediaTool, Path: savedPath})
		}
	}

	// A voice note that could not be transcribed still shows up on screen,
	// with the raw audio playable, so the silence is explained.
	if gotMedia && savedPath != "" && msg.IsVoiceNote() && voiceText == "" {
		if inChildMode && childActivityDir != "" {
			appendToolMessageToConversation("child", childActivityDir, enginedetect.ChatMessage{Role: "tool", Tool: "voice_failed", Path: savedPath})
		} else {
			appendToolMessageToConversation("parent", parentConversationID, enginedetect.ChatMessage{Role: "tool", Tool: "voice_failed", Path: savedPath})
		}
	}

	if inChildMode && gotMedia && voiceText == "" {
		stateMu.Lock()
		s := loadState()
		stateMu.Unlock()
		label := parentChildLabel(s.Child)
		if childActivityDir == "" {
			// The attachment already fell back to the shared inbox — not
			// lost, just not where it needs to be.
			msg.React("⚠️")
			w.send(msg, "You're in child mode, but there's no activity open right now, so I couldn't send this to her — open one on her side, or @parent to switch back.", 2, 15*time.Second, "child mode no-activity")
			return
		}
		if savedPath == "" {
			return // download failed — ingestWhatsAppMedia already logged why
		}
		saveCurrentUploadWithNote(savedPath, strings.TrimSpace(text))
		title := childActivityDir
		if act, ok := loadActivity(childActivityDir); ok && act.Title != "" {
			title = act.Title
		}
		msg.React("✅")
		w.send(msg, fmt.Sprintf("Added to %q — I'll look at it as soon as %s is back in that activity.", title, label), 2, 15*time.Second, "child photo confirmation")
		return
	}

	// Typed text in child mode runs a real turn in the CHILD's conversation
	// (a follow-up like "review these answers" belongs next to the photo
	// that was just added there, not in the parent conversation).
	if inChildMode && strings.TrimSpace(text) != "" && voiceText == "" {
		stateMu.Lock()
		s := loadState()
		stateMu.Unlock()
		if s.Engine == "" || s.Child == nil {
			msg.React("⚠️")
			w.send(msg, "Setup isn't complete yet, so I can't run this in child mode.", 2, 15*time.Second, "child mode setup incomplete")
			return
		}
		dir := currentActivityDir()
		if dir == "" {
			msg.React("⚠️")
			w.send(msg, "There's no activity open right now, so I can't send this to her — open one on her side, or @parent to switch back.", 2, 15*time.Second, "child mode no-activity text")
			return
		}
		if trySteer(context.Background(), dir, text) {
			appendUserMessageToConversation("child", dir, text)
			msg.React("↩️")
			return
		}
		msg.React("👀")
		stopPending := msg.Pending("⏳", 12*time.Second)
		existing, _ := loadStoredConversation("child", dir)
		history := append(append([]enginedetect.ChatMessage(nil), existing.Messages...), enginedetect.ChatMessage{Role: "user", Text: text})
		resp := runChildTurn(context.Background(), s, dir, history)
		stopPending()
		if resp.Error != "" {
			msg.React("⚠️")
			log.Printf("[whatsapp] child-mode turn failed: %s", resp.Error)
			return
		}
		msg.ClearReaction() // the reply is the completion signal
		if err := w.send(msg, resp.Reply, 3, 30*time.Second, "child mode reply"); err != nil {
			msg.React("⚠️")
		}
		return
	}

	if strings.TrimSpace(text) == "" && voiceText != "" {
		text = "🎙️ " + voiceText
		// Confirm what was heard right away: a turn can take minutes and
		// speech-to-text is not perfect, so the parent can correct it.
		w.send(msg, fmt.Sprintf("🎙️ I heard: “%s”", voiceText), 2, 15*time.Second, "voice transcript confirmation")
	}

	if strings.TrimSpace(text) == "" {
		if gotMedia {
			if savedPath != "" {
				w.noteUpload(savedPath)
			}
			msg.React("👀")
			log.Printf("[whatsapp] media with no text — filed, no turn started")
		} else {
			log.Printf("[whatsapp] message %s had no text we could read — ignored", msg.ID)
		}
		return
	}

	// If a parent turn is already running (web app, another phone), inject
	// this message into it live instead of queuing a separate turn.
	if trySteer(context.Background(), parentConversationID, text) {
		appendUserMessageToConversation("parent", parentConversationID, text)
		msg.React("↩️") // delivered live — no separate reply coming for THIS message
		return
	}

	// 👀 the instant we accept it, ⏳ if the turn runs long, cleared when the
	// reply is sent, ⚠️ if the turn fails.
	msg.React("👀")
	if notice := whatsappBusyNotice(agentTurnBusy()); notice != "" {
		w.send(msg, notice, 1, 10*time.Second, "busy notice")
	}
	stopPending := msg.Pending("⏳", 12*time.Second)
	reply, err := w.runTurn(text)
	stopPending()
	if err != nil {
		msg.React("⚠️")
		log.Printf("[whatsapp] turn failed: %v", err)
		return
	}
	msg.ClearReaction()
	if err := w.send(msg, reply, 3, 30*time.Second, "reply"); err != nil {
		msg.React("⚠️")
	}
}

// busyNoticeGap keeps a burst of messages from earning a notice each.
const busyNoticeGap = 5 * time.Minute

var lastBusyNotice struct {
	mu sync.Mutex
	at time.Time
}

// whatsappBusyNotice returns what to tell the sender about an in-progress turn,
// or "" to stay quiet. Takes agentTurnBusy's results directly so the caller
// reads as one statement.
func whatsappBusyNotice(kind string, running time.Duration, busy bool) string {
	if !busy {
		return ""
	}
	if running < 5*time.Second {
		return ""
	}
	lastBusyNotice.mu.Lock()
	defer lastBusyNotice.mu.Unlock()
	if time.Since(lastBusyNotice.at) < busyNoticeGap {
		return ""
	}
	lastBusyNotice.at = time.Now()

	switch kind {
	case "pulse":
		return "Quick heads-up — I'm in the middle of a check-in right now, so this one will take a few minutes. I'll reply as soon as it finishes."
	case "child":
		return "I'm helping with an activity right now — I'll get to this straight after."
	default:
		return "I'm still finishing something else — I'll reply to this right after."
	}
}

// runTurn runs one real agent turn for a message received over any linked
// WhatsApp account — the same agentic runtime as the in-app WhatsApp
// simulator (handleWhatsAppMessage). Every account funnels into the SAME
// shared "parent" conversation, so it never needs to know which account
// triggered it.
func (w *waBot) runTurn(text string) (string, error) {
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if s.Engine == "" {
		return "", fmt.Errorf("no learning engine selected")
	}
	if _, ok := engineToProvider(s.Engine); !ok {
		return "", fmt.Errorf("engine %q has no provider mapping", s.Engine)
	}

	convID := parentConversationID
	existing, _ := loadStoredConversation("parent", convID)
	messages := append([]enginedetect.ChatMessage(nil), existing.Messages...)
	messages = append(messages, enginedetect.ChatMessage{Role: "user", Text: text})

	// Drained once, here, so this turn is told PLAINLY that files are
	// waiting instead of leaving it to notice on its own.
	uploadNote := ""
	if pending := w.drainPendingUploads(); len(pending) > 0 {
		uploadNote = fmt.Sprintf(" Also, separately: %d file(s) landed in your inbox with no caption before this message (%s) — file them with the process-file skill at some point in this turn, but that is NOT necessarily what this message is about; answer what's actually being asked first.",
			len(pending), strings.Join(pending, ", "))
	}
	// Per-turn WhatsApp formatting hint sent to the model but NOT persisted.
	hint := "\n\n(Replying over WhatsApp on the phone — keep it short and plain text: no markdown, headings, or file paths. IMPORTANT: there is no screen/panel here — calling open_file does NOT show the parent anything, it's a silent no-op on this channel. NEVER say \"I've opened it\" or \"it's ready and open\" here — that's only true on the web app. Instead, either describe what's in the file directly in your reply, or if the parent wants the actual file, use send_whatsapp_file to send it as a real PDF attachment (export to PDF via agent_browser first if it isn't one already). If the message above starts with 🎙️, that prefix means it's a LOCAL, ON-DEVICE TRANSCRIPT of a voice note the parent just sent — the text after it is genuinely what they said, already fully readable by you. Respond directly to its content exactly as you would a typed message; do NOT say you can't listen to or process voice/audio messages — you just did." + uploadNote + ")"

	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	defer cancel()
	resp := runParentTurn(ctx, s, convID, messages, hint)
	if resp.Error != "" {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Reply, nil
}

// --- HTTP routes ---------------------------------------------------------

type whatsAppAccountStatus struct {
	JID       string `json:"jid"`
	Connected bool   `json:"connected"`
}

type whatsAppPairingStatus struct {
	QRAvailable bool   `json:"qr_available"`
	QRExpiresAt string `json:"qr_expires_at,omitempty"`
}

type whatsAppStatusResponse struct {
	Accounts           []whatsAppAccountStatus  `json:"accounts"`
	Pairing            whatsAppPairingStatus    `json:"pairing"`
	VoiceTranscription voiceTranscriptionStatus `json:"voice_transcription"`
}

func handleWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	whatsAppBot.EnsureConnecting(r.Context())
	resp := whatsAppStatusResponse{Accounts: []whatsAppAccountStatus{}}
	if c := whatsAppBot.connector(); c != nil {
		for _, acct := range c.Accounts() { // ordered by phone number
			resp.Accounts = append(resp.Accounts, whatsAppAccountStatus{JID: acct.OwnJID().User, Connected: acct.IsConnected()})
		}
	}
	if code, expires := whatsAppBot.GetQR(); code != "" {
		resp.Pairing.QRAvailable = true
		resp.Pairing.QRExpiresAt = expires.UTC().Format(time.RFC3339)
	}
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	resp.VoiceTranscription = currentVoiceTranscriptionStatus(s)
	writeJSON(w, http.StatusOK, resp)
}

func handleWhatsAppPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	whatsAppBot.EnsureConnecting(r.Context())
	png, err := whatsAppBot.GetQRImagePNG(320)
	if err != nil {
		http.Error(w, "failed to render QR: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if png == nil {
		http.Error(w, "no pairing QR available yet — try again in a moment", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func handleWhatsAppUnpair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JID string `json:"jid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.JID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jid is required"})
		return
	}
	if err := whatsAppBot.Unpair(r.Context(), strings.TrimSpace(req.JID)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
