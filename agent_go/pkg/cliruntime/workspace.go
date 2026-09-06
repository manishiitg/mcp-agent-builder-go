// Package cliruntime separates mutable CLI instructions from workflow data.
package cliruntime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Prepare returns a stable private projection directory. StateRoot is owned by
// the application instance, not a client-supplied workspace. Sessions retain
// their directory across restarts; provider cleanup only touches that directory.
// This is instruction isolation, not an OS sandbox or workflow write lock.
func Prepare(stateRoot, workspaceRoot, user, workflow, session, provider, mode string) (string, error) {
	if !filepath.IsAbs(stateRoot) || !filepath.IsAbs(workspaceRoot) || !filepath.IsAbs(workflow) {
		return "", fmt.Errorf("CLI isolation requires absolute state, workspace and workflow paths")
	}
	for _, value := range []string{user, session, provider, mode} {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("CLI isolation requires user, session, provider and mode identity")
		}
	}
	// Resolve trusted roots before checking containment (macOS /var is a symlink).
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		return "", err
	}
	stateRoot, err := filepath.EvalSymlinks(stateRoot)
	if err != nil {
		return "", err
	}
	workspaceRoot, err = filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return "", err
	}
	workflow, err = filepath.EvalSymlinks(workflow)
	if err != nil {
		return "", err
	}
	if !within(workspaceRoot, workflow) {
		return "", fmt.Errorf("workflow is outside the workspace root")
	}
	base := filepath.Join(stateRoot, "cli-runtimes")
	if within(workspaceRoot, base) || within(base, workspaceRoot) {
		return "", fmt.Errorf("CLI runtime storage must be separate from workspace documents")
	}
	identity, _ := json.Marshal([]string{user, workflow, session, provider, mode})
	digest := fmt.Sprintf("%x", sha256.Sum256(identity))
	dir := base
	for _, component := range []string{"", "v1", digest} {
		dir = filepath.Join(dir, component)
		if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
			return "", err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("CLI runtime path is not a private directory: %s", dir)
		}
		if info.Mode().Perm() != 0700 {
			return "", fmt.Errorf("CLI runtime directory must have mode 0700: %s", dir)
		}
	}
	return dir, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// CanResume intentionally refuses a different chat's native session, even if
// it belongs to the same user. The application can replay saved history into a
// fresh native session without sharing mutable projection files.
func CanResume(current, saved string) bool {
	return current != "" && saved != "" && filepath.Clean(current) == filepath.Clean(saved)
}
