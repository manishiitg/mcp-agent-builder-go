package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "langfuse-debug",
	Short: "Debug Langfuse API - Fetch traces, spans, and sessions",
	Long: `Debug tool for retrieving and inspecting existing Langfuse traces, spans, and sessions.
This tool only fetches data - it does not create new traces or spans.`,
}

var langfuseCmd = &cobra.Command{
	Use:   "langfuse",
	Short: "Debug Langfuse API - Fetch traces, spans, and sessions",
	Long: `Debug tool for retrieving and inspecting existing Langfuse traces, spans, and sessions.
This tool only fetches data - it does not create new traces or spans.`,
	Run: func(cmd *cobra.Command, args []string) {
		debug, _ := cmd.Flags().GetBool("debug")
		traceID, _ := cmd.Flags().GetString("trace-id")
		sessionID, _ := cmd.Flags().GetString("session-id")

		if debug {
			fmt.Println("Debug mode enabled")
		}

		// Load environment variables from .env file
		if err := godotenv.Load(".env"); err != nil {
			fmt.Printf("⚠️  Could not load .env file: %v\n", err)
		}

		// Check required environment variables (after loading .env)
		publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
		secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
		host := os.Getenv("LANGFUSE_HOST")

		if publicKey == "" || secretKey == "" {
			fmt.Println("Error: LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY must be set")
			fmt.Println("Found in .env:")
			fmt.Printf("  LANGFUSE_PUBLIC_KEY: %s\n", publicKey)
			fmt.Printf("  LANGFUSE_SECRET_KEY: %s\n", secretKey)
			fmt.Printf("  LANGFUSE_HOST: %s\n", host)
			return
		}

		// Set default host if not provided
		if host == "" {
			host = "https://cloud.langfuse.com"
			os.Setenv("LANGFUSE_HOST", host)
			fmt.Printf("Using default Langfuse host: %s\n", host)
		}

		fmt.Println("🔍 Langfuse Debug Tool - Fetch Mode Only")
		fmt.Printf("📊 Host: %s\n", host)
		fmt.Printf("🔑 Public Key: %s...\n", publicKey[:10])

		// Fetch traces based on provided parameters
		if traceID != "" {
			fmt.Printf("\n📋 Fetching specific trace: %s\n", traceID)
			fetchSingleTrace(host, publicKey, secretKey, traceID, debug)
		} else if sessionID != "" {
			fmt.Printf("\n📋 Fetching traces by session ID: %s\n", sessionID)
			fetchTracesBySessionID(host, publicKey, secretKey, sessionID, debug)
		} else {
			fmt.Println("\n📋 Fetching recent traces")
			fetchRecentTraces(host, publicKey, secretKey, debug)
		}
	},
}

// Langfuse API response structures
type LangfuseTrace struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	UserID       *string                `json:"userId"`
	SessionID    *string                `json:"sessionId"`
	Version      *string                `json:"version"`
	Release      *string                `json:"release"`
	Timestamp    string                 `json:"timestamp"`
	Public       bool                   `json:"public"`
	Bookmarked   bool                   `json:"bookmarked"`
	Tags         []string               `json:"tags"`
	Input        map[string]interface{} `json:"input"`
	Output       map[string]interface{} `json:"output"`
	Metadata     map[string]interface{} `json:"metadata"`
	Observations []LangfuseObservation  `json:"observations"`
	Scores       []LangfuseScore        `json:"scores"`
}

type LangfuseObservation struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Type                string                 `json:"type"`
	StartTime           string                 `json:"startTime"`
	EndTime             *string                `json:"endTime"`
	CompletionTime      *string                `json:"completionStartTime"`
	Model               *string                `json:"model"`
	Input               map[string]interface{} `json:"input"`
	Output              map[string]interface{} `json:"output"`
	Metadata            map[string]interface{} `json:"metadata"`
	Level               string                 `json:"level"`
	StatusMessage       *string                `json:"statusMessage"`
	ParentObservationID *string                `json:"parentObservationId"`
	Usage               map[string]interface{} `json:"usage"`
	TotalCost           *float64               `json:"totalCost"`
}

type LangfuseScore struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Value    interface{}            `json:"value"`
	Comment  *string                `json:"comment"`
	Source   string                 `json:"source"`
	DataType string                 `json:"dataType"`
	Config   map[string]interface{} `json:"config"`
}

type LangfuseTracesResponse struct {
	Data []LangfuseTrace `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	} `json:"meta"`
}

func fetchSingleTrace(host, publicKey, secretKey, traceID string, debug bool) {
	fmt.Printf("\n=== Fetching Trace: %s ===\n", traceID)

	apiURL := fmt.Sprintf("%s/api/public/traces/%s", host, traceID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.SetBasicAuth(publicKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	if debug {
		fmt.Printf("Making request to: %s\n", apiURL)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	if resp.StatusCode != 200 {
		fmt.Printf("API Error (Status %d): %s\n", resp.StatusCode, string(body))
		return
	}

	// Debug: Show raw response
	if debug {
		fmt.Printf("Raw API Response: %s\n", string(body))
	}

	// First try to parse as a single trace
	var trace LangfuseTrace
	if err := json.Unmarshal(body, &trace); err != nil {
		fmt.Printf("Error parsing single trace: %v\n", err)
		fmt.Printf("Response might be a list of traces. Trying to parse as traces list...\n")

		// Try to parse as traces response
		var tracesResponse LangfuseTracesResponse
		if listErr := json.Unmarshal(body, &tracesResponse); listErr != nil {
			fmt.Printf("Error parsing traces list: %v\n", listErr)
			fmt.Printf("Raw response (first 500 chars): %s...\n", string(body)[:min(500, len(body))])
			return
		}

		if len(tracesResponse.Data) == 0 {
			fmt.Printf("No traces found in response\n")
			return
		}

		fmt.Printf("Found %d traces in list. Analyzing first trace...\n", len(tracesResponse.Data))
		trace = tracesResponse.Data[0]
	}

	fmt.Printf("✓ Found trace: %s\n", trace.Name)
	fmt.Printf("  ID: %s\n", trace.ID)
	fmt.Printf("  Timestamp: %s\n", trace.Timestamp)
	fmt.Printf("  Public: %v\n", trace.Public)
	fmt.Printf("  Observations: %d\n", len(trace.Observations))
	fmt.Printf("  Scores: %d\n", len(trace.Scores))

	// Check for final output in trace metadata
	if trace.Output != nil && len(trace.Output) > 0 {
		fmt.Printf("  ✅ Final Output Present: %v\n", trace.Output)
	} else if trace.Output == nil {
		fmt.Printf("  ❌ MISSING Final Output in Trace (null)!\n")
	} else {
		fmt.Printf("  ❌ MISSING Final Output in Trace (empty)!\n")
	}

	if len(trace.Tags) > 0 {
		fmt.Printf("  Tags: %v\n", trace.Tags)
	}

	if trace.UserID != nil {
		fmt.Printf("  User ID: %s\n", *trace.UserID)
	}

	if trace.SessionID != nil {
		fmt.Printf("  Session ID: %s\n", *trace.SessionID)
	}

	if debug && len(trace.Observations) > 0 {
		fmt.Printf("\n  Observations:\n")
		for i, obs := range trace.Observations {
			fmt.Printf("    %d. %s (%s)\n", i+1, obs.Name, obs.Type)
			fmt.Printf("       ID: %s\n", obs.ID)
			if obs.Model != nil {
				fmt.Printf("       Model: %s\n", *obs.Model)
			}
		}
	}

	if debug && len(trace.Scores) > 0 {
		fmt.Printf("\n  Scores:\n")
		for i, score := range trace.Scores {
			fmt.Printf("    %d. %s: %v\n", i+1, score.Name, score.Value)
			if score.Comment != nil {
				fmt.Printf("       Comment: %s\n", *score.Comment)
			}
		}
	}
}

func fetchTracesBySessionID(host, publicKey, secretKey, sessionID string, debug bool) {
	fmt.Printf("\n=== Fetching Traces by Session ID: %s ===\n", sessionID)

	// Build URL with query parameters
	apiURL := fmt.Sprintf("%s/api/public/traces", host)
	params := url.Values{}
	params.Add("limit", "50")
	params.Add("orderBy", "timestamp")
	params.Add("orderDirection", "desc")
	params.Add("sessionId", sessionID)

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.SetBasicAuth(publicKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	if debug {
		fmt.Printf("Making request to: %s\n", fullURL)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	if resp.StatusCode != 200 {
		fmt.Printf("API Error (Status %d): %s\n", resp.StatusCode, string(body))
		return
	}

	// Debug: Show raw response
	if debug {
		fmt.Printf("Raw API Response: %s\n", string(body))
	}

	var response LangfuseTracesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	fmt.Printf("✓ Found %d traces (Page %d of %d, Total: %d)\n",
		len(response.Data), response.Meta.Page, response.Meta.TotalPages, response.Meta.TotalItems)

	if len(response.Data) == 0 {
		fmt.Println("No traces found for this session ID.")
		return
	}

	fmt.Println("\nRecent traces:")
	for i, trace := range response.Data {
		timestamp := trace.Timestamp
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			timestamp = t.Format("2006-01-02 15:04:05")
		}

		fmt.Printf("%d. %s (ID: %s)\n", i+1, trace.Name, trace.ID)
		fmt.Printf("   Created: %s\n", timestamp)
		fmt.Printf("   Observations: %d, Scores: %d\n", len(trace.Observations), len(trace.Scores))

		if debug {
			if trace.UserID != nil {
				fmt.Printf("   User: %s\n", *trace.UserID)
			}
			if trace.SessionID != nil {
				fmt.Printf("   Session: %s\n", *trace.SessionID)
			}
		}

		fmt.Println()
	}

	fmt.Println("To fetch a specific trace, use:")
	fmt.Printf("  ./langfuse-debug --trace-id <TRACE_ID>\n")
	fmt.Printf("To fetch traces by session ID, use:\n")
	fmt.Printf("  ./langfuse-debug --session-id <SESSION_ID>\n")
}

func fetchRecentTraces(host, publicKey, secretKey string, debug bool) {
	fmt.Println("\n=== Fetching Recent Traces ===")

	// Build URL with query parameters
	apiURL := fmt.Sprintf("%s/api/public/traces", host)
	params := url.Values{}
	params.Add("limit", "10")
	// Removed problematic orderBy and orderDirection parameters

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.SetBasicAuth(publicKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	if debug {
		fmt.Printf("Making request to: %s\n", fullURL)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	if resp.StatusCode != 200 {
		fmt.Printf("API Error (Status %d): %s\n", resp.StatusCode, string(body))
		return
	}

	// Debug: Show raw response
	if debug {
		fmt.Printf("Raw API Response: %s\n", string(body))
	}

	var response LangfuseTracesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	fmt.Printf("✓ Found %d traces (Page %d of %d, Total: %d)\n",
		len(response.Data), response.Meta.Page, response.Meta.TotalPages, response.Meta.TotalItems)

	if len(response.Data) == 0 {
		fmt.Println("No traces found.")
		return
	}

	fmt.Println("\nRecent traces:")
	for i, trace := range response.Data {
		timestamp := trace.Timestamp
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			timestamp = t.Format("2006-01-02 15:04:05")
		}

		fmt.Printf("%d. %s (ID: %s)\n", i+1, trace.Name, trace.ID)
		fmt.Printf("   Created: %s\n", timestamp)
		fmt.Printf("   Observations: %d, Scores: %d\n", len(trace.Observations), len(trace.Scores))

		if debug {
			if trace.UserID != nil {
				fmt.Printf("   User: %s\n", *trace.UserID)
			}
			if trace.SessionID != nil {
				fmt.Printf("   Session: %s\n", *trace.SessionID)
			}
		}

		fmt.Println()
	}

	fmt.Println("To fetch a specific trace, use:")
	fmt.Printf("  ./langfuse-debug --trace-id <TRACE_ID>\n")
	fmt.Printf("To fetch traces by session ID, use:\n")
	fmt.Printf("  ./langfuse-debug --session-id <SESSION_ID>\n")
}

// All trace creation functions have been removed - this is now a read-only debug tool

func init() {
	rootCmd.AddCommand(langfuseCmd)

	// Define flags for the langfuse command
	langfuseCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	langfuseCmd.Flags().StringP("trace-id", "", "", "Specific trace ID to fetch")
	langfuseCmd.Flags().StringP("session-id", "s", "", "Session ID to use for traces")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
