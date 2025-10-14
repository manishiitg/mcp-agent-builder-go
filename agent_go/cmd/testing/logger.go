package testing

import (
	"mcp-agent/agent_go/pkg/logger"
)

var testLogger logger.Logger

// InitTestLogger initializes the shared test logger with specified configuration
// This creates a single logger instance that all tests can use
func InitTestLogger(logFile string, level string) {
	testLogger = logger.CreateTestLogger(logFile, level)
}

// GetTestLogger returns the shared test logger instance
// If no logger has been initialized, creates a default one
func GetTestLogger() logger.Logger {
	if !testLogger.IsInitialized() {
		// Create default test logger if none exists
		testLogger = logger.CreateDefaultLogger()
	}
	return testLogger
}

// SetTestLogger allows tests to override the shared logger
// Useful for testing different logger configurations
func SetTestLogger(logger logger.Logger) {
	testLogger = logger
}
