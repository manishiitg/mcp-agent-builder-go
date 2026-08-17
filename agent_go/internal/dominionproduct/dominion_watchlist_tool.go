package dominionproduct

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

// The workflow's own variables.json, the same file the Dominion dashboard's
// WatchlistEditor reads/writes via the generic /api/workflow/variable-groups
// endpoint (frontend/src/products/dominion/adapters/watchlist.ts). This tool
// deliberately does NOT go through that endpoint -- it talks to the
// workspace API's generic document GET/PUT directly, the same underlying
// calls handleGetVariableGroups/handleUpdateVariableGroups
// (agent_go/cmd/server/workflow.go) make, so this stays a plain read-modify-
// write on the one file rather than routing a tool call back into the main
// server's own HTTP handlers.
const dominionWatchlistWorkspacePath = "Workflow/tectonicusadaytrading/variables/variables.json"
const dominionTickersVariableName = "TICKERS"

type dominionVariable struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description"`
}

type dominionVariableGroup struct {
	Name    string            `json:"name"`
	Values  map[string]string `json:"values"`
	Enabled bool              `json:"enabled"`
}

type dominionVariablesManifest struct {
	Objective      string                  `json:"objective"`
	Variables      []dominionVariable      `json:"variables"`
	Groups         []dominionVariableGroup `json:"groups,omitempty"`
	ExtractionDate string                  `json:"extraction_date"`
}

type dominionWatchlistItem struct {
	Symbol string `json:"symbol"`
	Tier   string `json:"tier"`
}

// addWatchlistSymbolFactory is deliberately ADD-ONLY: it will not remove or
// re-tier an existing symbol, only append a new one (refusing on a
// duplicate). Removing/editing stays a dashboard-only action -- see the
// tool's own description and the system prompt, both of which the model is
// told to take at face value rather than find a workaround for.
func addWatchlistSymbolFactory(workspaceAPIURL string) agentprofiles.ToolFactory {
	return func(runtime agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		return agentprofiles.ToolSpec{
			Name:        "add_dominion_watchlist_symbol",
			Category:    "dominion_watchlist",
			Description: "Add ONE new stock symbol to the Dominion watchlist. This tool can only ADD -- it cannot remove a symbol or change an existing symbol's tier; the user must do that from the Dominion dashboard's Watchlist panel. Refuses (no error, just a message) if the symbol is already on the watchlist.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Stock ticker symbol to add, e.g. NVDA. Case-insensitive; stored uppercased.",
					},
					"tier": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"large", "mid", "small"},
						"description": "Market-cap tier for the new symbol.",
					},
				},
				"required": []string{"symbol", "tier"},
			},
			Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
				symbol := strings.ToUpper(strings.TrimSpace(stringArg(args, "symbol")))
				tier := strings.TrimSpace(stringArg(args, "tier"))
				if symbol == "" {
					return "symbol is required.", nil
				}
				switch tier {
				case "large", "mid", "small":
				default:
					return fmt.Sprintf("Invalid tier %q. Must be one of: large, mid, small.", tier), nil
				}

				manifest, err := fetchDominionVariablesManifest(ctx, workspaceAPIURL)
				if err != nil {
					return "", err
				}

				var tickers []dominionWatchlistItem
				tickersIndex := -1
				for i, v := range manifest.Variables {
					if v.Name == dominionTickersVariableName {
						tickersIndex = i
						if strings.TrimSpace(v.Value) != "" {
							if err := json.Unmarshal([]byte(v.Value), &tickers); err != nil {
								return "", fmt.Errorf("parse existing %s value: %w", dominionTickersVariableName, err)
							}
						}
						break
					}
				}
				if tickersIndex == -1 {
					return "", fmt.Errorf("%s variable not found in the watchlist manifest", dominionTickersVariableName)
				}

				for _, item := range tickers {
					if strings.EqualFold(item.Symbol, symbol) {
						return fmt.Sprintf("%s is already on the watchlist (tier: %s) -- no change made.", symbol, item.Tier), nil
					}
				}
				tickers = append(tickers, dominionWatchlistItem{Symbol: symbol, Tier: tier})

				encoded, err := json.Marshal(tickers)
				if err != nil {
					return "", fmt.Errorf("encode updated %s value: %w", dominionTickersVariableName, err)
				}
				manifest.Variables[tickersIndex].Value = string(encoded)
				for i := range manifest.Groups {
					if manifest.Groups[i].Values == nil {
						manifest.Groups[i].Values = map[string]string{}
					}
					manifest.Groups[i].Values[dominionTickersVariableName] = string(encoded)
				}

				if err := saveDominionVariablesManifest(ctx, workspaceAPIURL, manifest); err != nil {
					return "", err
				}
				return fmt.Sprintf("Added %s (tier: %s) to the watchlist. It now has %d symbols.", symbol, tier, len(tickers)), nil
			},
		}, nil
	}
}

func dominionDocumentsURL(workspaceAPIURL, workspacePath string) string {
	segments := strings.Split(workspacePath, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return workspaceAPIURL + "/api/documents/" + strings.Join(segments, "/")
}

func fetchDominionVariablesManifest(ctx context.Context, workspaceAPIURL string) (*dominionVariablesManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dominionDocumentsURL(workspaceAPIURL, dominionWatchlistWorkspacePath), nil)
	if err != nil {
		return nil, fmt.Errorf("build watchlist read request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("read watchlist file: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read watchlist response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("watchlist read failed: status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Success bool `json:"success"`
		Data    struct {
			Content string `json:"content"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse watchlist read response: %w", err)
	}
	if !apiResp.Success {
		return nil, fmt.Errorf("watchlist read failed: %s", apiResp.Error)
	}

	var manifest dominionVariablesManifest
	if err := json.Unmarshal([]byte(apiResp.Data.Content), &manifest); err != nil {
		return nil, fmt.Errorf("parse watchlist manifest: %w", err)
	}
	return &manifest, nil
}

func saveDominionVariablesManifest(ctx context.Context, workspaceAPIURL string, manifest *dominionVariablesManifest) error {
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watchlist manifest: %w", err)
	}
	reqBody, err := json.Marshal(map[string]string{"content": string(encoded)})
	if err != nil {
		return fmt.Errorf("encode watchlist write request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, dominionDocumentsURL(workspaceAPIURL, dominionWatchlistWorkspacePath), bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build watchlist write request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("write watchlist file: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("watchlist write failed: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
