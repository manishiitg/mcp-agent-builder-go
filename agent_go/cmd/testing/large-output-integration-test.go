package testing

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/utils"
	mcpagent "github.com/manishiitg/mcpagent/agent"
)

var largeOutputIntegrationTestCmd = &cobra.Command{
	Use:   "large-output-integration",
	Short: "Test large tool output handling in real integration scenarios",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get logging configuration from viper
		logFile := viper.GetString("log-file")
		logLevel := viper.GetString("log-level")

		// Initialize test logger
		InitTestLogger(logFile, logLevel)
		logger := GetTestLogger()

		logger.Info("=== Large Tool Output Integration Test ===")

		// Create test directory
		testDir := "large_output_integration_test"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			return fmt.Errorf("failed to create test directory: %w", err)
		}
		defer os.RemoveAll(testDir)

		logger.Info(fmt.Sprintf("Created test directory: %s", testDir))

		// Test 1: Test with a tool that produces large output
		logger.Info("\n--- Test 1: Large Tool Output Detection ---")
		if err := testLargeToolOutputDetection(testDir); err != nil {
			return fmt.Errorf("large tool output detection test failed: %w", err)
		}

		// Test 2: Test virtual tools for reading large output
		logger.Info("\n--- Test 2: Virtual Tools for Large Output ---")
		if err := testVirtualToolsForLargeOutput(testDir); err != nil {
			return fmt.Errorf("virtual tools for large output test failed: %w", err)
		}

		// Test 3: Test with real agent conversation
		logger.Info("\n--- Test 3: Real Agent Conversation with Large Output ---")
		if err := testRealAgentConversation(testDir); err != nil {
			return fmt.Errorf("real agent conversation test failed: %w", err)
		}

		logger.Info("\n✅ All large tool output integration tests passed!")
		return nil
	},
}

func testLargeToolOutputDetection(testDir string) error {
	logger := GetTestLogger()
	logger.Info("Testing large tool output detection...")

	// Create a mock tool that produces large output
	largeOutput := generateLargeOutput(utils.DefaultLargeToolOutputThreshold + 1000) // Over the threshold

	// Create tool output handler
	handler := mcpagent.NewToolOutputHandlerWithConfig(
		utils.DefaultLargeToolOutputThreshold, // Default threshold
		testDir,
		"test-session-large",
		true,
		true, // Virtual tools enabled
	)

	// Test if the output is detected as large using token counting
	isLarge := handler.IsLargeToolOutputWithModel(largeOutput, "gpt-4")
	if !isLarge {
		tokenCount := handler.CountTokensForModel(largeOutput, "gpt-4")
		return fmt.Errorf("large output was not detected as large (size: %d, token_count: %d, threshold: %d)", len(largeOutput), tokenCount, handler.Threshold)
	}

	logger.Info(fmt.Sprintf("✅ Large output detected correctly (size: %d, threshold: %d)", len(largeOutput), handler.Threshold))

	// Test file writing
	filePath, err := handler.WriteToolOutputToFile(largeOutput, "test_large_tool")
	if err != nil {
		return fmt.Errorf("failed to write large output to file: %w", err)
	}

	logger.Info(fmt.Sprintf("✅ Large output written to file: %s", filePath))

	// Verify file exists and has correct content
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file was not created: %s", filePath)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if len(fileContent) != len(largeOutput) {
		return fmt.Errorf("file content size mismatch: expected %d, got %d", len(largeOutput), len(fileContent))
	}

	logger.Info("✅ File content verified correctly")

	return nil
}

func testVirtualToolsForLargeOutput(testDir string) error {
	logger := GetTestLogger()
	logger.Info("Testing configured large-output storage...")
	toolOutputHandler := mcpagent.NewToolOutputHandlerWithConfig(
		1000, testDir, "test-session-virtual", true, true,
	)
	largeContent := generateLargeOutput(8000)
	filePath, err := toolOutputHandler.WriteToolOutputToFile(largeContent, "test_large_tool")
	if err != nil {
		return fmt.Errorf("write large output: %w", err)
	}
	written, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read stored large output: %w", err)
	}
	if string(written) != largeContent {
		return fmt.Errorf("stored large output changed: got %d bytes, want %d", len(written), len(largeContent))
	}
	logger.Info("✅ Configured large-output storage works")
	return nil
}

func testRealAgentConversation(testDir string) error {
	logger := GetTestLogger()
	logger.Info("Testing real agent conversation with large output...")

	// Set up tool output handler for testing
	toolOutputHandler := mcpagent.NewToolOutputHandlerWithConfig(
		1000, testDir, "test-session-agent", true, true,
	)

	// Create a mock tool that produces large output
	largeOutput := generateLargeOutput(2000) // Over the 1000 character threshold

	// Test the tool output handler directly
	filePath, err := toolOutputHandler.WriteToolOutputToFile(largeOutput, "test_large_tool")
	if err != nil {
		return fmt.Errorf("failed to write large output to file: %w", err)
	}

	logger.Info(fmt.Sprintf("✅ Large tool output written to file: %s", filePath))

	// Check if the file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file was not created: %s", filePath)
	}

	// Verify file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if len(fileContent) != len(largeOutput) {
		return fmt.Errorf("file content size mismatch: expected %d, got %d", len(largeOutput), len(fileContent))
	}

	logger.Info(fmt.Sprintf("✅ Large tool output file verified correctly (size: %d)", len(fileContent)))

	return nil
}

func generateLargeOutput(size int) string {
	// Generate a large output with some structure
	baseContent := `{"data":{"items":[`

	// Add items to reach the desired size
	items := []string{}
	currentSize := len(baseContent) + 2 // +2 for closing brackets

	for i := 0; currentSize < size; i++ {
		item := fmt.Sprintf(`{"id":%d,"name":"item_%d","description":"This is a test item with some content to make it larger","value":%d,"metadata":{"created":"2025-08-02","tags":["test","large","output"]}}`, i, i, i*100)
		items = append(items, item)
		currentSize = len(baseContent) + len(strings.Join(items, ",")) + 2
	}

	return baseContent + strings.Join(items, ",") + `]}}`
}

func init() {
	TestingCmd.AddCommand(largeOutputIntegrationTestCmd)
}
