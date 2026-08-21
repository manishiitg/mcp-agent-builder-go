// Package workspacepathpolicy owns the filesystem lifecycle of paths granted to
// a workspace sandbox. A Folder Guard is an authorization policy; it cannot
// create a path that a Linux sandbox needs to compile into its ruleset.
package workspacepathpolicy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Lifecycle int

const (
	Required Lifecycle = iota
	PlatformManaged
	Optional
)

type Kind int

const (
	Any Kind = iota
	Directory
	File
)

type Grant struct {
	Path      string
	Lifecycle Lifecycle
	Kind      Kind
	Mode      fs.FileMode
}

// Materialize validates grants below root and creates platform-managed
// directories before the sandbox policy is registered. Optional missing paths
// are returned as inactive; callers must omit those from the actual guard.
func Materialize(root string, grants []Grant) (active []Grant, err error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("resolve workspace policy root: path is empty")
	}
	root, err = filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("resolve workspace policy root: %w", err)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = fmt.Errorf("not a directory")
		}
		return nil, fmt.Errorf("workspace policy root %q is unavailable: %w", root, statErr)
	}
	resolvedRoot, evalErr := filepath.EvalSymlinks(root)
	if evalErr != nil {
		return nil, fmt.Errorf("resolve workspace policy root %q: %w", root, evalErr)
	}
	root = resolvedRoot

	for _, grant := range grants {
		target, resolveErr := resolveBelowRoot(root, grant.Path)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if grant.Lifecycle == PlatformManaged {
			if grant.Kind != Directory {
				return nil, fmt.Errorf("platform-managed path %q must be a directory", grant.Path)
			}
			mode := grant.Mode
			if mode == 0 {
				mode = 0o700
			}
			if err := ensureNoSymlinkEscape(root, target); err != nil {
				return nil, err
			}
			if err := os.MkdirAll(target, mode); err != nil {
				return nil, fmt.Errorf("create platform-managed workspace directory %q: %w", grant.Path, err)
			}
		}

		info, statErr := os.Stat(target)
		if statErr != nil {
			if os.IsNotExist(statErr) && grant.Lifecycle == Optional {
				continue
			}
			return nil, fmt.Errorf("workspace path %q is unavailable: %w", grant.Path, statErr)
		}
		if err := validateKind(grant, info); err != nil {
			return nil, err
		}
		resolved, evalErr := filepath.EvalSymlinks(target)
		if evalErr != nil {
			return nil, fmt.Errorf("resolve workspace path %q: %w", grant.Path, evalErr)
		}
		if !isBelow(root, resolved) {
			return nil, fmt.Errorf("workspace path %q escapes policy root through a symlink", grant.Path)
		}
		active = append(active, grant)
	}
	return active, nil
}

func resolveBelowRoot(root, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("workspace policy path is empty")
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)
	if !isBelow(root, target) {
		return "", fmt.Errorf("workspace path %q escapes policy root", path)
	}
	return target, nil
}

func isBelow(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func ensureNoSymlinkEscape(root, target string) error {
	current := target
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return fmt.Errorf("resolve workspace path %q: %w", target, evalErr)
			}
			if !isBelow(root, resolved) {
				return fmt.Errorf("workspace path %q escapes policy root through a symlink", target)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect workspace path %q: %w", target, err)
		}
		parent := filepath.Dir(current)
		if parent == current || !isBelow(root, parent) {
			return fmt.Errorf("workspace path %q has no safe ancestor", target)
		}
		current = parent
	}
}

func validateKind(grant Grant, info fs.FileInfo) error {
	switch grant.Kind {
	case Directory:
		if !info.IsDir() {
			return fmt.Errorf("workspace path %q must be a directory", grant.Path)
		}
	case File:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workspace path %q must be a regular file", grant.Path)
		}
	}
	return nil
}
