package videoproduct

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/productdeps"
)

func syncManagedProductSkills(ctx context.Context, workspacePath string) error {
	if err := productdeps.Ensure(ctx, workspacePath, mustVideoStudioManifest().Dependencies); err != nil {
		return err
	}
	return syncVisibleProductSkills(workspacePath)
}

// syncVisibleProductSkills gives project owners an inspectable copy of the
// product-owned guidance. These are not the runtime source of truth (the
// agent uses the embedded copies registered in profile_definition.go), and we
// deliberately keep them below skills/video-studio so they cannot shadow a
// user-installed skill with the same name.
func syncVisibleProductSkills(workspacePath string) error {
	root := filepath.Join(workspacePath, "skills", "video-studio")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create visible Video Studio skills folder: %w", err)
	}
	readme := "# Video Studio product skills\n\nThis folder is a read-only-for-reference copy of the guidance built into the Video Studio agent. It is refreshed when the project connects. Do not put project work here; use `work/`, `planning/`, or `outputs/` instead.\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		return fmt.Errorf("write visible Video Studio skills README: %w", err)
	}
	for _, definition := range profileSkills {
		sourceRoot := filepath.ToSlash(filepath.Dir(definition.path))
		targetRoot := filepath.Join(root, definition.name)
		if err := fs.WalkDir(profileSkillFiles, sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(filepath.FromSlash(sourceRoot), filepath.FromSlash(path))
			if err != nil {
				return err
			}
			target := filepath.Join(targetRoot, relative)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			content, err := profileSkillFiles.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.WriteFile(target, content, 0o644)
		}); err != nil {
			return fmt.Errorf("materialize visible Video Studio skill %s: %w", strings.TrimSpace(definition.name), err)
		}
	}
	return nil
}

// SyncVisibleSkillsForExistingProjects upgrades workspaces created before the
// visible product-skill library existed. It is intentionally limited to the
// Video Studio project layout and is safe to run at every agent startup.
func SyncVisibleSkillsForExistingProjects(docsRoot string) error {
	docsRoot = strings.TrimSpace(docsRoot)
	if docsRoot == "" {
		return nil
	}
	usersRoot := filepath.Join(docsRoot, "_users")
	users, err := os.ReadDir(usersRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list workspace users: %w", err)
	}
	for _, user := range users {
		if !user.IsDir() {
			continue
		}
		projectsRoot := filepath.Join(usersRoot, user.Name(), "Chats", "Video Studio", "projects")
		projects, err := os.ReadDir(projectsRoot)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("list Video Studio projects for %s: %w", user.Name(), err)
		}
		for _, project := range projects {
			if !project.IsDir() {
				continue
			}
			if err := syncVisibleProductSkills(filepath.Join(projectsRoot, project.Name())); err != nil {
				return fmt.Errorf("sync visible skills for Video Studio project %s: %w", project.Name(), err)
			}
		}
	}
	return nil
}
