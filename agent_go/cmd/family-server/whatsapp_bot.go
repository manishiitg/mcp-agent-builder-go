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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/enginedetect"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/whatsapptransport"
	"github.com/skip2/go-qrcode"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// whatsappPairingTimeout bounds how long one pairing attempt (QR generation +
// waiting for the phone to scan it) stays alive before giving up. A fresh
// attempt (and a fresh QR) is started the next time the frontend asks.
const whatsappPairingTimeout = 30 * time.Second

// waAccount is ONE linked WhatsApp account — one parent's own phone, linked
// via its own QR scan — on top of the shared transport
// (pkg/whatsapptransport). Everything WhatsApp-protocol lives there; what
// stays here is this product's policy: self-chat only, plain-text replies,
// attachments into the inbox or the child's activity. Immutable after
// construction (unpairing removes the *waAccount from the manager's map
// rather than mutating it), so its methods need no locking of their own.
type waAccount struct {
	*whatsapptransport.Account
}

// ready reports whether this account can talk to WhatsApp at all.
func (a *waAccount) ready() bool { return a != nil && a.Account != nil }

// isSelfChat is the safety boundary described on waBot: true only for THIS
// account's own "Message Yourself" chat (phone-number JID or LID).
func (a *waAccount) isSelfChat(chat types.JID) bool {
	return a.ready() && a.IsSelfChat(chat)
}

// react adds (or, with emoji "", clears) an emoji reaction on an incoming
// message — the "got it / working on it" acknowledgement, since an agent turn
// can take a minute or two and there'd otherwise be no sign the message was
// received. Best-effort.
func (a *waAccount) react(chat, sender types.JID, msgID types.MessageID, emoji string) {
	if !a.ready() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.React(ctx, chat, sender, msgID, emoji); err != nil {
		log.Printf("[whatsapp] reaction failed: %v", err)
	}
}

// sendTextWithRetry sends a plain-text reply (HTML stripped: this channel is
// a phone), retrying transient failures. logPrefix labels the log lines.
func (a *waAccount) sendTextWithRetry(chat types.JID, text string, attempts int, attemptTimeout time.Duration, logPrefix string) error {
	if !a.ready() {
		return fmt.Errorf("whatsapp not paired")
	}
	if err := a.SendTextWithRetry(chat, stripHTMLForWhatsApp(text), attempts, attemptTimeout); err != nil {
		log.Printf("[whatsapp] %s: send failed after %d attempts: %v", logPrefix, attempts, err)
		return err
	}
	return nil
}

// SendToSelf pushes a message into this account's own "Message Yourself"
// chat proactively — used by notify_user/Pulse.
func (a *waAccount) SendToSelf(ctx context.Context, text string) error {
	if !a.ready() {
		return fmt.Errorf("whatsapp not paired")
	}
	self, err := a.SelfChat()
	if err != nil {
		return err
	}
	return a.SendText(ctx, self, stripHTMLForWhatsApp(text))
}

// SendDocumentToSelf sends a file as a document attachment to this account's
// own chat — how a test or study material reaches the parent as a real PDF.
func (a *waAccount) SendDocumentToSelf(ctx context.Context, data []byte, filename, mimetype, caption string) error {
	if !a.ready() {
		return fmt.Errorf("whatsapp not paired")
	}
	self, err := a.SelfChat()
	if err != nil {
		return err
	}
	return a.SendDocument(ctx, self, data, filename, mimetype, stripHTMLForWhatsApp(caption))
}

// ingestWhatsAppMedia downloads an image/document/voice-note attachment from
// an incoming WhatsApp message into the inbox and reports whether one was
// saved. For images/documents this deliberately does NOT tell the model about
// it or the path — only text messages go to the agent; the file just lands in
// the inbox and the process-file skill's own "check inbox/ before every
// reply" habit picks it up naturally on whatever the next real turn is, the
// same as any other inbox arrival. A voice note is different: it IS meant to
// drive a turn, so on top of saving the raw audio, it's transcribed locally
// (see transcribeAudioFile) and the transcript is returned so the caller can
// treat it as if the parent had typed it. Best-effort throughout: a failed
// download or transcription just degrades gracefully (any accompanying text
// still gets handled normally; a voice note that fails to transcribe still
// gets saved and silently acknowledged, same as an image/document).
// savedPath is the workspace-relative path of what's left after this call
// (inside destDir, or "" when nothing remains there — download failed, or a
// voice note was transcribed and its audio removed) — the caller uses it to
// tell the NEXT real turn what's waiting, see waBot.noteUpload.
//
// destDir is workspace-relative; "" defaults to "inbox" (the normal parent
// path). A caller that recognized an "@child" mention (see
// extractChildMention) passes the child's current activity dir instead, so
// the attachment lands where the child's own turn will actually look for it
// (via pendingChildUploadSuffix), not in the shared inbox no child turn ever
// reads from.
func (a *waAccount) ingestWhatsAppMedia(evt *events.Message, destDir string) (saved bool, voiceText string, savedPath string) {
	m := evt.Message
	if m == nil || !whatsapptransport.HasMedia(m) || !a.ready() {
		return false, "", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	media, err := a.Download(ctx, m, 0)
	if err != nil {
		log.Printf("[whatsapp] media download failed: %v", err)
		return false, "", ""
	}
	data := media.Data
	isVoice := whatsapptransport.IsVoiceNote(m)
	// Inbox naming policy: a document keeps the sender's file name; anything
	// else is named after the message id so two photos never collide.
	var name string
	switch {
	case media.Kind == "document" && media.FileName != "":
		name = media.FileName
	case isVoice:
		name = "wa-voice-" + evt.Info.ID + media.Ext
	case media.Kind == "document":
		name = "wa-" + evt.Info.ID + whatsapptransport.ExtensionForMime(media.MimeType, ".bin")
	default:
		name = "wa-" + evt.Info.ID + media.Ext
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
				// The transcript IS the message from here on — it becomes the
				// parent's chat turn and is persisted in conversation.json
				// permanently. The raw audio has no reader after this point:
				// unlike a photo, a voice note is never filed into materials/
				// by process-file, so it would sit in inbox/ forever, and the
				// "check inbox/ before every reply" habit would make the agent
				// re-notice the same dead files on every single turn.
				// Only deleted on SUCCESS — a failed or disabled transcription
				// keeps the audio, since then nothing else records what the
				// parent said and removing it would lose the message outright.
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

// waBot manages every linked WhatsApp account (one per parent) via whatsmeow
// (the unofficial WhatsApp Web multi-device protocol — the same mechanism as
// scanning a QR to link a device in the real WhatsApp app). Each parent links
// their OWN phone; all of them share the single "parent" conversation Quill
// already uses for web chat + Pulse (one unified family memory, not
// per-parent silos — the SAME shared conversation, just reachable from
// multiple linked phones).
//
// Safety boundary: each account only ever acts in ITS OWN paired "Message
// Yourself" chat (see waAccount.isSelfChat) — never a real contact or group.
// Without that restriction, Quill would start replying to whoever else a
// linked phone talks to, which would be a serious safety problem for a
// family app. This mirrors AgentWorks' whatsapp_service.go pattern, stripped
// of its multi-tenant/routing-table machinery this app doesn't need.
type waBot struct {
	mu       sync.RWMutex
	store    *whatsapptransport.Store
	accounts map[string]*waAccount // keyed by phone number (JID.User) once paired
	// bgCtx is a long-lived context for the connection/pairing goroutines —
	// deliberately NOT derived from any HTTP request's context. A request's
	// context is canceled the instant that response is written, which would
	// silently kill an in-flight whatsmeow Connect()/GetQRChannel() call if it
	// were used here instead (this was a real bug caught in testing: the
	// pairing goroutine died silently on every request with no log at all).
	bgCtx context.Context

	pairingMu sync.Mutex
	pending   *waAccount // the in-progress (not-yet-paired) pairing slot; nil when none
	qrMu      sync.RWMutex
	lastQR    string
	qrExpires time.Time

	// seenMsgs dedupes WhatsApp's own redelivery of the same event — a real,
	// documented whatsmeow/multi-device behavior (reconnect, retry, multi-device
	// resync can all redeliver an already-handled message). See alreadyHandled.
	seenMu sync.Mutex
	seen   *whatsapptransport.Dedupe

	// pendingUploads names bare (no-caption) attachments saved to inbox/ since
	// the last real turn — batched, not fired per-upload: a parent can send
	// several photos in a row and then a voice note, and firing a separate turn
	// per attachment would be wasteful and could race with each other. Drained
	// (read and cleared) by the very next real turn, which gets told plainly
	// that these arrived — see the per-turn note built from this in
	// handleIncomingMessage. Fixes a real bug: a bare photo used to sit in
	// inbox/ relying ONLY on the system prompt's "check inbox before every
	// reply" habit to notice it, and that habit ran INSIDE whatever the next
	// message happened to be about — confirmed live, "generate these questions"
	// (referring to something three messages earlier) got silently answered
	// from the new photo instead, because noticing it took over the whole
	// reply. An explicit, structural note next to the actual request removes
	// the ambiguity that prompt wording alone left room for.
	pendingUploadsMu sync.Mutex
	pendingUploads   []string
}

// alreadyHandled reports whether this WhatsApp message ID was already
// processed, recording it if this is the first time. Without this, a
// redelivered event started a brand-new turn each time — confirmed live: the
// same voice note ("process this again like the images...") triggered two
// separate turns 25 seconds apart, each with its own "🎙️ I heard: ..."
// confirmation and its own reply. Entries older than the window are pruned
// opportunistically on each call rather than via a separate goroutine, since
// traffic here is low-volume.
func (w *waBot) alreadyHandled(msgID string) bool {
	w.seenMu.Lock()
	if w.seen == nil {
		w.seen = whatsapptransport.NewDedupe(10 * time.Minute)
	}
	d := w.seen
	w.seenMu.Unlock()
	return d.Seen(msgID)
}

func (w *waBot) noteUpload(relPath string) {
	w.pendingUploadsMu.Lock()
	w.pendingUploads = append(w.pendingUploads, relPath)
	w.pendingUploadsMu.Unlock()
}

// drainPendingUploads returns and clears whatever bare attachments have
// accumulated since the last real turn — read once, by whichever turn comes
// next, so the same batch is never mentioned twice.
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

var whatsAppBot = &waBot{}

func whatsAppSessionPath() string {
	return filepath.Join(familyDataDir(), "whatsapp", "session.db")
}

// initWhatsAppBot opens (or creates) the local device store and reconnects
// every already-paired account found in it. It does NOT block on any one
// account's connect — each reconnects in its own goroutine so a slow/offline
// phone never delays server startup or the others. Called once at server
// startup so status reflects real state immediately.
func initWhatsAppBot(ctx context.Context) error {
	st, err := whatsapptransport.OpenStore(ctx, whatsAppSessionPath(), "SparkQuill", [3]uint32{1, 0, 0}, os.Getenv("WHATSAPP_DEBUG") == "true")
	if err != nil {
		return err
	}
	devices, err := st.Devices(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: load devices: %w", err)
	}
	whatsAppBot.mu.Lock()
	whatsAppBot.store = st
	whatsAppBot.accounts = map[string]*waAccount{}
	whatsAppBot.bgCtx = context.Background()
	whatsAppBot.mu.Unlock()
	for _, device := range devices {
		acct := whatsAppBot.buildAccount(device)
		if id := acct.OwnJID(); !id.IsEmpty() {
			whatsAppBot.mu.Lock()
			whatsAppBot.accounts[id.User] = acct
			whatsAppBot.mu.Unlock()
		}
		go func(a *waAccount) {
			if err := a.Connect(); err != nil {
				log.Printf("[whatsapp] connect failed for %s: %v", a.OwnJID().User, err)
			} else {
				log.Printf("[whatsapp] connected as %s", a.OwnJID().String())
			}
		}(acct)
	}
	return nil
}

func (w *waBot) buildAccount(device *store.Device) *waAccount {
	acct := &waAccount{}
	acct.Account = w.store.NewAccount(device, func(rawEvt interface{}) { w.handleEvent(acct, rawEvt) })
	return acct
}

func (w *waBot) EnsureConnecting(_ context.Context) {
	w.mu.RLock()
	accounts := make([]*waAccount, 0, len(w.accounts))
	for _, a := range w.accounts {
		accounts = append(accounts, a)
	}
	bgCtx := w.bgCtx
	w.mu.RUnlock()

	for _, a := range accounts {
		if a.ready() && !a.IsConnected() {
			go func(a *waAccount) { _ = a.Connect() }(a)
		}
	}

	if !w.pairingMu.TryLock() {
		return // a pairing attempt is already in flight
	}
	go func() {
		defer w.pairingMu.Unlock()
		w.startPairingAttempt(bgCtx)
	}()
}

// startPairingAttempt runs one QR-pairing attempt for a BRAND NEW phone —
// never touches already-linked accounts (each gets its own fresh
// container.NewDevice()). On success the newly-paired account is added to
// the accounts map under its own phone number; on timeout/failure nothing
// changes and the next EnsureConnecting call starts a fresh attempt.
func (w *waBot) startPairingAttempt(ctx context.Context) {
	w.mu.RLock()
	st := w.store
	w.mu.RUnlock()
	if st == nil {
		return
	}
	acct := w.buildAccount(st.NewDevice())
	w.mu.Lock()
	w.pending = acct
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.pending = nil
		w.mu.Unlock()
	}()
	log.Printf("[whatsapp] starting a new pairing attempt")
	paired, err := acct.Pair(ctx, whatsappPairingTimeout, func(u whatsapptransport.QRUpdate) {
		w.qrMu.Lock()
		w.lastQR, w.qrExpires = u.Code, u.Expires
		w.qrMu.Unlock()
		if u.Code != "" {
			log.Printf("[whatsapp] QR ready (expires %s)", u.Expires.Format(time.Kitchen))
		}
	})
	if err != nil {
		log.Printf("[whatsapp] pairing attempt failed: %v", err)
		return
	}
	if !paired {
		return
	}
	own := acct.OwnJID()
	log.Printf("[whatsapp] paired successfully as %s", own.String())
	if !own.IsEmpty() {
		w.mu.Lock()
		w.accounts[own.User] = acct
		w.mu.Unlock()
	}
}

func (w *waBot) IsPaired() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.accounts) > 0
}

// IsConnected reports whether at least one linked phone currently has a live
// connection.
func (w *waBot) IsConnected() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, a := range w.accounts {
		if a.IsConnected() {
			return true
		}
	}
	return false
}

func (w *waBot) GetQR() (code string, expires time.Time) {
	w.qrMu.RLock()
	defer w.qrMu.RUnlock()
	if w.lastQR != "" && !w.qrExpires.IsZero() && time.Now().After(w.qrExpires) {
		return "", time.Time{}
	}
	return w.lastQR, w.qrExpires
}

func (w *waBot) GetQRImagePNG(size int) ([]byte, error) {
	code, _ := w.GetQR()
	if code == "" {
		return nil, nil
	}
	if size <= 0 {
		size = 320
	}
	return qrcode.Encode(code, qrcode.Medium, size)
}

// Unpair removes ONE linked account by its phone number (JID.User) — logs it
// out, disconnects, and deletes just its own device row, leaving every OTHER
// linked account untouched. (The old single-account design deleted the whole
// session DB file on unpair, which would have wiped out every other parent
// too — that's exactly the regression this per-account deletion avoids.)
func (w *waBot) Unpair(ctx context.Context, phone string) error {
	w.mu.Lock()
	acct, ok := w.accounts[phone]
	st := w.store
	if ok {
		delete(w.accounts, phone)
	}
	w.mu.Unlock()
	if !ok || !acct.ready() {
		return fmt.Errorf("no linked account for %q", phone)
	}
	_ = acct.Logout(ctx)
	acct.Disconnect()
	if st != nil && acct.Device() != nil {
		if err := st.DeleteDevice(ctx, acct.Device()); err != nil {
			return fmt.Errorf("whatsapp: delete device: %w", err)
		}
	}
	return nil
}

func (w *waBot) SendToAllSelf(ctx context.Context, text string) (sent int, lastErr error) {
	for _, a := range w.connectedAccounts() {
		if err := a.SendToSelf(ctx, text); err != nil {
			lastErr = err
			continue
		}
		sent++
	}
	return sent, lastErr
}

// SendDocumentToAllSelf is SendToAllSelf for a document attachment — used by
// send_whatsapp_file. Sends to every currently-connected linked account, not
// just whichever one (if any) originated the turn — matching notify_user's
// existing "goes to every channel you've set up" philosophy rather than
// threading "which account asked" through the whole tool-call chain.
func (w *waBot) SendDocumentToAllSelf(ctx context.Context, data []byte, filename, mimetype, caption string) (sent int, lastErr error) {
	for _, a := range w.connectedAccounts() {
		if err := a.SendDocumentToSelf(ctx, data, filename, mimetype, caption); err != nil {
			lastErr = err
			continue
		}
		sent++
	}
	return sent, lastErr
}

func (w *waBot) connectedAccounts() []*waAccount {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]*waAccount, 0, len(w.accounts))
	for _, a := range w.accounts {
		if a.IsConnected() {
			out = append(out, a)
		}
	}
	return out
}

// sendWhatsAppFileTool lets the parent-mode agent hand over a test or study
// guide — or the academic map / progress report — as a real PDF attachment
// on WhatsApp, instead of only describing it in text — e.g. "send me the
// fractions test as a PDF on WhatsApp", or Pulse attaching the progress
// report to its own automated check-in. Scoped to files inside an activity
// folder OR reports/ only (not an answer key elsewhere, not any arbitrary
// file) — those stay off WhatsApp — and only sends to linked accounts' own
// self-chats (SendDocumentToAllSelf) — never a third party.
// onSent, when non-nil, is called with the workspace-relative path of each
// file successfully sent — so the caller (a web-chat or WhatsApp turn) can
// append a real, clickable reference to the persisted reply afterward. The
// model's own reply text alone doesn't reliably do this: the system prompt
// tells it to keep file paths out of prose, so without this the file was
// sent but genuinely invisible anywhere in the chat transcript/UI.
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
			// reports/ (the academic map, the progress report) is parent-facing
			// summary content, same as any activity's own files — deliberately
			// distinct from an answer key or anything else that stays off
			// WhatsApp, which live elsewhere and are NOT covered by this rule.
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

// --- incoming messages -------------------------------------------------

func (w *waBot) handleEvent(acct *waAccount, rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		w.handleIncomingMessage(acct, evt)
	default:
		if desc := whatsapptransport.DescribeEvent(rawEvt); desc != "" {
			log.Printf("[whatsapp] %s: %s", acct.OwnJID().User, desc)
		}
	}
}

// extForMime maps a media mimetype to a file extension, falling back to def.
func extForMime(mime, def string) string { return whatsapptransport.ExtensionForMime(mime, def) }

// sanitizeInboxName strips path separators and dodgy characters from an
// attachment filename so it can't escape the inbox.
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
// mode means no switch keyword was present. Whitespace left behind by
// removing the keyword is collapsed so the remaining text reads naturally
// either way (the keyword was at the start, the end, or mid-sentence).
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

// stripHTMLForWhatsApp removes HTML tags (keeping their text content) from a
// reply before it goes out over WhatsApp. Both childSystemPrompt and
// parentSystemPrompt explicitly encourage `<span style="color:...">` for the
// desktop app's rendered chat bubbles — WhatsApp is plain text with nowhere
// for that markup to go, so it showed up as literal `<span ...>` in the
// message instead of being reformatted. Confirmed live in Pulse's own
// WhatsApp summaries. Only tags are removed; the text inside them stays.
func stripHTMLForWhatsApp(text string) string {
	return htmlTagRe.ReplaceAllString(text, "")
}

func (w *waBot) handleIncomingMessage(acct *waAccount, evt *events.Message) {
	info := evt.Info
	if info.IsFromMe && !acct.isSelfChat(info.Chat) {
		return // outgoing message to a real contact/group — never act on these
	}
	if info.IsGroup || info.Chat.Server == types.BroadcastServer {
		return
	}
	if !acct.isSelfChat(info.Chat) {
		return // a real contact DMed this linked account — never reply as Quill
	}
	if w.alreadyHandled(info.ID) {
		log.Printf("[whatsapp] skipping redelivered message %s", info.ID)
		return
	}
	text := extractWhatsAppMessageText(evt.Message)

	// "@child"/"@parent" flip a PERSISTENT routing mode (see
	// whatsapp_routing.go) rather than tagging just this one message — once
	// switched, every later image attachment goes to that target until
	// switched back, so snapping several photos in a row doesn't need the
	// keyword retyped each time.
	if rest, switchTo := extractModeSwitch(text); switchTo != "" {
		setWaRoutingMode(switchTo)
		stateMu.Lock()
		s := loadState()
		stateMu.Unlock()
		label := parentChildLabel(s.Child)
		var confirmation string
		if switchTo == waRoutingModeChild {
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
		acct.react(info.Chat, info.Sender, info.ID, "🔀")
		if acct.ready() {
			_ = acct.sendTextWithRetry(info.Chat, confirmation, 2, 15*time.Second, "whatsapp mode switch")
		}
		text = rest // whatever text (if any) is left after stripping the keyword, e.g. "@child she got Q5 wrong"
	}

	// Where an attachment in THIS message lands: the child's current
	// activity if we're currently in child mode, otherwise the shared parent
	// inbox — unchanged default behavior.
	destDir := ""
	var childActivityDir string
	inChildMode := loadWaRoutingMode() == waRoutingModeChild
	if inChildMode {
		childActivityDir = currentActivityDir()
		if childActivityDir != "" {
			destDir = childActivityDir
		}
	}

	// An image or document attachment: save it straight into the inbox (or,
	// in child mode, the child's current activity folder) and stop there —
	// only text messages ever reach the agent. A bare attachment with no
	// caption just gets acknowledged (👀) below and never starts a turn; the
	// file waits in the inbox, NAMED in w.pendingUploads, until the next real
	// turn — which gets told about it explicitly (see below), not left to
	// notice it on its own. A voice note is the exception: its local
	// transcript (see ingestWhatsAppMedia) stands in for typed text below, so
	// it drives a turn just like normal — and is NEVER child-routed, even in
	// child mode, since it's genuinely meant to converse, not attach a photo.
	gotMedia, voiceText, savedPath := acct.ingestWhatsAppMedia(evt, destDir)

	// A photo or video (never a document or voice note) gets a visible card in
	// the desktop chat transcript the instant it's saved — independent of
	// whatever else happens with any caption text below — so "did a photo/
	// video arrive" is visible on screen, not just inferred from Quill's
	// reply. Lands in whichever conversation the file itself landed in: the
	// child's activity when routed there, otherwise the shared parent
	// conversation (including the child-mode-but-no-activity-open fallback,
	// where the file already fell back to the parent inbox).
	if gotMedia && savedPath != "" && (evt.Message.ImageMessage != nil || evt.Message.VideoMessage != nil) {
		mediaTool := "photo"
		if evt.Message.VideoMessage != nil {
			mediaTool = "video"
		}
		if inChildMode && childActivityDir != "" {
			appendToolMessageToConversation("child", childActivityDir, enginedetect.ChatMessage{Role: "tool", Tool: mediaTool, Path: savedPath})
		} else {
			appendToolMessageToConversation("parent", parentConversationID, enginedetect.ChatMessage{Role: "tool", Tool: mediaTool, Path: savedPath})
		}
	}

	// A voice note whose transcription failed (STT worker down/timed out) or
	// was disabled otherwise vanishes from the desktop's point of view —
	// WhatsApp still gets some ack below, but nothing on screen explained
	// why nothing happened. This card surfaces that failure in the same
	// transcript a successfully-transcribed voice note lands in (see the
	// "🎙️ I heard: ..." path further down), with the raw audio still
	// playable so it isn't lost even though it couldn't be read aloud as text.
	if gotMedia && savedPath != "" && evt.Message.AudioMessage != nil && voiceText == "" {
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
			// destDir was left "" above, so the attachment already fell back
			// to the shared inbox — not lost, just not where it needs to be.
			acct.react(info.Chat, info.Sender, info.ID, "⚠️")
			if acct.ready() {
				_ = acct.sendTextWithRetry(info.Chat, "You're in child mode, but there's no activity open right now, so I couldn't send this to her — open one on her side, or @parent to switch back.", 2, 15*time.Second, "child mode no-activity")
			}
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
		acct.react(info.Chat, info.Sender, info.ID, "✅")
		if acct.ready() {
			_ = acct.sendTextWithRetry(info.Chat, fmt.Sprintf("Added to %q — I'll look at it as soon as %s is back in that activity.", title, label), 2, 15*time.Second, "child photo confirmation")
		}
		return
	}

	// A genuinely typed message (not a voice transcript — those always stay
	// on the parent path, see above) sent while in child mode goes to the
	// CHILD's own conversation and runs a real turn there via runChildTurn —
	// otherwise a follow-up like "can you review these answers" would reach
	// the parent conversation instead, which has no idea a photo was just
	// added to the child's activity (confirmed live: exactly this happened
	// before this branch existed).
	if inChildMode && strings.TrimSpace(text) != "" && voiceText == "" {
		stateMu.Lock()
		s := loadState()
		stateMu.Unlock()
		if s.Engine == "" || s.Child == nil {
			acct.react(info.Chat, info.Sender, info.ID, "⚠️")
			if acct.ready() {
				_ = acct.sendTextWithRetry(info.Chat, "Setup isn't complete yet, so I can't run this in child mode.", 2, 15*time.Second, "child mode setup incomplete")
			}
			return
		}
		dir := currentActivityDir()
		if dir == "" {
			acct.react(info.Chat, info.Sender, info.ID, "⚠️")
			if acct.ready() {
				_ = acct.sendTextWithRetry(info.Chat, "There's no activity open right now, so I can't send this to her — open one on her side, or @parent to switch back.", 2, 15*time.Second, "child mode no-activity text")
			}
			return
		}

		// Same live-steer-first pattern as the parent path above: if a child
		// turn is already running for this exact activity, inject this
		// message into it rather than only ever queuing behind agentTurnMu.
		if trySteer(context.Background(), dir, text) {
			appendUserMessageToConversation("child", dir, text)
			acct.react(info.Chat, info.Sender, info.ID, "↩️")
			return
		}

		acct.react(info.Chat, info.Sender, info.ID, "👀")
		longRunDone := make(chan struct{})
		go func() {
			select {
			case <-longRunDone:
			case <-time.After(12 * time.Second):
				acct.react(info.Chat, info.Sender, info.ID, "⏳")
			}
		}()

		existing, _ := loadStoredConversation("child", dir)
		history := append(append([]enginedetect.ChatMessage(nil), existing.Messages...), enginedetect.ChatMessage{Role: "user", Text: text})
		resp := runChildTurn(context.Background(), s, dir, history)
		close(longRunDone)
		if resp.Error != "" {
			acct.react(info.Chat, info.Sender, info.ID, "⚠️")
			log.Printf("[whatsapp] child-mode turn failed: %s", resp.Error)
			return
		}
		acct.react(info.Chat, info.Sender, info.ID, "") // clear the ack — the reply is the completion signal
		if acct.ready() {
			if err := acct.sendTextWithRetry(info.Chat, resp.Reply, 3, 30*time.Second, "child mode reply"); err != nil {
				acct.react(info.Chat, info.Sender, info.ID, "⚠️")
			}
		}
		return
	}

	if strings.TrimSpace(text) == "" && voiceText != "" {
		text = "🎙️ " + voiceText
		// Confirm what was actually heard right away, decoupled from the real
		// turn below — a turn can take minutes, and speech-to-text isn't
		// perfect, so the parent should see (and can correct/retype) what
		// Quill transcribed without waiting for the full reply.
		if acct.ready() {
			heard := fmt.Sprintf("🎙️ I heard: “%s”", voiceText)
			// Lower stakes than the real reply below (the actual answer still
			// arrives separately even if this is lost), so a couple of quick
			// attempts and no visible failure signal is enough here.
			_ = acct.sendTextWithRetry(info.Chat, heard, 2, 15*time.Second, "voice transcript confirmation")
		}
	}

	if strings.TrimSpace(text) == "" {
		if gotMedia {
			if savedPath != "" {
				w.noteUpload(savedPath)
			}
			acct.react(info.Chat, info.Sender, info.ID, "👀")
			log.Printf("[whatsapp] media with no text — filed, no turn started")
		} else {
			// Nothing usable at all. Worth a line: this is the path a message
			// type we do not understand yet falls down, and it is otherwise
			// completely silent — which is how dropped captions went unnoticed.
			log.Printf("[whatsapp] message %s had no text we could read — ignored", info.ID)
		}
		return
	}

	// If a turn is already running for the shared parent conversation (started
	// from the web app, or another linked phone), try to inject this message
	// into it LIVE instead of just queuing behind agentTurnMu — the same
	// steer mechanism the web app's composer uses (see steer.go). This is
	// genuinely different from queuing: without it, a second WhatsApp message
	// sent while Quill is still mid-reply just waits its turn and gets
	// processed as a completely separate turn afterward, even though the
	// parent very likely meant to redirect or add to what's already running.
	if trySteer(context.Background(), parentConversationID, text) {
		appendUserMessageToConversation("parent", parentConversationID, text)
		acct.react(info.Chat, info.Sender, info.ID, "↩️") // delivered live — no separate reply coming for THIS message
		return
	}

	// Eager acknowledgement: 👀 on the parent's message the instant we accept it,
	// so they see it was received while the (possibly 1-2 min) turn runs. If the
	// turn runs long, layer on ⏳ ("still working"). Cleared when the reply is
	// sent; swapped to ⚠️ if the turn fails. Mirrors AgentWorks' reaction ack.
	acct.react(info.Chat, info.Sender, info.ID, "👀")
	// A reaction is easy to miss when the wait is minutes rather than seconds.
	// If something is ALREADY running, say so in words up front: measured on
	// 2026-08-04, a parent's message sat 207s behind back-to-back Pulse check-ins
	// (another waited 14 minutes) before its turn could even start. The reply
	// always arrived; the silence beforehand is what read as "no response".
	if notice := whatsappBusyNotice(agentTurnBusy()); notice != "" {
		_ = acct.sendTextWithRetry(info.Chat, notice, 1, 10*time.Second, "busy notice")
	}
	longRunDone := make(chan struct{})
	go func() {
		select {
		case <-longRunDone:
		case <-time.After(12 * time.Second):
			acct.react(info.Chat, info.Sender, info.ID, "⏳")
		}
	}()

	reply, err := w.runTurn(text)
	close(longRunDone)
	if err != nil {
		acct.react(info.Chat, info.Sender, info.ID, "⚠️")
		log.Printf("[whatsapp] turn failed: %v", err)
		return
	}
	acct.react(info.Chat, info.Sender, info.ID, "") // clear the ack — the reply is the completion signal

	if !acct.ready() {
		return
	}
	// A fully-generated reply is the highest-stakes send here — retry before
	// giving up, and if every attempt still fails, react so the parent sees
	// SOMETHING is wrong rather than a reply that silently never arrives.
	if err := acct.sendTextWithRetry(info.Chat, reply, 3, 30*time.Second, "reply"); err != nil {
		acct.react(info.Chat, info.Sender, info.ID, "⚠️")
	}
}

// busyNoticeGap keeps a burst of messages from earning a notice each. One
// heads-up is information; three is noise on top of an already-slow reply.
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
	// Something that just started will likely finish before a notice would even
	// be useful; below this the reaction already covers it.
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
// simulator (handleWhatsAppMessage), just triggered by a whatsmeow event
// instead of an HTTP request from the frontend. Every account funnels into
// the SAME shared "parent" conversation, so it never needs to know which
// account triggered it.
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

	// WhatsApp joins the SINGLE parent conversation (same file + same warm tmux
	// session as the web chat and Pulse) so Quill has one unified memory across
	// every channel — a message on WhatsApp continues the same thread as the web.
	convID := parentConversationID

	// WhatsApp has no frontend to supply the thread, so build it from the
	// persisted file (the UI's source of truth) — the web chat gets its
	// messages from the request instead, but both end up calling
	// runParentTurn with the same shape: full history including the newest
	// message, exactly as it should be persisted.
	existing, _ := loadStoredConversation("parent", convID)
	messages := append([]enginedetect.ChatMessage(nil), existing.Messages...)
	messages = append(messages, enginedetect.ChatMessage{Role: "user", Text: text})

	// Drained once, right here, so whichever real turn comes next — this one —
	// is told PLAINLY that file(s) are waiting, instead of leaving it to notice
	// on its own via the system prompt's general "check inbox" habit (which
	// runs inside whatever this message is actually about, and can hijack an
	// unrelated request — confirmed live, see pendingUploads' own comment).
	// Explicit and separate from "the request" on purpose: filing them is
	// background housekeeping, not necessarily what this message is asking for.
	uploadNote := ""
	if pending := w.drainPendingUploads(); len(pending) > 0 {
		uploadNote = fmt.Sprintf(" Also, separately: %d file(s) landed in your inbox with no caption before this message (%s) — file them with the process-file skill at some point in this turn, but that is NOT necessarily what this message is about; answer what's actually being asked first.",
			len(pending), strings.Join(pending, ", "))
	}
	// Per-turn WhatsApp formatting hint sent to the model but NOT persisted (so
	// the stored/visible message stays clean). Because the tmux session is shared
	// with the web chat, the base system prompt may be the web one; this keeps
	// replies phone-appropriate regardless.
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

// GET /api/whatsapp/status
func handleWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Always ensure connections are live and a pairing attempt for a possible
	// new phone is in flight — EnsureConnecting is idempotent.
	whatsAppBot.EnsureConnecting(r.Context())

	whatsAppBot.mu.RLock()
	resp := whatsAppStatusResponse{Accounts: make([]whatsAppAccountStatus, 0, len(whatsAppBot.accounts))}
	for jid, acct := range whatsAppBot.accounts {
		resp.Accounts = append(resp.Accounts, whatsAppAccountStatus{
			JID:       jid,
			Connected: acct.ready() && acct.IsConnected(),
		})
	}
	whatsAppBot.mu.RUnlock()
	sort.Slice(resp.Accounts, func(i, j int) bool { return resp.Accounts[i].JID < resp.Accounts[j].JID })

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

// GET /api/whatsapp/pair — a PNG pairing QR code for the CURRENT pairing
// attempt (adding one more phone), or 404 if none is available yet — the
// frontend polls this alongside /status. Always available regardless of how
// many phones are already linked (unlike the old single-account version,
// pairing another phone never requires unlinking an existing one first).
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

// POST /api/whatsapp/unpair — body {"jid": "<phone number>"}, unpairs just
// that one linked account.
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
