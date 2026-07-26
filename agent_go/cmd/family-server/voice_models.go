package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// whisperTier is one downloadable whisper.cpp model. All tiers share the SAME
// already-installed whisper-cli binary and the same transcribeAudioFile()
// path — only the model file differs — which is why these could be wired up
// with real confidence: nothing new has to work for them to work.
//
// Sizes are the real Content-Length from huggingface, checked rather than
// estimated, so the UI never promises "148MB" and then pulls 1.5GB.
type whisperTier struct {
	ID       string
	Filename string
	URL      string
	SizeMB   int
}

var whisperTiers = map[string]whisperTier{
	"builtin": {
		ID:       "builtin",
		Filename: "ggml-base.en.bin",
		URL:      "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin",
		SizeMB:   148,
	},
	"better": {
		ID:       "better",
		Filename: "ggml-small.en.bin",
		URL:      "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin",
		SizeMB:   465,
	},
	"best": {
		ID:       "best",
		Filename: "ggml-medium.en.bin",
		URL:      "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.en.bin",
		SizeMB:   1462,
	},
}

// whisperTierOrder is best-first, so "which model should transcribeAudioFile
// actually use" resolves to the strongest one the parent has installed
// without them having to also pick an "active" tier separately: installing a
// better model IS choosing it.
var whisperTierOrder = []string{"best", "better", "builtin"}

func whisperModelDir() string {
	if p := strings.TrimSpace(os.Getenv("WHISPER_MODEL_PATH")); p != "" {
		return filepath.Dir(p)
	}
	return filepath.Join(familyDataDir(), "whisper-models")
}

func whisperTierPath(t whisperTier) string {
	return filepath.Join(whisperModelDir(), t.Filename)
}

func whisperTierInstalled(id string) bool {
	t, ok := whisperTiers[id]
	if !ok {
		return false
	}
	fi, err := os.Stat(whisperTierPath(t))
	return err == nil && fi.Size() > 0
}

// bestInstalledWhisperModel returns the path of the strongest installed model,
// or "" when none is. transcribeAudioFile uses this so an upgrade takes effect
// immediately, with no separate "make it active" step to forget.
func bestInstalledWhisperModel() string {
	// An explicit env override still wins — it's how a developer pins a
	// specific model, and silently ignoring it would be surprising.
	if p := strings.TrimSpace(os.Getenv("WHISPER_MODEL_PATH")); p != "" {
		if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
			return p
		}
	}
	for _, id := range whisperTierOrder {
		if whisperTierInstalled(id) {
			return whisperTierPath(whisperTiers[id])
		}
	}
	return ""
}

// --- install progress -------------------------------------------------------

// modelInstallState is live progress for one tier's download, polled by the
// settings UI. A multi-hundred-MB download with no visible progress reads as
// a hang, so bytes/total are tracked as it streams rather than only at the end.
type modelInstallState struct {
	Installing bool   `json:"installing"`
	GotBytes   int64  `json:"got_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Error      string `json:"error,omitempty"`
}

var (
	modelInstallMu     sync.Mutex
	modelInstallStates = map[string]*modelInstallState{}
)

func installStateFor(id string) modelInstallState {
	modelInstallMu.Lock()
	defer modelInstallMu.Unlock()
	if s, ok := modelInstallStates[id]; ok {
		return *s
	}
	return modelInstallState{}
}

// installWhisperTier downloads one model in the background. Idempotent: a
// second call while the same tier is already downloading is a no-op rather
// than a second competing download of the same hundreds of megabytes.
func installWhisperTier(id string) {
	t, ok := whisperTiers[id]
	if !ok {
		return
	}
	modelInstallMu.Lock()
	if st, running := modelInstallStates[id]; running && st.Installing {
		modelInstallMu.Unlock()
		return
	}
	modelInstallStates[id] = &modelInstallState{Installing: true, TotalBytes: int64(t.SizeMB) * 1024 * 1024}
	modelInstallMu.Unlock()

	setErr := func(err error) {
		modelInstallMu.Lock()
		modelInstallStates[id] = &modelInstallState{Error: err.Error()}
		modelInstallMu.Unlock()
		log.Printf("[voice] installing %s failed: %v", t.Filename, err)
	}

	go func() {
		// whisper-cli itself is shared by every tier; make sure it (and ffmpeg)
		// exist before pulling a 1.5GB model that would be unusable without them.
		if err := ensureWhisperRuntime(); err != nil {
			setErr(err)
			return
		}
		if err := downloadWhisperTier(t, id); err != nil {
			setErr(err)
			return
		}
		modelInstallMu.Lock()
		delete(modelInstallStates, id)
		modelInstallMu.Unlock()
		log.Printf("[voice] installed %s", t.Filename)
	}()
}

// downloadWhisperTier streams to a temp file and renames into place, so an
// interrupted download never leaves a truncated model that would look
// installed and then fail at transcription time.
func downloadWhisperTier(t whisperTier, id string) error {
	dest := whisperTierPath(t)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	modelInstallMu.Lock()
	if st := modelInstallStates[id]; st != nil && resp.ContentLength > 0 {
		st.TotalBytes = resp.ContentLength
	}
	modelInstallMu.Unlock()

	tmpPath := dest + ".download"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	copyErr := copyWithProgress(f, resp.Body, id)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return closeErr
	}
	return os.Rename(tmpPath, dest)
}

// copyWithProgress streams src->dst, publishing bytes-so-far for the UI.
func copyWithProgress(dst io.Writer, src io.Reader, id string) error {
	buf := make([]byte, 256*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			total += int64(n)
			modelInstallMu.Lock()
			if st := modelInstallStates[id]; st != nil {
				st.GotBytes = total
			}
			modelInstallMu.Unlock()
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// removeWhisperTier deletes a model to reclaim its disk. Deliberately refuses
// to remove the LAST installed model: that would silently turn off speech
// input (and WhatsApp voice notes with it) rather than just downgrading it.
func removeWhisperTier(id string) error {
	t, ok := whisperTiers[id]
	if !ok {
		return fmt.Errorf("unknown model")
	}
	installed := 0
	for _, other := range whisperTierOrder {
		if whisperTierInstalled(other) {
			installed++
		}
	}
	if installed <= 1 && whisperTierInstalled(id) {
		return fmt.Errorf("this is the only speech model installed — install another first, or turn speech off in WhatsApp settings")
	}
	if err := os.Remove(whisperTierPath(t)); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Printf("[voice] removed %s", t.Filename)
	return nil
}

// POST /api/voice/model/install {"id":"better"} — starts a background
// download and returns immediately; the settings UI polls /api/voice/status
// for progress, which is also what makes a resumed page show a download that
// started before it loaded.
func handleVoiceModelInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.ID == "piper" {
		installPiper()
		writeJSON(w, http.StatusOK, map[string]string{"status": "installing"})
		return
	}
	if _, ok := whisperTiers[req.ID]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown model"})
		return
	}
	installWhisperTier(req.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "installing"})
}

// POST /api/voice/model/remove {"id":"better"} — deletes a model to reclaim
// its disk. Refuses to remove the last one (see removeWhisperTier).
func handleVoiceModelRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.ID == "piper" {
		if err := removePiper(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
		return
	}
	if err := removeWhisperTier(req.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// downloadFileWithProgress fetches url to dest, reporting progress scaled into
// the [startFrac..1.0] slice of an install that has other phases too (Piper's
// package tree finishes before its voice download starts).
func downloadFileWithProgress(url, dest, id string, startFrac float64) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	tmpPath := dest + ".download"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	buf := make([]byte, 256*1024)
	var got int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmpPath)
				return werr
			}
			got += int64(n)
			if resp.ContentLength > 0 {
				modelInstallMu.Lock()
				if st := modelInstallStates[id]; st != nil && st.TotalBytes > 0 {
					frac := startFrac + (1-startFrac)*(float64(got)/float64(resp.ContentLength))
					st.GotBytes = int64(frac * float64(st.TotalBytes))
				}
				modelInstallMu.Unlock()
			}
		}
		if rerr != nil {
			if rerr.Error() == "EOF" {
				break
			}
			f.Close()
			os.Remove(tmpPath)
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dest)
}
