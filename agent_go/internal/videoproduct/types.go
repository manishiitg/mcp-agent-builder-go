package videoproduct

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Project struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	OwnerID       string    `json:"ownerId"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	SessionStatus string    `json:"sessionStatus"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Message struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	UserID    string    `json:"userId,omitempty"`
	Role      string    `json:"role"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type Asset struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

type Video struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"createdAt"`
	ContentURL string    `json:"contentUrl"`
}

type WorkflowStep struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Position int    `json:"position"`
	Status   string `json:"status"`
	// Summary is the short user-facing description of the stage, sent so the UI
	// renders whichever pipeline is active instead of hardcoding one. The stage
	// agent's own instruction is deliberately not exposed.
	Summary string `json:"summary,omitempty"`
}

type WorkflowRun struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"projectId"`
	Name        string         `json:"name"`
	GroupName   string         `json:"groupName"`
	Status      string         `json:"status"`
	CurrentStep string         `json:"currentStep,omitempty"`
	ExecutionID string         `json:"executionId,omitempty"`
	Steps       []WorkflowStep `json:"steps"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type ProjectContext struct {
	Project       Project
	UserID        string
	WorkspacePath string
	SessionHandle []byte
	History       []Message
	SecretEnv     []string
}
