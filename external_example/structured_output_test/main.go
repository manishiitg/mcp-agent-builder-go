package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"mcp-agent/agent_go/pkg/external"
)

// Test structs for structured output testing
type TodoList struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Tasks       []Task `json:"tasks"`
	Status      string `json:"status"`
}

type Task struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	Subtasks     []Task   `json:"subtasks,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

type Project struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	TeamMembers []string    `json:"team_members"`
	Milestones  []Milestone `json:"milestones"`
	Status      string      `json:"status"`
}

type Milestone struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DueDate     string `json:"due_date"`
	Status      string `json:"status"`
}

type FinancialReport struct {
	Quarter    string             `json:"quarter"`
	Revenue    float64            `json:"revenue"`
	Profit     float64            `json:"profit"`
	GrowthRate float64            `json:"growth_rate"`
	KeyMetrics map[string]float64 `json:"key_metrics"`
	Status     string             `json:"status"`
}

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load("../../agent_go/.env"); err != nil {
		fmt.Println("No .env file found, using system environment variables")
	}

	// Check required environment variables
	requiredVars := []string{"OPENAI_API_KEY"}
	for _, varName := range requiredVars {
		if os.Getenv(varName) == "" {
			log.Fatalf("❌ Required environment variable %s is not set", varName)
		}
	}

	fmt.Println("🚀 Starting External Agent Structured Output Test")
	fmt.Println("==================================================")

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create external agent using the builder pattern
	config := external.DefaultConfig().
		WithAgentMode(external.SimpleAgent).
		WithLLM("openai", "gpt-4.1-mini", 0.7).
		WithMaxTurns(10).
		WithObservability("console", "")

	// Create external agent
	agent, err := external.NewAgent(ctx, config)
	if err != nil {
		fmt.Printf("❌ Failed to create external agent: %v\n", err)
		os.Exit(1)
	}
	defer agent.Close()

	fmt.Println("✅ External agent created successfully")

	// Initialize the agent
	if err := agent.Initialize(ctx); err != nil {
		fmt.Printf("❌ Failed to initialize external agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ External agent initialized successfully")

	// Test 1: TodoList with AskStructured
	fmt.Println("🧪 Test 1: AskStructured with TodoList")
	todoSchema := `{
		"type": "object",
		"properties": {
			"title": {"type": "string"},
			"description": {"type": "string"},
			"tasks": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"title": {"type": "string"},
						"status": {"type": "string"},
						"priority": {"type": "string"},
						"subtasks": {
							"type": "array",
							"items": {
								"type": "object",
								"properties": {
									"id": {"type": "string"},
									"title": {"type": "string"},
									"status": {"type": "string"}
								},
								"required": ["id", "title", "status"]
							}
						},
						"dependencies": {
							"type": "array",
							"items": {"type": "string"}
						}
					},
					"required": ["id", "title", "status", "priority"]
				}
			},
			"status": {"type": "string"}
		},
		"required": ["title", "description", "tasks", "status"]
	}`

	todoResponse, err := external.AskStructured(agent, ctx, "Create a simple todo list with 2 tasks for learning Go programming.", TodoList{}, todoSchema)
	if err != nil {
		fmt.Printf("❌ AskStructured TodoList failed: %v\n", err)
	} else {
		fmt.Println("✅ AskStructured TodoList successful")
		printStructuredOutput("TodoList", todoResponse)
	}

	// Test 2: Project with AskStructured
	fmt.Println("🧪 Test 2: AskStructured with Project")
	projectSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"description": {"type": "string"},
			"team_members": {
				"type": "array",
				"items": {"type": "string"}
			},
			"milestones": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"title": {"type": "string"},
						"description": {"type": "string"},
						"due_date": {"type": "string"},
						"status": {"type": "string"}
					},
					"required": ["id", "title", "description", "due_date", "status"]
				}
			},
			"status": {"type": "string"}
		},
		"required": ["name", "description", "team_members", "milestones", "status"]
	}`

	projectResponse, err := external.AskStructured(agent, ctx, "Create a project plan for developing a new AI-powered chatbot with 3 team members and 4 milestones.", Project{}, projectSchema)
	if err != nil {
		fmt.Printf("❌ AskStructured Project failed: %v\n", err)
	} else {
		fmt.Println("✅ AskStructured Project successful")
		printStructuredOutput("Project", projectResponse)
	}

	// Test 3: Financial Report with AskStructured
	fmt.Println("🧪 Test 3: AskStructured with Financial Report")
	financialSchema := `{
		"type": "object",
		"properties": {
			"quarter": {"type": "string"},
			"revenue": {"type": "number"},
			"profit": {"type": "number"},
			"growth_rate": {"type": "number"},
			"key_metrics": {
				"type": "object",
				"additionalProperties": {"type": "number"}
			},
			"status": {"type": "string"}
		},
		"required": ["quarter", "revenue", "profit", "growth_rate", "key_metrics", "status"]
	}`

	financialResponse, err := external.AskStructured(agent, ctx, "Create a quarterly financial report for a tech startup showing revenue growth, profitability metrics, and key financial ratios.", FinancialReport{}, financialSchema)
	if err != nil {
		fmt.Printf("❌ AskStructured Financial Report failed: %v\n", err)
	} else {
		fmt.Println("✅ AskStructured Financial Report successful")
		printStructuredOutput("Financial Report", financialResponse)
	}

	fmt.Println("🎉 All structured output tests completed!")
}

// printStructuredOutput prints the structured output in a formatted way
func printStructuredOutput(name string, data interface{}) {
	fmt.Printf("\n📊 %s Output:\n", name)
	fmt.Println("=" + strings.Repeat("=", len(name)+8))

	// Convert to JSON for pretty printing
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("❌ Failed to marshal %s: %v\n", name, err)
		return
	}

	fmt.Println(string(jsonData))
	fmt.Println()
}
