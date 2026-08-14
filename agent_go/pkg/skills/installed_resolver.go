package skills

import (
	"fmt"
	"path"
	"strings"
)

// InstalledSkillFile mirrors mcpagent's read_skill fallback payload without
// importing it, so this package stays free of agent-runtime dependencies.
type InstalledSkillFile struct {
	Content        string
	Description    string
	AvailableFiles []string
}

// NewInstalledSkillReader returns a resolver for skills that are installed in a
// workspace but not attached to the agent.
//
// Progressive disclosure attaches a router skill and leaves the specialists on
// disk; reaching one previously meant the agent had to know a filesystem path
// and shell out, which loses read_skill's per-call limits and behaves
// differently on each provider. This grants no new access — the session's
// folder guard already exposes skills/ — it only lets the agent ask by name.
func NewInstalledSkillReader(workspaceAPIURL, workspacePath string) func(string, string) (InstalledSkillFile, error) {
	return func(skillName, relPath string) (InstalledSkillFile, error) {
		name := strings.TrimSpace(skillName)
		if name == "" {
			return InstalledSkillFile{}, fmt.Errorf("skill name is required")
		}
		// The caller normalises the path, but this is a host boundary: re-check
		// rather than trust that the only caller always will.
		clean := strings.TrimSpace(relPath)
		if clean == "" {
			clean = SkillFileName
		}
		if path.IsAbs(clean) || strings.Contains(clean, "..") || strings.ContainsRune(clean, '\x00') {
			return InstalledSkillFile{}, fmt.Errorf("skill path must be a safe relative path: %q", relPath)
		}

		if clean == SkillFileName {
			skill, err := GetSkillIn(workspaceAPIURL, workspacePath, name)
			if err != nil {
				return InstalledSkillFile{}, err
			}
			return InstalledSkillFile{
				Content:        skill.Content,
				Description:    skill.Frontmatter.Description,
				AvailableFiles: installedSkillFileNames(workspaceAPIURL, workspacePath, name),
			}, nil
		}

		client := NewWorkspaceAPIClient(workspaceAPIURL)
		for _, base := range skillSearchBases(workspacePath) {
			content, err := client.ReadFile(path.Join(base, name, clean))
			if err == nil {
				return InstalledSkillFile{
					Content:        content,
					AvailableFiles: installedSkillFileNames(workspaceAPIURL, workspacePath, name),
				}, nil
			}
		}
		return InstalledSkillFile{}, fmt.Errorf("file %q is not part of installed skill %q", clean, name)
	}
}

// skillSearchBases is the same workspace-then-user order GetSkillIn uses.
func skillSearchBases(workspacePath string) []string {
	bases := make([]string, 0, 2)
	if strings.TrimSpace(workspacePath) != "" {
		bases = append(bases, path.Join(workspacePath, SkillsBasePath))
	}
	return append(bases, SkillsBasePath)
}

// installedSkillFileNames lists what else the agent could read. Best-effort:
// an empty list is a weaker result, not a failed read.
func installedSkillFileNames(workspaceAPIURL, workspacePath, name string) []string {
	client := NewWorkspaceAPIClient(workspaceAPIURL)
	for _, base := range skillSearchBases(workspacePath) {
		entries, err := client.ListFiles(path.Join(base, name))
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.Type == "folder" {
				continue
			}
			names = append(names, path.Base(entry.Filepath))
		}
		if len(names) > 0 {
			return names
		}
	}
	return nil
}
