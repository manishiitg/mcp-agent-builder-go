package step_based_workflow

import (
	"context"
	"path/filepath"
	"sort"
)

// snapshotCanonicalArtifactRef hashes the complete managed artifact tree, not
// just its index file. Learning updates commonly land under references/, so an
// SKILL.md-only audit would still miss material runtime-guidance mutations.
func (hcpo *StepBasedWorkflowOrchestrator) snapshotCanonicalArtifactRef(ctx context.Context, root string) string {
	entries := map[string]string{}
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if depth > 6 {
			return
		}
		names, err := hcpo.ListWorkspaceFiles(ctx, dir)
		if err != nil {
			return
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.ToSlash(filepath.Join(dir, name))
			if content, readErr := hcpo.ReadWorkspaceFile(ctx, path); readErr == nil {
				entries[path] = content
				continue
			}
			walk(path, depth+1)
		}
	}
	walk(root, 0)
	return artifactContentRef(entries)
}
