package handlers

import (
	"fmt"
	"strings"
	"testing"
)

func TestApplyDiffPatchFlexible(t *testing.T) {
	tests := []struct {
		name           string
		currentContent string
		diffContent    string
		expectedError  bool
		expectedResult string
	}{
		{
			name: "Traditional unified diff",
			currentContent: `# Todo List

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
+**Patch Test**: This was added via unified diff.

 ## Objective
`,
			expectedError: false,
			expectedResult: `# Todo List
**Patch Test**: This was added via unified diff.

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research`,
		},
		{
			name:           "Line ending normalization test (CRLF to LF)",
			currentContent: "# Todo List\r\n\r\n## Objective\r\n- Complete project analysis",
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
+**CRLF Test**: Added with normalized line endings

 ## Objective
`,
			expectedError: false,
			expectedResult: `# Todo List
**CRLF Test**: Added with normalized line endings

## Objective
- Complete project analysis`,
		},
		{
			name: "Diff without newline ending is normalized",
			currentContent: `# Todo List

## Objective`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,2 +1,3 @@
 # Todo List
+**No Newline**: This should be normalized`,
			expectedError: false,
			expectedResult: `# Todo List
**No Newline**: This should be normalized

## Objective`,
		},
		{
			name: "Diff with proper newline ending",
			currentContent: `# Todo List

## Objective`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,2 +1,3 @@
 # Todo List
+**With Newline**: This should work

## Objective
`,
			expectedError: false,
			expectedResult: `# Todo List
**With Newline**: This should work

## Objective`,
		},
		{
			name: "Missing diff headers (should fail validation)",
			currentContent: `# Todo List

## Objective`,
			diffContent: `@@ -1,2 +1,3 @@
 # Todo List
+**No Headers**: This should fail

## Objective
`,
			expectedError: true, // Should fail validation due to missing headers
		},
		{
			name: "Context mismatch (should fail patch)",
			currentContent: `# Todo List

## Objective
- Complete project analysis`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
+**Context Mismatch**: This should fail

## Different Content
`,
			expectedError: true, // Should fail due to context mismatch
		},
		{
			name: "Simplified diff format with exact context",
			currentContent: `# Todo List

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ ... @@
 # Todo List
+**Simplified Patch**: This was added via simplified diff.

 ## Objective
`,
			expectedError: false,
			expectedResult: `# Todo List
**Simplified Patch**: This was added via simplified diff.

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research`,
		},
		{
			name: "Simplified addition with unique full-file context",
			currentContent: `# Todo List

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ ... @@
 # Todo List
+**Single Addition**: This is a test.

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research
`,
			expectedError: false,
			expectedResult: `# Todo List
**Single Addition**: This is a test.

## Objective
- Complete project analysis
- Generate comprehensive report

## Notes
- Leverages tavily-search for comprehensive research`,
		},
		{
			name: "Empty diff (should fail validation)",
			currentContent: `# Todo List

## Objective
- Complete project analysis`,
			diffContent:   "",
			expectedError: true, // Empty diff should fail validation
		},
		{
			name: "Malformed diff",
			currentContent: `# Todo List

## Objective`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,2 +1,3 @@
 # Todo List
+**Malformed**: This should fail
## Objective
`,
			expectedError: true,
		},
		{
			name: "Ambiguous non-contiguous bullet context is rejected",
			currentContent: `# Todo List

## Objective
- Complete project analysis`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
- Complete project analysis
+- Test patch: Added via AI agent
`,
			expectedError: true,
		},
		{
			name: "Duplicate exact fallback context is rejected",
			currentContent: `## Section
same value

## Section
same value`,
			diffContent: `--- a/todo.md
+++ b/todo.md
@@ -100,2 +100,3 @@
 ## Section
 same value
+new value
`,
			expectedError: true,
		},
		{
			name: "Real agent-generated diff pattern (should be auto-corrected)",
			currentContent: `## Notes
- Each todo builds on previous research to create comprehensive analysis
- Success criteria are measurable and tied to specific deliverables
- Dependencies ensure logical progression of analysis depth`,
			diffContent: `--- a/Tasks/Workflow-Testing/todo.md
+++ b/Tasks/Workflow-Testing/todo.md
@@ -200,3 +200,4 @@ - Each todo builds on previous research to create comprehensive analysis
 - Success criteria are measurable and tied to specific deliverables
 - Dependencies ensure logical progression of analysis depth
+- Test patch: Added via diff tool
`,
			expectedError: false, // Should be auto-corrected and succeed
			expectedResult: `## Notes
- Each todo builds on previous research to create comprehensive analysis
- Success criteria are measurable and tied to specific deliverables
- Dependencies ensure logical progression of analysis depth
- Test patch: Added via diff tool`,
		},
		{
			name: "Ambiguous invalid line references are rejected",
			currentContent: `## Notes
- Each todo builds on previous research to create comprehensive analysis
- Success criteria are measurable and tied to specific deliverables
- Dependencies ensure logical progression of analysis depth
-Updated for testing.`,
			diffContent: `--- a/Tasks/Workflow-Testing/todo.md
+++ b/Tasks/Workflow-Testing/todo.md
@@ -last,2 +last-1,1 @@
 - Dependencies ensure logical progression of analysis depth
-Updated for testing.`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyDiffPatchFlexible(tt.currentContent, tt.diffContent)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectedResult != "" && result != tt.expectedResult {
				t.Errorf("Result mismatch.\nExpected:\n%s\n\nGot:\n%s", tt.expectedResult, result)
			}
		})
	}
}

// TestNormalizeLineEndings tests the line ending normalization function
func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "LF endings (no change)",
			input:    "line1\nline2\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "CRLF endings",
			input:    "line1\r\nline2\r\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "CR endings",
			input:    "line1\rline2\rline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "Mixed endings",
			input:    "line1\nline2\r\nline3\r",
			expected: "line1\nline2\nline3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeLineEndings(tt.input)
			if result != tt.expected {
				t.Errorf("Expected: %q, Got: %q", tt.expected, result)
			}
		})
	}
}

// TestValidateDiffFormat tests the diff format validation function
func TestValidateDiffFormat(t *testing.T) {
	tests := []struct {
		name        string
		diffContent string
		expectError bool
	}{
		{
			name: "Valid diff format",
			diffContent: `--- a/file.md
+++ b/file.md
@@ -1,3 +1,4 @@
 # Header
+New line
` + " \n" + ` ## Section
`,
			expectError: false,
		},
		{
			name:        "Empty diff",
			diffContent: "",
			expectError: true,
		},
		{
			name: "Missing headers",
			diffContent: `@@ -1,2 +1,3 @@
 # Header
+New line`,
			expectError: true,
		},
		{
			name: "Missing hunk headers",
			diffContent: `--- a/file.md
+++ b/file.md
 # Header
+New line`,
			expectError: true,
		},
		{
			name: "Diff without newline ending",
			diffContent: `--- a/file.md
+++ b/file.md
@@ -1,2 +1,3 @@
 # Header
+New line`,
			expectError: true,
		},
		{
			name: "Too short diff",
			diffContent: `--- a/file.md
+++ b/file.md`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDiffFormat(tt.diffContent)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestCorrectAgentGeneratedDiff tests the automatic correction of agent-generated diffs
func TestCorrectAgentGeneratedDiff(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Correct contiguous bullet context with an explicit anchor",
			input: `--- a/todo.md
+++ b/todo.md
@@ -3,2 +3,3 @@
 ## Objective
- Complete project analysis
+- Test patch: Added via AI agent
`,
			expected: `--- a/todo.md
+++ b/todo.md
@@ -3,2 +3,3 @@
 ## Objective
 - Complete project analysis
+- Test patch: Added via AI agent
`,
		},
		{
			name: "Do not correct non-contiguous bullet context",
			input: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
- Complete project analysis
+- Test patch: Added via AI agent
`,
			expected: `--- a/todo.md
+++ b/todo.md
@@ -1,2 +1,2 @@
 # Todo List
- Complete project analysis
+- Test patch: Added via AI agent
`,
		},
		{
			name: "No changes needed for correct diff",
			input: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
+**New addition**: Added via unified diff.
` + " \n" + ` ## Objective
`,
			expected: `--- a/todo.md
+++ b/todo.md
@@ -1,3 +1,4 @@
 # Todo List
+**New addition**: Added via unified diff.
` + " \n" + ` ## Objective
`,
		},
		{
			name: "Multiple contiguous bullet context corrections",
			input: `--- a/todo.md
+++ b/todo.md
@@ -3,3 +3,4 @@
 ## Objective
- Complete project analysis
- Generate comprehensive report
+- Test patch: Added via AI agent
`,
			expected: `--- a/todo.md
+++ b/todo.md
@@ -3,3 +3,4 @@
 ## Objective
 - Complete project analysis
 - Generate comprehensive report
+- Test patch: Added via AI agent
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a simple current content for testing
			currentContent := `# Todo List

## Objective
- Complete project analysis
- Generate comprehensive report`

			result := correctAgentGeneratedDiff(tt.input, currentContent)
			if result != tt.expected {
				t.Errorf("Expected:\n%s\n\nGot:\n%s", tt.expected, result)
			}
		})
	}
}

// Tests for removed functions have been removed since we're using the simplified approach

// PLAT-023: on 2026-08-04, a stale-context patch failure against a 150KB file
// cost 4 extra tool calls to recover — one to learn the match failed, more to
// re-read the file and locate the real content by hand. The fallback matcher
// already scans every candidate position computing mismatches; these tests
// prove the closest position's real content now travels with the failure
// instead of being discarded, and that it is actually enough to fix the diff
// in one corrected retry — the acceptance boundary, not just "the failure is
// well-formed."
func largeFixtureFileContent(lines int) string {
	rows := make([]string, lines)
	for i := 0; i < lines; i++ {
		rows[i] = fmt.Sprintf("line-%05d: some representative content for row %d", i, i)
	}
	return strings.Join(rows, "\n")
}

func TestApplyAgentGeneratedDiffFallbackReportsBoundedContextOnLargeFileMismatch(t *testing.T) {
	const totalLines = 3000
	const targetLine = 2100 // deep enough into a large file that "just re-read it" is expensive

	rows := strings.Split(largeFixtureFileContent(totalLines), "\n")
	// The file's real content has drifted since the agent last read it: the
	// target line changed; its neighbors did not. A single-line hunk cannot
	// distinguish "close" from "anywhere else" — every non-matching position
	// ties at exactly one mismatch — so the hunk carries two lines of real
	// surrounding context on each side. Only the window anchored at
	// targetLine-2 can match that surrounding context at all; everywhere else
	// in the file, all five lines differ (the fixture's content is unique per
	// row), which is the actual signal a "closest match" ranking needs.
	original := rows[targetLine]
	current := strings.Replace(original, fmt.Sprintf("row %d", targetLine), fmt.Sprintf("row %d v2", targetLine), 1)
	rows[targetLine] = current
	currentContent := strings.Join(rows, "\n")

	hunkWithStaleTarget := func(target string) string {
		return strings.Join([]string{rows[targetLine-2], rows[targetLine-1], target, rows[targetLine+1], rows[targetLine+2]}, "\n ")
	}
	staleDiff := fmt.Sprintf("--- a/large.txt\n+++ b/large.txt\n@@ -1,5 +1,5 @@\n %s\n+replaced\n", hunkWithStaleTarget(original))

	_, err := applyAgentGeneratedDiffFallback(currentContent, staleDiff)
	if err == nil {
		t.Fatal("stale context was accepted instead of rejected — this must never silently apply an ambiguous hunk")
	}

	msg := err.Error()
	if !strings.Contains(msg, "Closest match: 1 of 5") {
		t.Fatalf("hint does not report the true per-candidate mismatch count (must not be capped at the zero-tolerance threshold): %v", err)
	}
	if !strings.Contains(msg, current) {
		t.Fatalf("hint does not contain the file's actual current content at the near-miss position:\n%s", msg)
	}
	if !strings.Contains(msg, fmt.Sprintf("file line %d", targetLine-1)) {
		t.Fatalf("hint does not name the correct 1-based file line (%d):\n%s", targetLine-1, msg)
	}
	// Bounded: this file is 3000 lines; the hint must not become a second copy
	// of it.
	if len(msg) > 4000 {
		t.Fatalf("hint is not bounded: %d bytes", len(msg))
	}

	// The actual acceptance boundary: the hint must contain enough to fix the
	// diff in one corrected retry, not just explain the failure.
	correctedDiff := fmt.Sprintf("--- a/large.txt\n+++ b/large.txt\n@@ -1,5 +1,5 @@\n %s\n+replaced\n", hunkWithStaleTarget(current))
	result, err := applyAgentGeneratedDiffFallback(currentContent, correctedDiff)
	if err != nil {
		t.Fatalf("corrected diff built from the hint's own reported content still failed: %v", err)
	}
	if !strings.Contains(result, "replaced") {
		t.Fatalf("corrected retry did not apply the intended change")
	}
}

func TestApplyAgentGeneratedDiffFallbackHintStaysBoundedOnAPathologicallyLongLine(t *testing.T) {
	rows := strings.Split(largeFixtureFileContent(50), "\n")
	rows[10] = strings.Repeat("x", 10000) // one absurdly long current line
	currentContent := strings.Join(rows, "\n")

	staleDiff := "--- a/f.txt\n+++ b/f.txt\n@@ -1,1 +1,1 @@\n-this line does not exist anywhere in the file\n+replacement\n"

	_, err := applyAgentGeneratedDiffFallback(currentContent, staleDiff)
	if err == nil {
		t.Fatal("expected a mismatch error")
	}
	if len(err.Error()) > 4000 {
		t.Fatalf("one long line blew up the hint instead of being truncated: %d bytes", len(err.Error()))
	}
}

func TestApplyAgentGeneratedDiffFallbackDoesNotRecommendAnArbitraryTiedNearMatch(t *testing.T) {
	currentContent := strings.Join([]string{
		"first section",
		"shared before",
		"current value A",
		"shared after",
		"separator",
		"second section",
		"shared before",
		"current value B",
		"shared after",
	}, "\n")
	staleDiff := strings.Join([]string{
		"--- a/f.txt",
		"+++ b/f.txt",
		"@@ -1,3 +1,3 @@",
		" shared before",
		"-stale value",
		"+replacement",
		" shared after",
		"",
	}, "\n")

	_, err := applyAgentGeneratedDiffFallback(currentContent, staleDiff)
	if err == nil {
		t.Fatal("expected a mismatch error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "2 locations are equally close") {
		t.Fatalf("tied nearest matches were not reported as ambiguous: %s", msg)
	}
	if strings.Contains(msg, "Current content there:") {
		t.Fatalf("an arbitrary tied location was presented as the closest retry target: %s", msg)
	}
}

// TestCountContentLines locks down the two edge cases plain
// len(strings.Split(content, "\n")) gets wrong: an empty file is 0 lines
// (not 1), and a trailing newline must not count as an extra blank line.
// Getting this wrong made verifyDiffApplied reject a legitimate single-line
// file-creation diff as corrupted (see TestDiffPatchCreationWithControllingTTY).
func TestCountContentLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty file", "", 0},
		{"one line, no trailing newline", "hello", 1},
		{"one line, with trailing newline", "hello\n", 1},
		{"three lines, with trailing newline", "a\nb\nc\n", 3},
		{"three lines, no trailing newline", "a\nb\nc", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countContentLines(tt.content); got != tt.want {
				t.Errorf("countContentLines(%q) = %d, want %d", tt.content, got, tt.want)
			}
		})
	}
}

// TestVerifyDiffAppliedCatchesSilentPartialApply reproduces the shape of the
// confida-login live finding: a two-hunk patch where the first hunk (net +1
// line) applied correctly, but the second hunk's own changes were silently
// dropped and an unrelated trailing line was deleted too (net -1) — so the
// file's real net change (0) does not match what the diff's own +/- lines
// claim (+1). The specific internal mechanism that produced this isn't
// reproducible on demand (see PLAT-19x); this test proves the safety net
// catches the failure SHAPE regardless of which internal path caused it.
func TestVerifyDiffAppliedCatchesSilentPartialApply(t *testing.T) {
	original := "line1\nline2\nline3\n"
	diff := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,4 @@\n line1\n+added-line\n line2\n line3\n"
	// Simulates the corrupted result: the addition landed, but an unrelated
	// trailing line was silently dropped, so the net change is 0, not +1.
	corruptedResult := "line1\nadded-line\nline2\n"

	err := verifyDiffApplied(original, diff, corruptedResult)
	if err == nil {
		t.Fatal("expected verifyDiffApplied to reject a result whose line-count change does not match the diff's claim")
	}
	if !strings.Contains(err.Error(), "unexpected line-count change") {
		t.Errorf("error should name the mismatch, got: %v", err)
	}
}

// TestVerifyDiffAppliedAcceptsACorrectApply guards against false positives:
// a normal, correctly-applied multi-hunk patch must pass verification.
func TestVerifyDiffAppliedAcceptsACorrectApply(t *testing.T) {
	original := "line1\nline2\nline3\nline4\nline5\n"
	diff := "--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,3 @@\n line1\n+added-line\n line2\n@@ -4,2 +5,2 @@\n-line4\n+changed-line4\n line5\n"
	correctResult := "line1\nadded-line\nline2\nline3\nchanged-line4\nline5\n"

	if err := verifyDiffApplied(original, diff, correctResult); err != nil {
		t.Fatalf("verifyDiffApplied rejected a correctly-applied patch: %v", err)
	}
}

// TestDiffClaimedLineDeltaCountsBodyLinesNotHeaders proves the delta is
// computed from the actual +/- body lines, not the (sometimes wrong,
// LLM-supplied) @@ header counts — the same signal correctAgentGeneratedDiff
// already trusts over the header.
func TestDiffClaimedLineDeltaCountsBodyLinesNotHeaders(t *testing.T) {
	// Header claims +1,+1 (net 0) but the body has one addition, no removals.
	diff := "--- a/f.txt\n+++ b/f.txt\n@@ -1,1 +1,1 @@\n line1\n+added-line\n"
	if got := diffClaimedLineDelta(diff); got != 1 {
		t.Fatalf("diffClaimedLineDelta = %d, want 1 (from the body's +/- lines, ignoring the header)", got)
	}
}

func TestApplyAgentGeneratedDiffFallbackOmitsHintWhenFileIsShorterThanTheHunk(t *testing.T) {
	currentContent := "only one line"
	diffContent := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n line a\n line b\n line c\n"

	_, err := applyAgentGeneratedDiffFallback(currentContent, diffContent)
	if err == nil {
		t.Fatal("expected an error when the file is shorter than the hunk")
	}
	if strings.Contains(err.Error(), "Closest match:") {
		t.Fatalf("hint claims a closest match that cannot exist: %v", err)
	}
}
