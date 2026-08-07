// Package platformevents defines the product-facing execution event contract.
//
// Products adapt provider and workflow-specific events at their boundary, then
// persist and expose only this stable vocabulary. Product UIs may add their own
// domain events, but must not change the meaning of these core events.
package platformevents

import (
	_ "embed"
	"encoding/json"
	"time"
)

type Type string

const (
	MessageStarted     Type = "message_started"
	MessageDelta       Type = "message_delta"
	MessageCompleted   Type = "message_completed"
	ToolStarted        Type = "tool_started"
	ToolCompleted      Type = "tool_completed"
	ToolFailed         Type = "tool_failed"
	StatusChanged      Type = "status_changed"
	HumanInputRequired Type = "human_input_required"
	RunStarted         Type = "run_started"
	RunCompleted       Type = "run_completed"
	RunFailed          Type = "run_failed"
	RunCancelled       Type = "run_cancelled"
)

//go:embed contract.json
var contractJSON []byte

// CoreTypes comes from the same contract artifact imported by product
// frontends, preventing silent Go/TypeScript inventory drift.
var CoreTypes = loadCoreTypes()

func loadCoreTypes() []Type {
	var contract struct {
		CoreTypes []Type `json:"coreTypes"`
	}
	if err := json.Unmarshal(contractJSON, &contract); err != nil {
		panic("invalid platform execution-event contract: " + err.Error())
	}
	return contract.CoreTypes
}

type Event struct {
	ID                string    `json:"id"`
	ScopeID           string    `json:"scopeId,omitempty"`
	Type              Type      `json:"type"`
	Name              string    `json:"name"`
	Status            string    `json:"status,omitempty"`
	ExecutionID       string    `json:"executionId,omitempty"`
	ParentExecutionID string    `json:"parentExecutionId,omitempty"`
	Message           string    `json:"message,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}
