package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"planner/models"
	"planner/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// SearchDocuments handles GET /api/search
func SearchDocuments(c *gin.Context) {
	var req models.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Invalid query parameters",
			Error:   err.Error(),
		})
		return
	}

	docsDir := viper.GetString("docs-dir")
	query := req.Query
	folder := req.Folder

	// Build search path
	var searchPath string
	if folder != "" {
		searchPath = filepath.Join(docsDir, folder)

		// Validate that the folder exists and is within docs directory
		if !utils.IsValidFilePath(searchPath, docsDir) {
			c.JSON(http.StatusBadRequest, models.APIResponse{
				Success: false,
				Message: "Invalid folder path",
				Error:   "Folder path contains invalid characters or attempts directory traversal",
			})
			return
		}

		// Check if folder exists
		if _, err := os.Stat(searchPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Message: "Folder not found",
				Error:   "The specified folder does not exist: " + folder,
			})
			return
		}
	} else {
		searchPath = docsDir
	}

	// Check if ripgrep is available
	if !isRipgrepAvailable() {
		// Fallback to basic regex search
		results, err := basicRegexSearch(searchPath, query, req.Limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Message: "Failed to search documents",
				Error:   err.Error(),
			})
			return
		}

		responseData := map[string]interface{}{
			"query":   req.Query,
			"results": results,
			"total":   len(results),
			"method":  "basic_regex",
		}

		c.JSON(http.StatusOK, models.APIResponse{
			Success: true,
			Message: "Search completed successfully (basic regex mode)",
			Data:    responseData,
		})
		return
	}

	// Use ripgrep for regex search
	results, err := ripgrepRegexSearch(searchPath, query, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Failed to search documents with ripgrep",
			Error:   err.Error(),
		})
		return
	}

	responseData := map[string]interface{}{
		"query":   req.Query,
		"results": results,
		"total":   len(results),
		"method":  "ripgrep_regex",
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Search completed successfully",
		Data:    responseData,
	})
}

// GetNestedContent function removed - no longer needed

// getContentPreview returns a preview of content around the search query
func getContentPreview(content, query string) string {
	queryLower := strings.ToLower(query)
	contentLower := strings.ToLower(content)

	index := strings.Index(contentLower, queryLower)
	if index == -1 {
		// Return first 200 characters if no match found
		if len(content) > 200 {
			return content[:200] + "..."
		}
		return content
	}

	// Get context around the match
	start := index - 100
	if start < 0 {
		start = 0
	}

	end := index + len(query) + 100
	if end > len(content) {
		end = len(content)
	}

	preview := content[start:end]

	// Add ellipsis if we're not at the beginning/end
	if start > 0 {
		preview = "..." + preview
	}
	if end < len(content) {
		preview = preview + "..."
	}

	return preview
}

// findNestedContent finds content by following a nested path through headings
func findNestedContent(content string, pathParts []string) (string, error) {
	lines := strings.Split(content, "\n")

	currentLevel := 0
	foundContent := []string{}
	inTargetSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a heading
		if strings.HasPrefix(trimmed, "#") {
			// Extract heading text
			headingText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))

			// Check if this matches our current path part
			if currentLevel < len(pathParts) {
				if strings.Contains(strings.ToLower(headingText), strings.ToLower(pathParts[currentLevel])) {
					currentLevel++

					if currentLevel == len(pathParts) {
						// We found our target section
						inTargetSection = true
						continue
					}
				} else if currentLevel > 0 {
					// We're in a section but this heading doesn't match
					// Check if it's a subheading of our current level
					headingLevel := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
					if headingLevel <= currentLevel {
						// This is a sibling or parent heading, reset
						currentLevel = 0
						inTargetSection = false
					}
				}
			}
		}

		// If we're in the target section, collect content
		if inTargetSection {
			// Stop if we hit another heading at our level or higher
			if strings.HasPrefix(trimmed, "#") {
				headingLevel := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
				if headingLevel <= len(pathParts) {
					break
				}
			}

			foundContent = append(foundContent, line)
		}
	}

	if len(foundContent) == 0 {
		return "", fmt.Errorf("content not found for path: %s", strings.Join(pathParts, " -> "))
	}

	return strings.Join(foundContent, "\n"), nil
}

// isRipgrepAvailable checks if ripgrep is available on the system
func isRipgrepAvailable() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

// ripgrepRegexSearch performs regex search using ripgrep
func ripgrepRegexSearch(docsDir, query string, limit int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	// Build ripgrep command for regex search
	args := []string{
		"--json",                                // JSON output
		"--ignore-case",                         // Case insensitive
		"--max-count", fmt.Sprintf("%d", limit), // Limit results per file
		"--with-filename", // Include filename
		"--line-number",   // Include line numbers
		"--column",        // Include column numbers
		query,             // Regex query (positional argument)
		docsDir,           // Search directory
	}

	// Execute ripgrep
	cmd := exec.Command("rg", args...)
	output, err := cmd.CombinedOutput() // Capture both stdout and stderr for better error messages
	if err != nil {
		// If no matches found, ripgrep returns exit code 1
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return results, nil // No matches found
		}
		return nil, fmt.Errorf("ripgrep failed: %v (output: %s)", err, string(output))
	}

	// Parse JSON output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var rgResult struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
				Submatches []struct {
					Match struct {
						Text string `json:"text"`
					} `json:"match"`
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(line), &rgResult); err != nil {
			continue // Skip invalid JSON lines
		}

		if rgResult.Type != "match" {
			continue
		}

		// Extract file information
		filePath := rgResult.Data.Path.Text
		relPath, _ := filepath.Rel(docsDir, filePath)
		docID := strings.TrimSuffix(filepath.Base(filePath), ".md")

		// Determine folder
		folder := ""
		if dir := filepath.Dir(relPath); dir != "." {
			folder = dir
		}

		// Get file info
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Calculate score based on regex matches
		score := calculateScore(rgResult.Data.Lines.Text, query, rgResult.Data.Submatches)

		// Get content preview
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		result := map[string]interface{}{
			"id":              docID,
			"title":           docID,
			"filepath":        filePath,
			"folder":          folder,
			"matches":         []string{fmt.Sprintf("line %d: %s", rgResult.Data.LineNumber, rgResult.Data.Lines.Text)},
			"score":           score,
			"last_modified":   fileInfo.ModTime(),
			"content_preview": getContentPreview(string(content), query),
			"line_number":     rgResult.Data.LineNumber,
			"matched_text":    rgResult.Data.Lines.Text,
		}

		results = append(results, result)
	}

	// Sort by score (highest first)
	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			if results[j]["score"].(int) < results[j+1]["score"].(int) {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}

	return results, nil
}

// basicRegexSearch performs basic regex search as fallback
func basicRegexSearch(docsDir, query string, limit int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	// Compile regex pattern
	regex, err := regexp.Compile("(?i)" + query) // Case-insensitive
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %v", err)
	}

	err = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-text files
		if info.IsDir() {
			return nil
		}

		// Check if it's a text-based file
		if !isTextBasedFile(info.Name(), "") {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		contentStr := string(content)
		relPath, _ := filepath.Rel(docsDir, path)

		// Determine folder
		folder := ""
		if dir := filepath.Dir(relPath); dir != "." {
			folder = dir
		}

		// Search for regex matches in content
		matches := regex.FindAllString(contentStr, -1)
		if len(matches) > 0 {
			// Get file info
			fileInfo, err := os.Stat(path)
			if err != nil {
				return err
			}

			// Calculate score based on number of matches
			score := len(matches)

			result := map[string]interface{}{
				"filepath":        relPath,
				"folder":          folder,
				"matches":         matches,
				"score":           score,
				"last_modified":   fileInfo.ModTime(),
				"content_preview": getContentPreview(contentStr, query),
				"line_number":     1, // Basic search doesn't provide line numbers
				"matched_text":    strings.Join(matches, ", "),
			}

			results = append(results, result)
		}

		return nil
	})

	return results, err
}

// calculateScore calculates a relevance score for search results
func calculateScore(lineText, query string, submatches []struct {
	Match struct {
		Text string `json:"text"`
	} `json:"match"`
	Start int `json:"start"`
	End   int `json:"end"`
}) int {
	score := 1
	lineLower := strings.ToLower(lineText)
	queryLower := strings.ToLower(query)

	// Base score for match
	score += len(submatches)

	// Bonus for exact match
	if strings.Contains(lineLower, queryLower) {
		score += 2
	}

	// Bonus for title/heading matches
	if strings.HasPrefix(strings.TrimSpace(lineText), "#") {
		score += 5
	}

	// Bonus for beginning of line
	for _, submatch := range submatches {
		if submatch.Start < 10 { // Within first 10 characters
			score += 2
		}
	}

	return score
}
