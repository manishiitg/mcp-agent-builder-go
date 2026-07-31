package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type uploadResponse struct {
	Name    string `json:"name,omitempty"`
	Path    string `json:"path,omitempty"` // workspace-relative
	Size    int64  `json:"size,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Subject string `json:"subject,omitempty"`
	Topic   string `json:"topic,omitempty"`
	Error   string `json:"error,omitempty"`
}

// saveCurrentUpload queues the exact path of a child's just-uploaded photo —
// see appendCurrentUpload (child_upload.go) for the queueing rationale.
func saveCurrentUpload(rel string) { appendCurrentUpload(rel, "") }

// saveCurrentUploadWithNote is the same, plus an optional short note
// alongside the file — e.g. a parent's WhatsApp caption sent with the photo
// ("she got confused on Q5"), which pendingChildUploadSuffix folds into what
// it hands the next real turn.
func saveCurrentUploadWithNote(rel, note string) { appendCurrentUpload(rel, note) }

// safeName sanitizes an uploaded filename to its base, no traversal.
func safeName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "..", "-")
	name = strings.Trim(name, ". ")
	if name == "" {
		return "upload.bin"
	}
	return name
}

// POST /api/upload (multipart/form-data) — add school material to the workspace.
// Fields:
//
//	file    (required) the uploaded file
//	scope   parent|child (default parent)
//
// Files land at workspace/inbox/<filename> (parent) — the agent then reads,
// classifies, and files each one into materials/<subject>/<topic>/ with a
// metadata JSON, see skills/process-file/SKILL.md — or directly into the
// child's current activity folder (scope=child), since the child is
// sandboxed to exactly that one folder and it's immediately visible to their
// own agent turn without needing parent approval.
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil { // 64 MB
		writeJSON(w, http.StatusBadRequest, uploadResponse{Error: "invalid upload (expected multipart/form-data)"})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, uploadResponse{Error: "a file is required"})
		return
	}
	defer file.Close()

	relDir := "inbox"
	scope := "parent"
	if strings.TrimSpace(r.FormValue("scope")) == "child" {
		dir := currentActivityDir()
		if dir == "" {
			writeJSON(w, http.StatusBadRequest, uploadResponse{Error: "no activity is currently active"})
			return
		}
		relDir = dir
		scope = "child"
	}
	absDir := filepath.Join(familyDataDir(), "workspace", relDir)
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, uploadResponse{Error: err.Error()})
		return
	}

	name := safeName(hdr.Filename)
	absPath := filepath.Join(absDir, name)
	out, err := os.Create(absPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, uploadResponse{Error: err.Error()})
		return
	}
	size, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		writeJSON(w, http.StatusInternalServerError, uploadResponse{Error: "failed to save the file"})
		return
	}
	relPath := filepath.ToSlash(filepath.Join(relDir, name))
	if scope == "child" {
		saveCurrentUpload(relPath)
	}

	writeJSON(w, http.StatusOK, uploadResponse{
		Name:  name,
		Path:  relPath,
		Size:  size,
		Scope: scope,
	})
}
