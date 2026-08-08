package workspace

import (
	"context"
	"encoding/json"
	"fmt"
)

type InitializeWorkflowDBParams struct {
	DBPath     string   `json:"db_path"`
	Migrations []string `json:"migrations"`
}

// InitializeWorkflowDB invokes the token-protected generic managed-database
// initializer. Product runtimes use this once per workspace; normal agents do
// not receive this capability as a tool.
func (c *Client) InitializeWorkflowDB(ctx context.Context, params InitializeWorkflowDBParams) error {
	if params.DBPath == "" || len(params.Migrations) == 0 {
		return fmt.Errorf("db_path and migrations are required")
	}
	body, err := c.request(ctx, "POST", "/api/db/initialize", params)
	if err != nil {
		return err
	}
	var response APIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decode database initialization response: %w", err)
	}
	if !response.Success {
		return fmt.Errorf("database initialization failed: %s", response.Error)
	}
	return nil
}
