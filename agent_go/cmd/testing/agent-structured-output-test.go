package testing

import (
	"context"
	"encoding/json"
	"time"

	"mcp-agent/agent_go/pkg/mcpagent"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/openai"
)

// Struct definitions for different structured output tests

// TodoList types for structured output testing
type Subtask struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	Priority       string    `json:"priority"`
	Description    string    `json:"description,omitempty"`
	EstimatedHours int       `json:"estimated_hours,omitempty"`
	Subtasks       []Subtask `json:"subtasks,omitempty"`
	Dependencies   []string  `json:"dependencies,omitempty"`
}

type TodoList struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tasks       []Subtask `json:"tasks"`
	Status      string    `json:"status"`
}

// Project Management types
type ProjectMember struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	Capacity int    `json:"capacity_hours_per_week"`
}

type ProjectMilestone struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DueDate     time.Time `json:"due_date"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress_percentage"`
}

type Project struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Status      string             `json:"status"`
	StartDate   time.Time          `json:"start_date"`
	EndDate     time.Time          `json:"end_date"`
	Budget      float64            `json:"budget"`
	Members     []ProjectMember    `json:"members"`
	Milestones  []ProjectMilestone `json:"milestones"`
	Risks       []string           `json:"risks"`
	Tags        []string           `json:"tags"`
}

// Financial Analysis types
type FinancialMetric struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Change      float64 `json:"change_percentage"`
	Trend       string  `json:"trend"`
	Description string  `json:"description"`
}

type FinancialReport struct {
	ReportID    string            `json:"report_id"`
	CompanyName string            `json:"company_name"`
	ReportDate  time.Time         `json:"report_date"`
	Period      string            `json:"period"`
	Revenue     FinancialMetric   `json:"revenue"`
	Profit      FinancialMetric   `json:"profit"`
	CashFlow    FinancialMetric   `json:"cash_flow"`
	Assets      FinancialMetric   `json:"assets"`
	Liabilities FinancialMetric   `json:"liabilities"`
	Ratios      []FinancialMetric `json:"ratios"`
	Highlights  []string          `json:"highlights"`
	Risks       []string          `json:"risks"`
	Outlook     string            `json:"outlook"`
}

// Technical Documentation types
type CodeExample struct {
	Language    string `json:"language"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Code        string `json:"code"`
	Output      string `json:"output,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

type APIDocumentation struct {
	Endpoint     string        `json:"endpoint"`
	Method       string        `json:"method"`
	Description  string        `json:"description"`
	Parameters   []string      `json:"parameters"`
	Headers      []string      `json:"headers"`
	RequestBody  string        `json:"request_body,omitempty"`
	ResponseBody string        `json:"response_body,omitempty"`
	Examples     []CodeExample `json:"examples"`
	StatusCodes  []int         `json:"status_codes"`
	Notes        string        `json:"notes,omitempty"`
}

type TechnicalDoc struct {
	Title           string             `json:"title"`
	Version         string             `json:"version"`
	LastUpdated     time.Time          `json:"last_updated"`
	Author          string             `json:"author"`
	Summary         string             `json:"summary"`
	Prerequisites   []string           `json:"prerequisites"`
	Installation    string             `json:"installation"`
	Usage           string             `json:"usage"`
	APIEndpoints    []APIDocumentation `json:"api_endpoints"`
	CodeExamples    []CodeExample      `json:"code_examples"`
	Troubleshooting []string           `json:"troubleshooting"`
	References      []string           `json:"references"`
}

var agentStructuredOutputTestCmd = &cobra.Command{
	Use:   "agent-structured-output",
	Short: "Test agent structured output generation across multiple LLM providers and agent modes",
	Long: `Test the agent's structured output methods across multiple LLM providers and agent modes simultaneously.

This test demonstrates cross-provider and cross-agent-mode compatibility by running different structured output tests
with different LLM providers and both Simple and ReAct agent modes:

Test 1: TodoList generation using OpenAI GPT-4o-mini (Simple + ReAct)
Test 2: Project management using AWS Bedrock (Claude) (Simple + ReAct)
Test 3: Financial analysis using Anthropic Direct API (Simple + ReAct)
Test 4: Technical documentation using OpenAI GPT-4.1 (Simple + ReAct)

Environment variables required:
- For OpenAI: OPENAI_API_KEY
- For Bedrock: AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
- For Anthropic: ANTHROPIC_API_KEY

Examples:
  # Run all tests with multiple providers and agent modes
  go run main.go test agent-structured-output
  
  # Run with specific log file
  go run main.go test agent-structured-output --log-file logs/multi-provider-agent-mode-test.log

The test will create multiple agents (both Simple and ReAct) and test structured output schemas including:
- TodoList with nested tasks (OpenAI - Simple & ReAct)
- Project management with team and milestones (Bedrock - Simple & ReAct)
- Financial analysis with metrics and trends (Anthropic - Simple & ReAct)
- Technical documentation with API endpoints and code examples (OpenAI GPT-4.1 - Simple & ReAct)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return testAgentStructuredOutput(cmd)
	},
}

var modelID string

func init() {
	agentStructuredOutputTestCmd.Flags().StringVarP(&modelID, "model", "m", "", "Model ID (optional, uses provider defaults)")
}

func testAgentStructuredOutput(cmd *cobra.Command) error {
	// Get logging configuration from viper
	logFile := viper.GetString("log-file")
	logLevel := viper.GetString("log-level")

	// Initialize test logger
	InitTestLogger(logFile, logLevel)
	logger := GetTestLogger()
	logger.Info("Starting agent structured output test with multiple LLM providers and agent modes")

	// Test 1: TodoList with OpenAI (Simple + ReAct)
	logger.Info("🧪 Test 1: AskStructured with TodoList using OpenAI (Simple + ReAct)")

	openaiLLM, err := openai.New(
		openai.WithModel("gpt-4o-mini"),
	)
	if err != nil {
		logger.Errorf("❌ Failed to create OpenAI LLM: %v", err)
		return err
	}

	// Create OpenAI Simple Agent
	ctx := context.Background()
	openaiSimpleAgent, err := mcpagent.NewSimpleAgent(ctx, openaiLLM, "openai-simple-test", "configs/mcp_servers_simple.json", "gpt-4o-mini", nil, "openai-simple-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create OpenAI Simple agent: %w", err)
		return err
	}
	logger.Info("✅ OpenAI Simple Agent created successfully")

	// Create OpenAI ReAct Agent
	openaiReActAgent, err := mcpagent.NewReActAgent(ctx, openaiLLM, "openai-react-test", "configs/mcp_servers_simple.json", "gpt-4o-mini", nil, "openai-react-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create OpenAI ReAct agent: %w", err)
		return err
	}
	logger.Info("✅ OpenAI ReAct Agent created successfully")

	// Define the exact schema we want
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
						"description": {"type": "string"},
						"estimated_hours": {"type": "integer"},
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

	// Test Simple Agent
	logger.Info("🔍 Testing OpenAI Simple Agent...")
	todoResponseSimple, err := mcpagent.AskStructured(openaiSimpleAgent, ctx, "Create a simple todo list with 2 tasks for learning Go programming.", TodoList{}, todoSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured TodoList with OpenAI Simple Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured TodoList with OpenAI Simple Agent successful")
		logger.Infof("Title: %s", todoResponseSimple.Title)
		logger.Infof("Description: %s", todoResponseSimple.Description)
		logger.Infof("Status: %s", todoResponseSimple.Status)
		logger.Infof("Number of tasks: %d", len(todoResponseSimple.Tasks))

		for i, task := range todoResponseSimple.Tasks {
			logger.Infof("Task %d: %s (Priority: %s, Status: %s)", i+1, task.Title, task.Priority, task.Status)
		}
	}

	// Test ReAct Agent
	logger.Info("🔍 Testing OpenAI ReAct Agent...")
	todoResponseReAct, err := mcpagent.AskStructured(openaiReActAgent, ctx, "Create a simple todo list with 2 tasks for learning Go programming.", TodoList{}, todoSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured TodoList with OpenAI ReAct Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured TodoList with OpenAI ReAct Agent successful")
		logger.Infof("Title: %s", todoResponseReAct.Title)
		logger.Infof("Description: %s", todoResponseReAct.Description)
		logger.Infof("Status: %s", todoResponseReAct.Status)
		logger.Infof("Number of tasks: %d", len(todoResponseReAct.Tasks))

		for i, task := range todoResponseReAct.Tasks {
			logger.Infof("Task %d: %s (Priority: %s, Status: %s)", i+1, task.Title, task.Priority, task.Status)
		}
	}

	// Test 2: Project Management with AWS Bedrock (Simple + ReAct)
	logger.Info("🧪 Test 2: AskStructured with Project Management using AWS Bedrock (Simple + ReAct)")

	bedrockLLM, err := bedrock.New(
		bedrock.WithModel("us.anthropic.claude-sonnet-4-20250514-v1:0"),
	)
	if err != nil {
		logger.Errorf("❌ Failed to create Bedrock LLM: %v", err)
		return err
	}

	// Create Bedrock Simple Agent
	bedrockSimpleAgent, err := mcpagent.NewSimpleAgent(ctx, bedrockLLM, "bedrock-simple-test", "configs/mcp_servers_simple.json", "us.anthropic.claude-sonnet-4-20250514-v1:0", nil, "bedrock-simple-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create Bedrock Simple agent: %w", err)
		return err
	}
	logger.Info("✅ Bedrock Simple Agent created successfully")

	// Create Bedrock ReAct Agent
	bedrockReActAgent, err := mcpagent.NewReActAgent(ctx, bedrockLLM, "bedrock-react-test", "configs/mcp_servers_simple.json", "us.anthropic.claude-sonnet-4-20250514-v1:0", nil, "bedrock-react-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create Bedrock ReAct agent: %w", err)
		return err
	}
	logger.Info("✅ Bedrock ReAct Agent created successfully")

	projectSchema := `{
		"type": "object",
		"properties": {
			"id": {"type": "string"},
			"name": {"type": "string"},
			"description": {"type": "string"},
			"status": {"type": "string"},
			"start_date": {"type": "string", "format": "date-time"},
			"end_date": {"type": "string", "format": "date-time"},
			"budget": {"type": "number"},
			"members": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"name": {"type": "string"},
						"role": {"type": "string"},
						"email": {"type": "string"},
						"capacity_hours_per_week": {"type": "integer"}
					},
					"required": ["id", "name", "role", "email", "capacity_hours_per_week"]
				}
			},
			"milestones": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"title": {"type": "string"},
						"description": {"type": "string"},
						"due_date": {"type": "string", "format": "date-time"},
						"status": {"type": "string"},
						"progress_percentage": {"type": "integer"}
					},
					"required": ["id", "title", "description", "due_date", "status", "progress_percentage"]
				}
			},
			"risks": {"type": "array", "items": {"type": "string"}},
			"tags": {"type": "array", "items": {"type": "string"}}
		},
		"required": ["id", "name", "description", "status", "start_date", "end_date", "budget", "members", "milestones"]
	}`

	// Test Simple Agent
	logger.Info("🔍 Testing Bedrock Simple Agent...")
	projectResponseSimple, err := mcpagent.AskStructured(bedrockSimpleAgent, ctx, "Create a project plan for developing a new AI-powered chatbot with 3 team members and 4 milestones.", Project{}, projectSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured Project with Bedrock Simple Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured Project with Bedrock Simple Agent successful")
		logger.Infof("Project: %s", projectResponseSimple.Name)
		logger.Infof("Status: %s", projectResponseSimple.Status)
		logger.Infof("Budget: $%.2f", projectResponseSimple.Budget)
		logger.Infof("Team Members: %d", len(projectResponseSimple.Members))
		logger.Infof("Milestones: %d", len(projectResponseSimple.Milestones))
	}

	// Test ReAct Agent
	logger.Info("🔍 Testing Bedrock ReAct Agent...")
	projectResponseReAct, err := mcpagent.AskStructured(bedrockReActAgent, ctx, "Create a project plan for developing a new AI-powered chatbot with 3 team members and 4 milestones.", Project{}, projectSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured Project with Bedrock ReAct Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured Project with Bedrock ReAct Agent successful")
		logger.Infof("Project: %s", projectResponseReAct.Name)
		logger.Infof("Status: %s", projectResponseReAct.Status)
		logger.Infof("Budget: $%.2f", projectResponseReAct.Budget)
		logger.Infof("Team Members: %d", len(projectResponseReAct.Members))
		logger.Infof("Milestones: %d", len(projectResponseReAct.Milestones))
	}

	// Test 3: Financial Analysis with Anthropic (Simple + ReAct)
	logger.Info("🧪 Test 3: AskStructured with Financial Analysis using Anthropic Direct API (Simple + ReAct)")

	anthropicLLM, err := anthropic.New(
		anthropic.WithModel("claude-4-sonnet-20241022"),
	)
	if err != nil {
		logger.Errorf("❌ Failed to create Anthropic LLM: %v", err)
		return err
	}

	// Create Anthropic Simple Agent
	anthropicSimpleAgent, err := mcpagent.NewSimpleAgent(ctx, anthropicLLM, "anthropic-simple-test", "configs/mcp_servers_simple.json", "claude-4-sonnet-20241022", nil, "anthropic-simple-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create Anthropic Simple agent: %w", err)
		return err
	}
	logger.Info("✅ Anthropic Simple Agent created successfully")

	// Create Anthropic ReAct Agent
	anthropicReActAgent, err := mcpagent.NewReActAgent(ctx, anthropicLLM, "anthropic-react-test", "configs/mcp_servers_simple.json", "claude-4-sonnet-20241022", nil, "anthropic-react-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create Anthropic ReAct agent: %w", err)
		return err
	}
	logger.Info("✅ Anthropic ReAct Agent created successfully")

	financialSchema := `{
		"type": "object",
		"properties": {
			"report_id": {"type": "string"},
			"company_name": {"type": "string"},
			"report_date": {"type": "string", "format": "date-time"},
			"period": {"type": "string"},
			"revenue": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"value": {"type": "number"},
					"unit": {"type": "string"},
					"change_percentage": {"type": "number"},
					"trend": {"type": "string"},
					"description": {"type": "string"}
				},
				"required": ["name", "value", "unit", "change_percentage", "trend", "description"]
			},
			"profit": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"value": {"type": "number"},
					"unit": {"type": "string"},
					"change_percentage": {"type": "number"},
					"trend": {"type": "string"},
					"description": {"type": "string"}
				},
				"required": ["name", "value", "unit", "change_percentage", "trend", "description"]
			},
			"cash_flow": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"value": {"type": "number"},
					"unit": {"type": "string"},
					"change_percentage": {"type": "number"},
					"trend": {"type": "string"},
					"description": {"type": "string"}
				},
				"required": ["name", "value", "unit", "change_percentage", "trend", "description"]
			},
			"assets": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"value": {"type": "number"},
					"unit": {"type": "string"},
					"change_percentage": {"type": "number"},
					"trend": {"type": "string"},
					"description": {"type": "string"}
				},
				"required": ["name", "value", "unit", "change_percentage", "trend", "description"]
			},
			"liabilities": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"value": {"type": "number"},
					"unit": {"type": "string"},
					"change_percentage": {"type": "number"},
					"trend": {"type": "string"},
					"description": {"type": "string"}
				},
				"required": ["name", "value", "unit", "change_percentage", "trend", "description"]
			},
			"ratios": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"value": {"type": "number"},
						"unit": {"type": "string"},
						"change_percentage": {"type": "number"},
						"trend": {"type": "string"},
						"description": {"type": "string"}
					},
					"required": ["name", "value", "unit", "change_percentage", "trend", "description"]
				}
			},
			"highlights": {"type": "array", "items": {"type": "string"}},
			"risks": {"type": "array", "items": {"type": "string"}},
			"outlook": {"type": "string"}
		},
		"required": ["report_id", "company_name", "report_date", "period", "revenue", "profit", "cash_flow", "assets", "liabilities", "ratios", "highlights", "risks", "outlook"]
	}`

	// Test Simple Agent
	logger.Info("🔍 Testing Anthropic Simple Agent...")
	financialResponseSimple, err := mcpagent.AskStructured(anthropicSimpleAgent, ctx, "Create a quarterly financial report for a tech startup showing revenue growth, profitability metrics, and key financial ratios.", FinancialReport{}, financialSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured Financial with Anthropic Simple Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured Financial with Anthropic Simple Agent successful")
		logger.Infof("Company: %s", financialResponseSimple.CompanyName)
		logger.Infof("Period: %s", financialResponseSimple.Period)
		logger.Infof("Revenue: $%.2f %s", financialResponseSimple.Revenue.Value, financialResponseSimple.Revenue.Unit)
		logger.Infof("Profit: $%.2f %s", financialResponseSimple.Profit.Value, financialResponseSimple.Profit.Unit)
		logger.Infof("Financial Ratios: %d", len(financialResponseSimple.Ratios))
	}

	// Test ReAct Agent
	logger.Info("🔍 Testing Anthropic ReAct Agent...")
	financialResponseReAct, err := mcpagent.AskStructured(anthropicReActAgent, ctx, "Create a quarterly financial report for a tech startup showing revenue growth, profitability metrics, and key financial ratios.", FinancialReport{}, financialSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured Financial with Anthropic ReAct Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured Financial with Anthropic ReAct Agent successful")
		logger.Infof("Company: %s", financialResponseReAct.CompanyName)
		logger.Infof("Period: %s", financialResponseReAct.Period)
		logger.Infof("Revenue: $%.2f %s", financialResponseReAct.Revenue.Value, financialResponseReAct.Revenue.Unit)
		logger.Infof("Profit: $%.2f %s", financialResponseReAct.Profit.Value, financialResponseReAct.Profit.Unit)
		logger.Infof("Financial Ratios: %d", len(financialResponseReAct.Ratios))
	}

	// Test 4: Technical Documentation with OpenAI (different model) (Simple + ReAct)
	logger.Info("🧪 Test 4: AskStructured with Technical Documentation using OpenAI GPT-4.1 (Simple + ReAct)")

	openaiGPT4LLM, err := openai.New(
		openai.WithModel("gpt-4.1"),
	)
	if err != nil {
		logger.Errorf("❌ Failed to create OpenAI GPT-4 LLM: %v", err)
		return err
	}

	// Create OpenAI GPT-4.1 Simple Agent
	openaiGPT4SimpleAgent, err := mcpagent.NewSimpleAgent(ctx, openaiGPT4LLM, "openai-gpt4.1-simple-test", "configs/mcp_servers_simple.json", "gpt-4.1", nil, "openai-gpt4.1-simple-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create OpenAI GPT-4.1 Simple agent: %w", err)
		return err
	}
	logger.Info("✅ OpenAI GPT-4.1 Simple Agent created successfully")

	// Create OpenAI GPT-4.1 ReAct Agent
	openaiGPT4ReActAgent, err := mcpagent.NewReActAgent(ctx, openaiGPT4LLM, "openai-gpt4.1-react-test", "configs/mcp_servers_simple.json", "gpt-4.1", nil, "openai-gpt4.1-react-trace", logger)
	if err != nil {
		logger.Errorf("❌ Failed to create OpenAI GPT-4.1 ReAct agent: %w", err)
		return err
	}
	logger.Info("✅ OpenAI GPT-4 ReAct Agent created successfully")

	techDocSchema := `{
		"type": "object",
		"properties": {
			"title": {"type": "string"},
			"version": {"type": "string"},
			"last_updated": {"type": "string", "format": "date-time"},
			"author": {"type": "string"},
			"summary": {"type": "string"},
			"prerequisites": {"type": "array", "items": {"type": "string"}},
			"installation": {"type": "string"},
			"usage": {"type": "string"},
			"api_endpoints": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"endpoint": {"type": "string"},
						"method": {"type": "string"},
						"description": {"type": "string"},
						"parameters": {"type": "array", "items": {"type": "string"}},
						"headers": {"type": "array", "items": {"type": "string"}},
						"request_body": {"type": "string"},
						"response_body": {"type": "string"},
						"examples": {
							"type": "array",
							"items": {
								"type": "object",
								"properties": {
									"language": {"type": "string"},
									"title": {"type": "string"},
									"description": {"type": "string"},
									"code": {"type": "string"},
									"output": {"type": "string"},
									"notes": {"type": "string"}
								},
								"required": ["language", "title", "description", "code"]
							}
						},
						"status_codes": {"type": "array", "items": {"type": "integer"}},
						"notes": {"type": "string"}
					},
					"required": ["endpoint", "method", "description", "parameters", "headers", "examples", "status_codes"]
				}
			},
			"code_examples": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"language": {"type": "string"},
						"title": {"type": "string"},
						"description": {"type": "string"},
						"code": {"type": "string"},
						"output": {"type": "string"},
						"notes": {"type": "string"}
					},
					"required": ["language", "title", "description", "code"]
				}
			},
			"troubleshooting": {"type": "array", "items": {"type": "string"}},
			"references": {"type": "array", "items": {"type": "string"}}
		},
		"required": ["title", "version", "last_updated", "author", "summary", "prerequisites", "installation", "usage", "api_endpoints", "code_examples", "troubleshooting", "references"]
	}`

	// Test Simple Agent
	logger.Info("🔍 Testing OpenAI GPT-4.1 Simple Agent...")
	techDocResponseSimple, err := mcpagent.AskStructured(openaiGPT4SimpleAgent, ctx, "Create technical documentation for a REST API that manages user authentication with endpoints for login, logout, and user profile management. Include code examples in Python and JavaScript.", TechnicalDoc{}, techDocSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured Technical Doc with OpenAI GPT-4.1 Simple Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured Technical Doc with OpenAI GPT-4.1 Simple Agent successful")
		logger.Infof("Title: %s", techDocResponseSimple.Title)
		logger.Infof("Version: %s", techDocResponseSimple.Version)
		logger.Infof("Author: %s", techDocResponseSimple.Author)
		logger.Infof("API Endpoints: %d", len(techDocResponseSimple.APIEndpoints))
		logger.Infof("Code Examples: %d", len(techDocResponseSimple.CodeExamples))
	}

	// Test ReAct Agent
	logger.Info("🔍 Testing OpenAI GPT-4.1 ReAct Agent...")
	techDocResponseReAct, err := mcpagent.AskStructured(openaiGPT4ReActAgent, ctx, "Create technical documentation for a REST API that manages user authentication with endpoints for login, logout, and user profile management. Include code examples in Python and JavaScript.", TechnicalDoc{}, techDocSchema)
	if err != nil {
		logger.Errorf("❌ AskStructured Technical Doc with OpenAI GPT-4.1 ReAct Agent failed: %v", err)
	} else {
		logger.Info("✅ AskStructured Technical Doc with OpenAI GPT-4.1 ReAct Agent successful")
		logger.Infof("Title: %s", techDocResponseReAct.Title)
		logger.Infof("Version: %s", techDocResponseReAct.Version)
		logger.Infof("Author: %s", techDocResponseReAct.Author)
		logger.Infof("API Endpoints: %d", len(techDocResponseReAct.APIEndpoints))
		logger.Infof("Code Examples: %d", len(techDocResponseReAct.CodeExamples))
	}

	// Test 5: JSON validation for all responses (Simple + ReAct)
	logger.Info("🧪 Test 5: JSON validation for all structured outputs across different providers and agent modes")

	// Validate TodoList (OpenAI - Simple)
	if todoResponseSimple.Title != "" {
		jsonBytes, _ := json.MarshalIndent(todoResponseSimple, "", "  ")
		logger.Infof("OpenAI Simple Agent TodoList JSON:\n%s", string(jsonBytes))
	}

	// Validate TodoList (OpenAI - ReAct)
	if todoResponseReAct.Title != "" {
		jsonBytes, _ := json.MarshalIndent(todoResponseReAct, "", "  ")
		logger.Infof("OpenAI ReAct Agent TodoList JSON:\n%s", string(jsonBytes))
	}

	// Validate Project (Bedrock - Simple)
	if projectResponseSimple.Name != "" {
		jsonBytes, _ := json.MarshalIndent(projectResponseSimple, "", "  ")
		logger.Infof("Bedrock Simple Agent Project JSON:\n%s", string(jsonBytes))
	}

	// Validate Project (Bedrock - ReAct)
	if projectResponseReAct.Name != "" {
		jsonBytes, _ := json.MarshalIndent(projectResponseReAct, "", "  ")
		logger.Infof("Bedrock ReAct Agent Project JSON:\n%s", string(jsonBytes))
	}

	// Validate Financial Report (Anthropic - Simple)
	if financialResponseSimple.CompanyName != "" {
		jsonBytes, _ := json.MarshalIndent(financialResponseSimple, "", "  ")
		logger.Infof("Anthropic Simple Agent Financial Report JSON:\n%s", string(jsonBytes))
	}

	// Validate Financial Report (Anthropic - ReAct)
	if financialResponseReAct.CompanyName != "" {
		jsonBytes, _ := json.MarshalIndent(financialResponseReAct, "", "  ")
		logger.Infof("Anthropic ReAct Agent Financial Report JSON:\n%s", string(jsonBytes))
	}

	// Validate Technical Documentation (OpenAI GPT-4.1 - Simple)
	if techDocResponseSimple.Title != "" {
		jsonBytes, _ := json.MarshalIndent(techDocResponseSimple, "", "  ")
		logger.Infof("OpenAI GPT-4.1 Simple Agent Technical Doc JSON:\n%s", string(jsonBytes))
	}

	// Validate Technical Documentation (OpenAI GPT-4.1 - ReAct)
	if techDocResponseReAct.Title != "" {
		jsonBytes, _ := json.MarshalIndent(techDocResponseReAct, "", "  ")
		logger.Infof("OpenAI GPT-4.1 ReAct Agent Technical Doc JSON:\n%s", string(jsonBytes))
	}

	logger.Info("🎉 All agent structured output tests completed across multiple LLM providers and agent modes!")
	logger.Info("📊 Provider and Agent Mode Summary:")
	logger.Info("  • OpenAI GPT-4o-mini: TodoList generation (Simple + ReAct)")
	logger.Info("  • AWS Bedrock (Claude): Project management (Simple + ReAct)")
	logger.Info("  • Anthropic Direct API: Financial analysis (Simple + ReAct)")
	logger.Info("  • OpenAI GPT-4.1: Technical documentation (Simple + ReAct)")
	logger.Info("🔍 Agent Mode Comparison:")
	logger.Info("  • Simple Agent: Direct tool usage without explicit reasoning")
	logger.Info("  • ReAct Agent: Explicit reasoning with step-by-step thinking")
	return nil
}
