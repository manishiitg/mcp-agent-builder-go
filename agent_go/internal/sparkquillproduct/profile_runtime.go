package sparkquillproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
)

// FamilyFile is the family's saved state, relative to the product workspace
// root (Chats/SparkQuill). It is the platform home of what the standalone
// app keeps in ~/.sunlit-learning/state.json.
const FamilyFile = "family.json"

// Child is the one child on this account.
type Child struct {
	Name     string `json:"name"`
	Grade    string `json:"grade"`
	Board    string `json:"board"`
	Language string `json:"language,omitempty"`
}

// ScheduleEntry is one recurring weekly commitment.
type ScheduleEntry struct {
	Day   string `json:"day"`
	Start string `json:"start"`
	End   string `json:"end"`
	Label string `json:"label"`
}

// FamilyState is family.json.
type FamilyState struct {
	Child       *Child          `json:"child,omitempty"`
	ParentLabel string          `json:"parent_label,omitempty"`
	PinHash     string          `json:"pin_hash,omitempty"`
	WatchSites  []string        `json:"watch_sites,omitempty"`
	Schedule    []ScheduleEntry `json:"schedule,omitempty"`
}

// ParentPromptVariables computes the parent prompt's Product variables:
// who the child is, and the nudges for whatever the family has not told
// Quill yet. Mirrors parentSystemPrompt in cmd/family-server.
func ParentPromptVariables(s FamilyState) map[string]string {
	name := "your child"
	who := name
	if s.Child != nil && strings.TrimSpace(s.Child.Name) != "" {
		name = strings.TrimSpace(s.Child.Name)
		who = name
	}
	var missing []string
	if s.Child == nil || strings.TrimSpace(s.Child.Name) == "" {
		missing = append(missing, "name")
	}
	if s.Child != nil && strings.TrimSpace(s.Child.Grade) != "" {
		who += ", Grade " + strings.TrimSpace(s.Child.Grade)
	} else {
		missing = append(missing, "grade")
	}
	if s.Child != nil && strings.TrimSpace(s.Child.Board) != "" {
		who += " (" + strings.TrimSpace(s.Child.Board) + ")"
	} else {
		missing = append(missing, "board (e.g. CBSE, ICSE, State Board)")
	}
	vars := map[string]string{
		"CHILD_NAME":         name,
		"CHILD_WHO":          who,
		"CHILD_INFO_NUDGE":   "",
		"PARENT_LABEL_NUDGE": "",
		"SCHEDULE_NUDGE":     "",
		"CONNECTOR_NOTE":     "",
		"INBOX_NOTE":         "",
	}
	if len(missing) > 0 {
		vars["CHILD_INFO_NUDGE"] = "IMPORTANT — you do not yet know the child's " + strings.Join(missing, ", ") +
			". Early in the conversation, warmly ask the parent for these, then save them with the set_child_profile tool. You need them to tailor material to the right grade and board.\n"
	}
	if strings.TrimSpace(s.ParentLabel) == "" {
		vars["PARENT_LABEL_NUDGE"] = "IMPORTANT — you don't yet know what to call the parent when you talk ABOUT them to " + name +
			" (\"your mom\" vs \"your dad\" vs a name). Early on, warmly ask once and save the answer with set_parent_label. Don't block other work on this.\n"
	}
	if len(s.Schedule) == 0 {
		vars["SCHEDULE_NUDGE"] = "You don't yet know " + name + "'s recurring weekly schedule (school hours, tuition, sports practice). If the conversation is about planning, study time, or when she's free, ask about her class schedule and save what you learn with set_child_schedule. Not urgent otherwise.\n"
	} else {
		vars["SCHEDULE_NUDGE"] = "You already have some of " + name + "'s recurring weekly schedule saved. If the parent mentions a NEW recurring commitment in conversation, capture it with set_child_schedule right then — an exact duplicate is silently skipped. Don't proactively ask for more.\n"
	}
	if sites := cleanSites(s.WatchSites); len(sites) > 0 {
		vars["CONNECTOR_NOTE"] = "The parent has asked you to keep an eye on these website(s): " + strings.Join(sites, ", ") +
			". Whenever they ask ANYTHING about them, call agent_browser(command=\"status\") FIRST, right then, before replying; never report \"no access\" without having just tried. Check memory/browser-notes.md before navigating and save what you learn there.\n"
	}
	return vars
}

// ChildPromptVariables computes the child prompt's Product variables for
// one activity. interests is the trimmed content of memory/interests.md,
// which the child's sandbox cannot read itself.
func ChildPromptVariables(s FamilyState, activityDir, interests string) map[string]string {
	name := "there"
	if s.Child != nil && strings.TrimSpace(s.Child.Name) != "" {
		name = strings.TrimSpace(s.Child.Name)
	}
	parent := strings.TrimSpace(s.ParentLabel)
	if parent == "" {
		parent = "parent"
	}
	vars := map[string]string{
		"CHILD_NAME":           name,
		"PARENT_LABEL":         parent,
		"GRADE_SUFFIX":         "",
		"GRADE_FOR_FORMATTING": "a school student",
		"ACTIVITY_DIR":         strings.Trim(strings.TrimSpace(activityDir), "/"),
		"INTERESTS_NOTE":       "",
	}
	if s.Child != nil && strings.TrimSpace(s.Child.Grade) != "" {
		grade := strings.TrimSpace(s.Child.Grade)
		vars["GRADE_SUFFIX"] = " (Grade " + grade + ")"
		vars["GRADE_FOR_FORMATTING"] = "in Grade " + grade
	}
	if note := strings.TrimSpace(interests); note != "" {
		if len(note) > 2000 {
			note = note[:2000]
		}
		vars["INTERESTS_NOTE"] = "WHAT SHE GENUINELY LIKES (from home, learned over time — never mention where this came from): " + note + "\n" +
			"Where it truly fits, nod to this in an example or an analogy — never force it into every turn.\n\n"
	}
	return vars
}

func cleanSites(sites []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range sites {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// RegisterAgentProfileRuntime wires both profiles' prompt variables to the
// family's saved state in the user's product workspace, registers the
// family tools, and prepares the parent workspace's presentation table.
func RegisterAgentProfileRuntime(registry *agentprofiles.Registry, workspaceAPIURL string) error {
	factories := map[string]agentprofiles.ToolFactory{
		"sparkquill.set-child-profile":        setChildProfileFactory(workspaceAPIURL),
		"sparkquill.set-child-schedule":       setChildScheduleFactory(workspaceAPIURL),
		"sparkquill.set-parent-label":         setParentLabelFactory(workspaceAPIURL),
		"sparkquill.create-learning-activity": createLearningActivityFactory(workspaceAPIURL),
		"sparkquill.open-file":                openFileFactory(workspaceAPIURL, false),
		"sparkquill.open-activity-file":       openFileFactory(workspaceAPIURL, true),
		"sparkquill.open-activity":            openActivityFactory(workspaceAPIURL),
		"sparkquill.suggest-actions":          suggestActionsFactory(),
		"sparkquill.celebrate":                celebrateFactory(),
		"sparkquill.show-scene":               showSceneFactory(),
		"sparkquill.find-image":               findImageFactory(workspaceAPIURL, false),
		"sparkquill.find-activity-image":      findImageFactory(workspaceAPIURL, true),
	}
	for id, factory := range factories {
		if err := registry.RegisterToolFactory(id, factory); err != nil {
			return err
		}
	}
	if err := registry.RegisterInitializer(ParentProfileID, func(ctx context.Context, rt agentprofiles.RuntimeContext) error {
		// The session id is what authorizes workspace execution (DB init is
		// one), the same way every tool call carries it.
		client := workspace.NewClient(workspaceAPIURL, workspace.WithUserID(rt.UserID),
			workspace.WithExtraEnv(map[string]string{"MCP_SESSION_ID": rt.SessionID}))
		_, err := client.InitializeWorkflowDB(ctx, workspace.InitializeWorkflowDBParams{
			DBPath:     path.Join(rt.WorkspacePath, "db/db.sqlite"),
			Migrations: []string{presentationsMigration, presentationsKindIndexMigration},
		})
		return err
	}); err != nil {
		return err
	}
	loader := familyLoader{workspaceAPIURL: workspaceAPIURL}
	if err := registry.RegisterPromptVariables(ParentProfileID, func(ctx context.Context, rt agentprofiles.RuntimeContext) (map[string]string, error) {
		familyRoot := runtimeRoot(rt.UserID, rt.WorkspacePath)
		state, err := loader.load(ctx, rt.UserID, familyRoot)
		if err != nil {
			return nil, err
		}
		vars := ParentPromptVariables(state)
		vars["INBOX_NOTE"] = InboxNote(loader.listInbox(ctx, rt.UserID, familyRoot))
		return vars, nil
	}); err != nil {
		return err
	}
	return registry.RegisterPromptVariables(ChildProfileID, func(ctx context.Context, rt agentprofiles.RuntimeContext) (map[string]string, error) {
		// The activity folder is the child conversation's workspace; the
		// family root is its projects root's parent.
		activityRoot := runtimeRoot(rt.UserID, rt.WorkspacePath)
		familyRoot := familyRootFromActivity(activityRoot)
		state, err := loader.load(ctx, rt.UserID, familyRoot)
		if err != nil {
			return nil, err
		}
		interests := loader.read(ctx, rt.UserID, path.Join(familyRoot, "memory", "interests.md"))
		return ChildPromptVariables(state, activityRoot, interests), nil
	})
}

// InboxNote turns the unfiled uploads into one turn-context line. The prompt
// used to make the model run `ls inbox/` before every reply; the platform
// renders the prompt per turn, so it can just say what is there.
func InboxNote(files []string) string {
	if len(files) == 0 {
		return ""
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if n := strings.TrimSpace(path.Base(f)); n != "" && n != "." && n != "/" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("INBOX — %d file(s) the parent uploaded are not filed yet: %s. File them with the process-file skill as a quiet background step this turn, then answer what the parent actually asked.\n", len(names), strings.Join(names, ", "))
}

// familyRootFromActivity maps ".../Chats/SparkQuill/activities/<activity>"
// to ".../Chats/SparkQuill".
func familyRootFromActivity(activityWorkspace string) string {
	clean := strings.Trim(strings.TrimSpace(activityWorkspace), "/")
	if i := strings.LastIndex(clean, "/activities/"); i >= 0 {
		return clean[:i]
	}
	return path.Dir(clean)
}

type familyLoader struct {
	workspaceAPIURL string
}

func (l familyLoader) client(userID string) *workspace.Client {
	return workspace.NewClient(l.workspaceAPIURL, workspace.WithUserID(userID))
}

// read returns a workspace file's content, or "" when it does not exist.
func (l familyLoader) read(ctx context.Context, userID, filePath string) string {
	if strings.TrimSpace(l.workspaceAPIURL) == "" {
		return ""
	}
	result, err := l.client(userID).ReadWorkspaceFile(ctx, workspace.ReadWorkspaceFileParams{Filepath: filePath})
	if err != nil {
		return ""
	}
	return result.Content
}

// listInbox returns the files waiting in <familyRoot>/inbox, or nothing when
// the folder is missing or the workspace cannot be reached (the note is a
// convenience, never a reason to fail a turn).
func (l familyLoader) listInbox(ctx context.Context, userID, familyRoot string) []string {
	if strings.TrimSpace(l.workspaceAPIURL) == "" {
		return nil
	}
	depth := 1
	result, err := l.client(userID).ListWorkspaceFiles(ctx, workspace.ListWorkspaceFilesParams{Folder: path.Join(strings.Trim(familyRoot, "/"), "inbox"), MaxDepth: &depth})
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range parseFolderListing(result.Raw) {
		if e.Type == "folder" || strings.TrimSpace(e.FilePath) == "" || strings.HasSuffix(e.FilePath, ".meta.json") {
			continue
		}
		files = append(files, e.FilePath)
	}
	return files
}

type folderEntry struct {
	FilePath string        `json:"filepath"`
	Type     string        `json:"type"`
	Children []folderEntry `json:"children,omitempty"`
}

// parseFolderListing reads the documents API's folder listing in the shapes
// it is known to use: a bare array, {data: [...]}, or a single folder object
// whose children are the listing.
func parseFolderListing(raw json.RawMessage) []folderEntry {
	var list []folderEntry
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Data) > 0 {
		if json.Unmarshal(wrapped.Data, &list) == nil {
			return list
		}
		var one folderEntry
		if json.Unmarshal(wrapped.Data, &one) == nil {
			return one.Children
		}
	}
	var one folderEntry
	if json.Unmarshal(raw, &one) == nil {
		return one.Children
	}
	return nil
}

// load reads family.json from the product root; a missing file is a new
// family (empty state), a corrupt one is an error worth surfacing.
func (l familyLoader) load(ctx context.Context, userID, familyRoot string) (FamilyState, error) {
	var state FamilyState
	raw := l.read(ctx, userID, path.Join(strings.Trim(familyRoot, "/"), FamilyFile))
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, fmt.Errorf("parse %s: %w", FamilyFile, err)
	}
	return state, nil
}
