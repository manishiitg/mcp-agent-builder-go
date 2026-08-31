package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/workspace/models"
)

type externalDiffPatchRequest struct {
	Filepath    string                    `json:"filepath" binding:"required"`
	Diff        string                    `json:"diff" binding:"required"`
	FolderGuard *models.FolderGuardConfig `json:"folder_guard" binding:"required"`
}

// DiffPatchExternalFile is deliberately separate from /api/documents. It is
// token-protected by the router and accepts only a target covered by the
// internally attached FolderGuard. The public documents API remains confined
// to workspace-docs.
func DiffPatchExternalFile(c *gin.Context) {
	var req externalDiffPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Error: "invalid external diff request"})
		return
	}
	if req.FolderGuard == nil || !req.FolderGuard.Enabled {
		c.JSON(http.StatusForbidden, models.APIResponse[any]{Success: false, Error: "external diff requires an enabled folder guard"})
		return
	}
	target, err := authorizeExternalWriteTarget(req.Filepath, req.FolderGuard)
	if err != nil {
		c.JSON(http.StatusForbidden, models.APIResponse[any]{Success: false, Error: err.Error()})
		return
	}
	current, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		current = nil
	} else if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Error: err.Error()})
		return
	}
	patchCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	updated, err := applyDiffPatchFlexibleContext(patchCtx, string(current), req.Diff)
	if err == nil {
		err = verifyDiffApplied(string(current), req.Diff, updated)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[models.DiffPatchResponse]{
			Success: false,
			Error:   "Failed to apply diff patch: " + err.Error(),
			Data:    models.DiffPatchResponse{Applied: false},
		})
		return
	}
	if err := patchCtx.Err(); err != nil {
		c.JSON(http.StatusRequestTimeout, models.APIResponse[any]{Success: false, Error: err.Error()})
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{Success: false, Error: err.Error()})
		return
	}
	if err := os.WriteFile(target, []byte(updated), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{Success: false, Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse[models.DiffPatchResponse]{
		Success: true,
		Message: "External file diff-patched successfully",
		Data:    models.DiffPatchResponse{Applied: true},
	})
}

func authorizeExternalWriteTarget(raw string, guard *models.FolderGuardConfig) (string, error) {
	target := filepath.Clean(strings.TrimSpace(raw))
	if !filepath.IsAbs(target) || strings.ContainsRune(target, '\x00') {
		return "", os.ErrPermission
	}
	canonicalTarget, err := canonicalTargetForWrite(target)
	if err != nil {
		return "", os.ErrPermission
	}
	for _, blocked := range append(append([]string{}, guard.BlockedPaths...), guard.BlockedWritePaths...) {
		if canonicalRoot, ok := canonicalDirectoryRoot(blocked); ok && externalPathWithin(canonicalTarget, canonicalRoot) {
			return "", os.ErrPermission
		}
	}
	for _, allowed := range guard.WritePaths {
		canonicalRoot, ok := canonicalDirectoryRoot(allowed)
		if !ok || canonicalRoot == filepath.VolumeName(canonicalRoot)+string(filepath.Separator) {
			continue
		}
		if externalPathWithin(canonicalTarget, canonicalRoot) {
			return canonicalTarget, nil
		}
	}
	return "", os.ErrPermission
}

func externalPathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func canonicalDirectoryRoot(raw string) (string, bool) {
	path := filepath.Clean(strings.TrimSpace(raw))
	if !filepath.IsAbs(path) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	return filepath.Clean(resolved), err == nil && info.IsDir()
}

func canonicalTargetForWrite(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved), nil
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
