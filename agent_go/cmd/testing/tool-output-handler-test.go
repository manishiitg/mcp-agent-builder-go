package testing

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/utils"
	mcpagent "github.com/manishiitg/mcpagent/agent"
)

var toolOutputHandlerTestCmd = &cobra.Command{
	Use:   "tool-output-handler",
	Short: "Test tool output handler functionality",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get logging configuration from viper
		logFile := viper.GetString("log-file")
		logLevel := viper.GetString("log-level")

		// Initialize test logger
		InitTestLogger(logFile, logLevel)
		logger := GetTestLogger()

		logger.Info("=== Tool Output Handler Test ===")

		// Create test directory
		testDir := "tool_output_test"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			return fmt.Errorf("failed to create test directory: %w", err)
		}
		defer os.RemoveAll(testDir)

		logger.Info(fmt.Sprintf("Created test directory: %s", testDir))

		// Test the extractActualContent function with MCP format
		logger.Info("\n--- Test 1: Content Extraction ---")
		if err := testContentExtraction(); err != nil {
			return fmt.Errorf("content extraction test failed: %w", err)
		}

		// Test file creation with MCP format
		logger.Info("\n--- Test 2: File Creation with MCP Format ---")
		if err := testFileCreationWithMCPFormat(testDir); err != nil {
			return fmt.Errorf("file creation test failed: %w", err)
		}

		// Test large output virtual tools
		logger.Info("\n--- Test 3: Large Output Virtual Tools ---")
		if err := testLargeOutputVirtualTools(testDir); err != nil {
			return fmt.Errorf("large output virtual tools test failed: %w", err)
		}

		logger.Info("\n✅ All tool output handler tests passed!")
		return nil
	},
}

func testContentExtraction() error {
	logger := GetTestLogger()

	// Test cases for MCP format
	testCases := []struct {
		input    string
		expected string
		name     string
	}{
		{
			input:    `{"type":"text","text":"{\"key\": \"value\"}"}`,
			expected: `{"key": "value"}`,
			name:     "MCP JSON format",
		},
		{
			input:    `{"type":"text","text":"Hello World"}`,
			expected: "Hello World",
			name:     "MCP text format",
		},
		{
			input:    `{"type":"text","text":"{\"name\":\"test\",\"items\":[\"a\",\"b\"]}"}`,
			expected: `{"name":"test","items":["a","b"]}`,
			name:     "MCP complex JSON format",
		},
		{
			input:    "TOOL RESULT for aws_cli_query: {\"key\": \"value\"}",
			expected: "{\"key\": \"value\"}",
			name:     "old format",
		},
		{
			input:    "{\"key\": \"value\"}",
			expected: "{\"key\": \"value\"}",
			name:     "plain JSON",
		},
	}

	for _, tc := range testCases {
		result := utils.ExtractActualContent(tc.input)
		if result != tc.expected {
			return fmt.Errorf("content extraction test '%s' failed: expected '%s', got '%s'",
				tc.name, tc.expected, result)
		}
		logger.Info(fmt.Sprintf("✅ Content extraction test '%s' passed", tc.name))
	}

	return nil
}

func testFileCreationWithMCPFormat(testDir string) error {
	logger := GetTestLogger()

	// Create tool output handler
	handler := mcpagent.NewToolOutputHandlerWithConfig(
		100, // Small threshold for testing
		testDir,
		"test-session-123",
		true,
		true,
	)

	// Test with MCP format content
	mcpContent := `{"type":"text","text":"{\"name\":\"test\",\"value\":123,\"items\":[\"a\",\"b\",\"c\"]}"}`
	toolName := "test_tool"

	// Write to file
	filePath, err := handler.WriteToolOutputToFile(mcpContent, toolName)
	if err != nil {
		return fmt.Errorf("failed to write tool output to file: %w", err)
	}

	logger.Info(fmt.Sprintf("File written to: %s", filePath))

	// Verify file exists and has content
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file was not created: %s", filePath)
	}

	// Read file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	logger.Info(fmt.Sprintf("File content length: %d", len(fileContent)))
	logger.Info(fmt.Sprintf("File content: %s", string(fileContent)))

	// Verify the content was extracted correctly (should not contain MCP wrapper)
	if strings.Contains(string(fileContent), `"type":"text"`) {
		return fmt.Errorf("file still contains MCP wrapper")
	}

	// Verify it's valid JSON
	if !isValidJSON(string(fileContent)) {
		return fmt.Errorf("file content is not valid JSON")
	}

	logger.Info(fmt.Sprintf("✅ File creation test passed - content extracted correctly"))

	return nil
}

func isValidJSON(content string) bool {
	var js interface{}
	return json.Unmarshal([]byte(content), &js) == nil
}

func testLargeOutputVirtualTools(testDir string) error {
	logger := GetTestLogger()

	logger.Info("Testing large output handler configuration...")
	handler := mcpagent.NewToolOutputHandlerWithConfig(16, testDir, "test-session", true, true)
	content := strings.Repeat("large-output ", 100)
	if !handler.IsLargeToolOutput(content) {
		return fmt.Errorf("expected configured handler to offload large content")
	}
	path, err := handler.WriteToolOutputToFile(content, "test_tool")
	if err != nil {
		return fmt.Errorf("write tool output: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat offloaded output: %w", err)
	}
	handler.SetEnabled(false)
	if handler.IsLargeToolOutput(content) {
		return fmt.Errorf("disabled handler still classified output for offloading")
	}

	logger.Info("✅ Large output virtual tools tests passed!")
	return nil
}

func init() {
	TestingCmd.AddCommand(toolOutputHandlerTestCmd)
}
