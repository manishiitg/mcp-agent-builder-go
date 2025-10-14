package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"mcp-agent/agent_go/pkg/external"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

// 🔑 LOCAL ExtendedLogger interface definition (matches the one used by external package)
type ExtendedLogger interface {
	// Core MCP-Go compatibility methods
	Infof(format string, v ...any)
	Errorf(format string, v ...any)

	// Additional methods we need
	Info(args ...interface{})
	Error(args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})

	// Structured logging methods
	WithField(key string, value interface{}) *logrus.Entry
	WithFields(fields logrus.Fields) *logrus.Entry
	WithError(err error) *logrus.Entry

	// File management
	Close() error
}

// CustomLogger implements our ExtendedLogger interface - writes ONLY to custom file (NO console output)
type CustomLogger struct {
	file   *os.File
	prefix string
}

func NewCustomLogger(prefix string) *CustomLogger {
	// Create custom log file
	file, err := os.OpenFile("my_custom_logs.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Silently fail - don't print to console
		return &CustomLogger{
			file:   nil,
			prefix: prefix,
		}
	}

	return &CustomLogger{
		file:   file,
		prefix: prefix,
	}
}

// 🔑 IMPLEMENTING ExtendedLogger interface - ALL methods write ONLY to file, NO console output

// Core MCP-Go compatibility methods
func (l *CustomLogger) Infof(format string, args ...interface{}) {
	l.logToFile("INFO", format, args...)
}

func (l *CustomLogger) Errorf(format string, args ...interface{}) {
	l.logToFile("ERROR", format, args...)
}

// Additional methods from ExtendedLogger interface
func (l *CustomLogger) Info(args ...interface{}) {
	l.logToFile("INFO", "%v", args...)
}

func (l *CustomLogger) Error(args ...interface{}) {
	l.logToFile("ERROR", "%v", args...)
}

func (l *CustomLogger) Debug(args ...interface{}) {
	l.logToFile("DEBUG", "%v", args...)
}

func (l *CustomLogger) Debugf(format string, args ...interface{}) {
	l.logToFile("DEBUG", format, args...)
}

func (l *CustomLogger) Warn(args ...interface{}) {
	l.logToFile("WARN", "%v", args...)
}

func (l *CustomLogger) Warnf(format string, args ...interface{}) {
	l.logToFile("WARN", format, args...)
}

func (l *CustomLogger) Fatal(args ...interface{}) {
	l.logToFile("FATAL", "%v", args...)
}

func (l *CustomLogger) Fatalf(format string, args ...interface{}) {
	l.logToFile("FATAL", format, args...)
}

// Structured logging methods
func (l *CustomLogger) WithField(key string, value interface{}) *logrus.Entry {
	// Create a minimal logrus entry that won't output to console
	entry := logrus.NewEntry(logrus.New())
	entry.Data = map[string]interface{}{key: value}
	return entry
}

func (l *CustomLogger) WithFields(fields logrus.Fields) *logrus.Entry {
	// Create a minimal logrus entry that won't output to console
	entry := logrus.NewEntry(logrus.New())
	entry.Data = fields
	return entry
}

// 🔑 ADDING MISSING WithError method to complete ExtendedLogger interface
func (l *CustomLogger) WithError(err error) *logrus.Entry {
	// Create a minimal logrus entry that won't output to console
	entry := logrus.NewEntry(logrus.New())
	// Use the proper method to set the error
	return entry.WithError(err)
}

// File management
func (l *CustomLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Private helper method for consistent logging
func (l *CustomLogger) logToFile(level, format string, args ...interface{}) {
	if l.file == nil {
		return // Silently fail if file is not available
	}

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05.000")

	// Add custom prefix to ALL logs
	formatted := fmt.Sprintf("[%s][%s][%s] %s", l.prefix, level, timestamp, msg)

	// Write ONLY to custom file (NO console output)
	l.file.WriteString(fmt.Sprintf("%s\n", formatted))
}

func main() {
	// Load environment variables silently (no console output)
	_ = godotenv.Load()

	// Create custom logger with prefix
	customLogger := NewCustomLogger("MY-AGENT")
	defer customLogger.Close()

	customLogger.Infof("🚀 Starting custom logger test")
	customLogger.Infof("📝 This logger should add '[MY-AGENT]' prefix to ALL logs")
	customLogger.Infof("📁 All logs will be written to 'my_custom_logs.log' ONLY (no console output)")

	// 🔑 UPDATED: Use new simplified configuration pattern from external package
	config := external.DefaultConfig().
		WithAgentMode(external.SimpleAgent).
		WithLLM("openai", "gpt-4o-mini", 0.1).
		WithMaxTurns(3).
		WithServer("filesystem", "mcp_servers.json").
		WithLogger(customLogger) // 🔑 Pass custom logger here

	customLogger.Infof("✅ Configuration created with custom logger")
	customLogger.Infof("🤖 Creating agent...")

	// Create agent
	ctx := context.Background()
	agent, err := external.NewAgent(ctx, config)
	if err != nil {
		customLogger.Errorf("❌ Failed to create agent: %v", err)
		return
	}

	customLogger.Infof("✅ Agent created successfully!")
	customLogger.Infof("🔍 Now testing if agent uses our custom logger...")

	// Test agent operations
	customLogger.Infof("🔍 Test 1: Server health check")
	health := agent.CheckHealth(ctx)
	for server, status := range health {
		if status != nil {
			customLogger.Errorf("❌ Server %s: %v", server, status)
		} else {
			customLogger.Infof("✅ Server %s: healthy", server)
		}
	}

	customLogger.Infof("🔍 Test 2: Simple query")
	answer, err := agent.Invoke(ctx, "What files are available?")
	if err != nil {
		customLogger.Errorf("❌ Query failed: %v", err)
	} else {
		customLogger.Infof("✅ Query answer: %s", answer)
	}

	customLogger.Infof("🎯 ANALYSIS:")
	customLogger.Infof("   If you see '[MY-AGENT]' prefix on ALL logs above, it's working!")
	customLogger.Infof("   If you see some logs WITHOUT the prefix, the agent is NOT using our logger")
	customLogger.Infof("   Check 'my_custom_logs.log' file for all captured logs")
	customLogger.Infof("   Console should be completely clean (no output)")
}
