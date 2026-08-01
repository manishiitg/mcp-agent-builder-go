package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// currentUploadMu guards read-modify-write access to current-upload.json —
// needed because multiple uploads can legitimately land back-to-back before
// the next real child turn ever runs: several WhatsApp photos sent in a
// burst (each its own ingestWhatsAppMedia call), or a multi-file/multi-paste
// desktop upload (each file its own POST /api/upload, see handleUpload). A
// bare overwrite here — the previous design — meant only the LAST of any
// burst ever reached the child's next turn; the rest silently vanished from
// the agent's point of view even though the files themselves were saved to
// disk. Confirmed live: 8 WhatsApp photos sent in one burst, and the child's
// very next turn only ever got told about the 8th.
var currentUploadMu sync.Mutex

// currentUploadPath is the current-upload.json pointer file written by
// handleUpload (scope=child) — same pattern as current-activity.json for
// handoffs.
func currentUploadPath() (string, bool) { return resolveWorkspacePath("current-upload.json") }

type currentUploadEntry struct {
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

type currentUploadFile struct {
	Uploads []currentUploadEntry `json:"uploads,omitempty"`
}

func loadCurrentUploads() []currentUploadEntry {
	abs, ok := currentUploadPath()
	if !ok {
		return nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	var f currentUploadFile
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	return f.Uploads
}

func clearCurrentUploads() {
	if abs, ok := currentUploadPath(); ok {
		_ = os.Remove(abs)
	}
}

// appendCurrentUpload queues one just-uploaded child file — the SAME pattern
// as current-activity.json for handoffs. A prompt instruction telling the
// child agent to proactively list their activity folder for a new upload
// proved unreliable in testing (the model kept defaulting to checking the
// wrong folder instead); pointing it at specific, deterministic paths to read
// removes the guessing entirely. Appends rather than overwrites — see
// currentUploadMu's own comment for why that matters.
func appendCurrentUpload(rel, note string) {
	currentUploadMu.Lock()
	defer currentUploadMu.Unlock()
	abs, ok := currentUploadPath()
	if !ok {
		return
	}
	f := currentUploadFile{Uploads: loadCurrentUploads()}
	f.Uploads = append(f.Uploads, currentUploadEntry{Path: rel, Note: note})
	b, err := json.Marshal(f)
	if err != nil {
		return
	}
	_ = os.WriteFile(abs, b, 0o600)
}

// pendingChildUploadSuffix checks for any just-uploaded child photo(s) and,
// if any exist, returns text to APPEND onto this turn's own last message
// naming their EXACT path(s) — rather than relying on the model to
// proactively guess which folder to check (testing showed it unreliably
// defaults to the top-level inbox), or pre-computing the transcription for it
// to just trust (testing showed the model treats an unverifiable claim
// embedded in a "user" message with the same skepticism it would show the
// child fabricating an answer, and insists on checking the real file itself
// anyway — reasonable caution, but it fights a pre-computed answer). Stating
// the real path(s) lets the model's OWN read_image call succeed on the first
// try — verification stays genuinely the model's, only the "which folder"
// guess is removed. Always clears the queue so the same photo is never
// re-flagged as new on a later turn.
func pendingChildUploadSuffix() string {
	currentUploadMu.Lock()
	uploads := loadCurrentUploads()
	if len(uploads) > 0 {
		clearCurrentUploads()
	}
	currentUploadMu.Unlock()
	if len(uploads) == 0 {
		return ""
	}
	if len(uploads) == 1 {
		suffix := "\n\n(I uploaded it to " + uploads[0].Path + ")"
		if uploads[0].Note != "" {
			// A parent's own note about the photo (e.g. a WhatsApp caption sent
			// alongside it) — passed through as context, not as a claim to trust
			// blindly; the model still looks at the file itself via read_image.
			suffix += " (note from the parent: " + uploads[0].Note + ")"
		}
		return suffix
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n\n(I uploaded %d files while you weren't looking — please check each one:", len(uploads))
	for _, u := range uploads {
		sb.WriteString("\n- " + u.Path)
		if u.Note != "" {
			sb.WriteString(" (note from the parent: " + u.Note + ")")
		}
	}
	sb.WriteString(")")
	return sb.String()
}
