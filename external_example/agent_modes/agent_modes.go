package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"mcp-agent/agent_go/pkg/external"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

// DemoLogger is a simple logger for the demo
type DemoLogger struct {
	prefix string
}

func (l *DemoLogger) Info(args ...interface{}) {
	log.Printf("[%s][INFO] %s", l.prefix, fmt.Sprint(args...))
}

func (l *DemoLogger) Error(args ...interface{}) {
	log.Printf("[%s][ERROR] %s", l.prefix, fmt.Sprint(args...))
}

func (l *DemoLogger) Debug(args ...interface{}) {
	log.Printf("[%s][DEBUG] %s", l.prefix, fmt.Sprint(args...))
}

func (l *DemoLogger) Warn(args ...interface{}) {
	log.Printf("[%s][WARN] %s", l.prefix, fmt.Sprint(args...))
}

func (l *DemoLogger) Infof(format string, args ...interface{}) {
	log.Printf("[%s][INFO] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *DemoLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[%s][ERROR] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *DemoLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[%s][DEBUG] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *DemoLogger) Warnf(format string, args ...interface{}) {
	log.Printf("[%s][WARN] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *DemoLogger) Fatal(args ...interface{}) {
	log.Fatalf("[%s][FATAL] %s", l.prefix, fmt.Sprint(args...))
}

func (l *DemoLogger) Fatalf(format string, args ...interface{}) {
	log.Fatalf("[%s][FATAL] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *DemoLogger) WithField(key string, value interface{}) *logrus.Entry {
	entry := logrus.NewEntry(logrus.StandardLogger())
	return entry.WithField(key, value)
}

func (l *DemoLogger) WithFields(fields logrus.Fields) *logrus.Entry {
	entry := logrus.NewEntry(logrus.StandardLogger())
	return entry.WithFields(fields)
}

func (l *DemoLogger) WithError(err error) *logrus.Entry {
	entry := logrus.NewEntry(logrus.StandardLogger())
	return entry.WithError(err)
}

func (l *DemoLogger) Close() error {
	return nil
}

// AgentModeDemo demonstrates the differences between Simple and ReAct agent modes
type AgentModeDemo struct {
	simpleAgent external.Agent
	reactAgent  external.Agent
	logger      *DemoLogger
}

// NewAgentModeDemo creates a new demo instance with both agent types
func NewAgentModeDemo() (*AgentModeDemo, error) {
	// Load environment variables
	_ = godotenv.Load()

	// Create custom logger for this demo
	logger := &DemoLogger{
		prefix: "[AGENT-MODES]",
	}

	logger.Info("🚀 Creating Agent Mode Demo")
	logger.Info("📋 This demo will show the differences between Simple and ReAct agents")

	// Create Simple Agent configuration
	simpleConfig := external.DefaultConfig().
		WithAgentMode(external.SimpleAgent).
		WithLLM("openai", "gpt-4o-mini", 0.1).
		WithMaxTurns(10).
		WithServer("filesystem", "mcp_servers.json").
		WithLogger(logger)

	logger.Info("🔧 Simple Agent Configuration:")
	logger.Info(fmt.Sprintf("  - Mode: %s", external.SimpleAgent))
	logger.Info(fmt.Sprintf("  - Max Turns: %d", 10))
	logger.Info(fmt.Sprintf("  - LLM: OpenAI GPT-4o-mini"))

	// Create ReAct Agent configuration
	reactConfig := external.DefaultConfig().
		WithAgentMode(external.ReActAgent).
		WithLLM("openai", "gpt-4o", 0.1).
		WithMaxTurns(20).
		WithServer("filesystem", "mcp_servers.json").
		WithLogger(logger)

	logger.Info("🔧 ReAct Agent Configuration:")
	logger.Info(fmt.Sprintf("  - Mode: %s", external.ReActAgent))
	logger.Info(fmt.Sprintf("  - Max Turns: %d", 20))
	logger.Info(fmt.Sprintf("  - LLM: OpenAI GPT-4o"))

	ctx := context.Background()

	// Create Simple Agent
	logger.Info("🤖 Creating Simple Agent...")
	simpleAgent, err := external.NewAgent(ctx, simpleConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Simple agent: %w", err)
	}
	logger.Info("✅ Simple Agent created successfully")

	// Create ReAct Agent
	logger.Info("🤖 Creating ReAct Agent...")
	reactAgent, err := external.NewAgent(ctx, reactConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReAct agent: %w", err)
	}
	logger.Info("✅ ReAct Agent created successfully")

	return &AgentModeDemo{
		simpleAgent: simpleAgent,
		reactAgent:  reactAgent,
		logger:      logger,
	}, nil
}

// RunSimpleAgentDemo demonstrates the Simple agent mode
func (d *AgentModeDemo) RunSimpleAgentDemo(ctx context.Context) error {
	d.logger.Info("🎯 ========================================")
	d.logger.Info("🎯 RUNNING SIMPLE AGENT DEMO")
	d.logger.Info("🎯 ========================================")

	// Test query that will show Simple agent behavior
	query := "List the files in the current directory and tell me how many there are"

	d.logger.Info(fmt.Sprintf("📝 Query: %s", query))
	d.logger.Info("💡 Simple Agent will: Use tools directly without explanation")
	d.logger.Info("⏱️  Expected: Fast response, fewer turns, direct tool usage")

	startTime := time.Now()
	response, err := d.simpleAgent.Invoke(ctx, query)
	duration := time.Since(startTime)

	if err != nil {
		d.logger.Error(fmt.Sprintf("❌ Simple Agent failed: %v", err))
		return err
	}

	d.logger.Info(fmt.Sprintf("✅ Simple Agent completed in %v", duration))
	d.logger.Info("📤 Response:")
	d.logger.Info("---")
	d.logger.Info(response)
	d.logger.Info("---")

	return nil
}

// RunReActAgentDemo demonstrates the ReAct agent mode
func (d *AgentModeDemo) RunReActAgentDemo(ctx context.Context) error {
	d.logger.Info("🎯 ========================================")
	d.logger.Info("🎯 RUNNING REACT AGENT DEMO")
	d.logger.Info("🎯 ========================================")

	// Test query that will show ReAct agent behavior
	query := "List the files in the current directory and tell me how many there are"

	d.logger.Info(fmt.Sprintf("📝 Query: %s", query))
	d.logger.Info("💡 ReAct Agent will: Think step-by-step with explicit reasoning")
	d.logger.Info("⏱️  Expected: Slower response, more turns, detailed reasoning")

	startTime := time.Now()
	response, err := d.reactAgent.Invoke(ctx, query)
	duration := time.Since(startTime)

	if err != nil {
		d.logger.Error(fmt.Sprintf("❌ ReAct Agent failed: %v", err))
		return err
	}

	d.logger.Info(fmt.Sprintf("✅ ReAct Agent completed in %v", duration))
	d.logger.Info("📤 Response:")
	d.logger.Info("---")
	d.logger.Info(response)
	d.logger.Info("---")

	return nil
}

// RunComparisonDemo runs both agents on the same query for comparison
func (d *AgentModeDemo) RunComparisonDemo(ctx context.Context) error {
	d.logger.Info("🎯 ========================================")
	d.logger.Info("🎯 RUNNING COMPARISON DEMO")
	d.logger.Info("🎯 ========================================")

	// Use a query that will clearly show the differences
	query := "Analyze the current directory structure and provide insights about the project organization"

	d.logger.Info(fmt.Sprintf("📝 Comparison Query: %s", query))
	d.logger.Info("🔍 This will show the key differences between Simple and ReAct modes")

	// Run Simple Agent
	d.logger.Info("🚀 Running Simple Agent...")
	simpleStart := time.Now()
	simpleResponse, simpleErr := d.simpleAgent.Invoke(ctx, query)
	simpleDuration := time.Since(simpleStart)

	if simpleErr != nil {
		d.logger.Error(fmt.Sprintf("❌ Simple Agent failed: %v", simpleErr))
	} else {
		d.logger.Info(fmt.Sprintf("✅ Simple Agent completed in %v", simpleDuration))
		d.logger.Info("📊 Simple Agent Response Length: %d characters", len(simpleResponse))
	}

	// Run ReAct Agent
	d.logger.Info("🚀 Running ReAct Agent...")
	reactStart := time.Now()
	reactResponse, reactErr := d.reactAgent.Invoke(ctx, query)
	reactDuration := time.Since(reactStart)

	if reactErr != nil {
		d.logger.Error(fmt.Sprintf("❌ ReAct Agent failed: %v", reactErr))
	} else {
		d.logger.Info(fmt.Sprintf("✅ ReAct Agent completed in %v", reactDuration))
		d.logger.Info("📊 ReAct Agent Response Length: %d characters", len(reactResponse))
	}

	// Show comparison
	d.logger.Info("📊 ========================================")
	d.logger.Info("📊 COMPARISON RESULTS")
	d.logger.Info("📊 ========================================")

	if simpleErr == nil && reactErr == nil {
		d.logger.Info(fmt.Sprintf("⏱️  Simple Agent: %v", simpleDuration))
		d.logger.Info(fmt.Sprintf("⏱️  ReAct Agent:  %v", reactDuration))
		d.logger.Info(fmt.Sprintf("📈 Speed Difference: %v", reactDuration-simpleDuration))
		d.logger.Info(fmt.Sprintf("📝 Simple Response Length: %d chars", len(simpleResponse)))
		d.logger.Info(fmt.Sprintf("📝 ReAct Response Length: %d chars", len(reactResponse)))
		d.logger.Info(fmt.Sprintf("📊 Length Difference: %d chars", len(reactResponse)-len(simpleResponse)))

		// Show response samples
		d.logger.Info("📤 Simple Agent Response Sample:")
		d.logger.Info("---")
		if len(simpleResponse) > 200 {
			d.logger.Info(simpleResponse[:200] + "...")
		} else {
			d.logger.Info(simpleResponse)
		}
		d.logger.Info("---")

		d.logger.Info("📤 ReAct Agent Response Sample:")
		d.logger.Info("---")
		if len(reactResponse) > 200 {
			d.logger.Info(reactResponse[:200] + "...")
		} else {
			d.logger.Info(reactResponse)
		}
		d.logger.Info("---")
	}

	return nil
}

func main() {
	// Create logs directory
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	// Create demo instance
	demo, err := NewAgentModeDemo()
	if err != nil {
		log.Fatalf("Failed to create demo: %v", err)
	}

	ctx := context.Background()

	// Check command line arguments for different demo modes
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "simple":
			if err := demo.RunSimpleAgentDemo(ctx); err != nil {
				log.Fatalf("Simple agent demo failed: %v", err)
			}
		case "react":
			if err := demo.RunReActAgentDemo(ctx); err != nil {
				log.Fatalf("ReAct agent demo failed: %v", err)
			}
		case "compare":
			if err := demo.RunComparisonDemo(ctx); err != nil {
				log.Fatalf("Comparison demo failed: %v", err)
			}
		default:
			log.Printf("Unknown mode: %s. Available modes: simple, react, compare", os.Args[1])
			log.Printf("Running comparison demo by default...")
			if err := demo.RunComparisonDemo(ctx); err != nil {
				log.Fatalf("Comparison demo failed: %v", err)
			}
		}
	} else {
		// Default: run comparison demo
		log.Printf("No mode specified. Running comparison demo...")
		if err := demo.RunComparisonDemo(ctx); err != nil {
			log.Fatalf("Comparison demo failed: %v", err)
		}
	}

	log.Println("🎉 Demo completed successfully!")
}
