package workflowtypes

import "strings"

const (
	FolderAccessReadOnly  = "read_only"
	FolderAccessReadWrite = "read_write"
)

// WorkflowFolderGrant is an owner-approved, host-local filesystem capability.
// Agents refer to Alias; they never establish authority by supplying Path.
type WorkflowFolderGrant struct {
	ID        string `json:"id"`
	Alias     string `json:"alias"`
	Path      string `json:"path"`
	Access    string `json:"access"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// WorkflowFolderAccessRequest is an agent-authored proposal awaiting trusted
// user approval. RequestedPath may retain an exact absolute path supplied by
// the user, but it is not an authority grant until the user approves it.
type WorkflowFolderAccessRequest struct {
	ID            string `json:"id"`
	Alias         string `json:"alias"`
	RequestedPath string `json:"requested_path,omitempty"`
	Access        string `json:"access"`
	Reason        string `json:"reason"`
	RequestedAt   string `json:"requested_at"`
}

func (g WorkflowFolderGrant) CanWrite() bool {
	return strings.TrimSpace(g.Access) == FolderAccessReadWrite
}
