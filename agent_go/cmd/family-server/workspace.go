package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// treeNode is one entry in the workspace file tree.
type treeNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"` // relative to the workspace root
	Type     string     `json:"type"` // "dir" | "file"
	Children []treeNode `json:"children,omitempty"`
	// Size is bytes on disk: the file's own size, or for a directory the
	// recursive total of everything under it — INCLUDING entries hidden from
	// the tree itself (dotfiles like a .cursor/ or .git marker), since the
	// point of the number is the real footprint of that folder, not the sum
	// of what happens to be listed.
	Size int64 `json:"size"`
}

// workspaceRootHiddenNames are top-level entries that are app/agent
// machinery, not family content — the parent's Files tab shouldn't show
// them at all. skills/ is app-shipped reference content reseeded from the
// binary on every startup (see seedSkills), and AGENTS.md is mcpagent's own
// auto-managed per-session system-prompt file, restored/cleaned up by the
// coding-agent runtime itself. Scoped to top-level only (rel == "" in
// buildTree) since a same-named subfolder deeper in real family content
// (however unlikely) shouldn't be swept up by this.
var workspaceRootHiddenNames = map[string]bool{
	"skills":    true,
	"AGENTS.md": true,
}

// buildTreeSized returns absDir's visible child nodes AND the total bytes on
// disk beneath it. Hidden entries (dotfiles, and skills/ at the root) still
// count toward the size even though they're never listed — see treeNode.Size.
// Sizes are accumulated during this one walk rather than by a second pass per
// directory, which would re-walk every level once per ancestor.
func buildTreeSized(absDir, rel string) ([]treeNode, int64) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, 0
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di // directories first
		}
		return entries[i].Name() < entries[j].Name()
	})
	nodes := make([]treeNode, 0, len(entries))
	var total int64
	for _, e := range entries {
		name := e.Name()
		hidden := strings.HasPrefix(name, ".") || (rel == "" && workspaceRootHiddenNames[name])
		abs := filepath.Join(absDir, name)
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if e.IsDir() {
			// A hidden directory only needs its bytes counted, not its nodes
			// built — skip the node-building recursion entirely for it.
			if hidden {
				total += dirSizeBytes(abs)
				continue
			}
			kids, size := buildTreeSized(abs, childRel)
			total += size
			nodes = append(nodes, treeNode{
				Name:     name,
				Path:     childRel,
				Type:     "dir",
				Children: kids,
				Size:     size,
			})
			continue
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		total += size
		if hidden {
			continue
		}
		nodes = append(nodes, treeNode{Name: name, Path: childRel, Type: "file", Size: size})
	}
	return nodes, total
}

// dirSizeBytes sums every regular file beneath abs. Used for directories whose
// contents are never listed, so no tree nodes are built for them.
func dirSizeBytes(abs string) int64 {
	var total int64
	_ = filepath.WalkDir(abs, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort: an unreadable entry just doesn't count
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// workspaceTreeResponse carries the tree plus the workspace's TRUE total size.
// The total can't be derived by summing Nodes: entries hidden from the listing
// (skills/, AGENTS.md, and dotfiles like the coding-agent's own .git marker —
// measured at 8.6 MB of a 68 MB workspace) still occupy real disk, and the
// point of this number is to watch actual growth. Callers that only want the
// listing can keep reading Nodes and ignore the rest.
type workspaceTreeResponse struct {
	Nodes []treeNode `json:"nodes"`
	// TotalSize is every byte under the workspace root, listed or not.
	TotalSize int64 `json:"total_size"`
	// VisibleSize is the sum of Nodes only, so a caller can show the gap
	// between "what you can see here" and "what's actually on disk".
	VisibleSize int64 `json:"visible_size"`
}

// GET /api/workspace/tree — the live family workspace as a hierarchical tree.
func handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := filepath.Join(familyDataDir(), "workspace")
	_ = os.MkdirAll(root, 0o700)
	nodes, total := buildTreeSized(root, "")
	var visible int64
	for _, n := range nodes {
		visible += n.Size
	}
	writeJSON(w, http.StatusOK, workspaceTreeResponse{Nodes: nodes, TotalSize: total, VisibleSize: visible})
}

// seedWorkspace writes a couple of starter files so the tree is meaningful and
// the file-first model has real content. Later, agent tools write here too.
func seedWorkspace(child *Child) {
	base := filepath.Join(familyDataDir(), "workspace")
	if child != nil {
		if b, err := json.MarshalIndent(child, "", "  "); err == nil {
			_ = os.MkdirAll(filepath.Join(base, "memory"), 0o700)
			_ = os.WriteFile(filepath.Join(base, "memory", "child-profile.json"), b, 0o600)
		}
	}
	name := "your child"
	if child != nil && strings.TrimSpace(child.Name) != "" {
		name = child.Name
	}
	reportsDir := filepath.Join(base, "reports")
	_ = os.MkdirAll(reportsDir, 0o700)
	mapPath := filepath.Join(reportsDir, "academic-map.html")
	if _, err := os.Stat(mapPath); os.IsNotExist(err) {
		html := "<!doctype html>\n<meta charset=\"utf-8\">\n<title>Academic map</title>\n<h1>" + name + "’s academic map</h1>\n<p>This living view grows as " + name + " learns.</p>\n"
		_ = os.WriteFile(mapPath, []byte(html), 0o600)
	}
	progressPath := filepath.Join(reportsDir, "progress.html")
	if _, err := os.Stat(progressPath); os.IsNotExist(err) {
		html := "<!doctype html>\n<meta charset=\"utf-8\">\n<title>Progress</title>\n<h1>" + name + "’s progress</h1>\n<p>This living report grows as " + name + " learns.</p>\n"
		_ = os.WriteFile(progressPath, []byte(html), 0o600)
	}
}

// mirrorChildSchedule keeps memory/child-schedule.json (agent-readable) in
// sync with familyState.Schedule (the authoritative copy, saved via
// saveState) — same MarshalIndent/MkdirAll/WriteFile pattern seedWorkspace
// uses for memory/child-profile.json, just called explicitly at the specific
// spots the schedule actually changes (the set_child_schedule tool, the
// POST /api/child-schedule handler) plus once at startup, rather than
// threaded through seedWorkspace's existing Child-only signature.
func mirrorChildSchedule(sched ChildSchedule) {
	b, err := json.MarshalIndent(sched, "", "  ")
	if err != nil {
		return
	}
	base := filepath.Join(familyDataDir(), "workspace")
	_ = os.MkdirAll(filepath.Join(base, "memory"), 0o700)
	_ = os.WriteFile(filepath.Join(base, "memory", "child-schedule.json"), b, 0o600)
}
