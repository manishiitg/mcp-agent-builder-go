package step_based_workflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// sharedPlatformInstructionPatterns are deliberately narrow. Workflow plan
// prose may describe domain APIs and business operations, but it must not copy
// AgentWorks' own transport, auth, sandbox, session, or managed-tool mechanics.
// Those contracts belong to the runtime prompt and tool schemas, where they can
// be changed once without making every saved workflow stale.
var sharedPlatformInstructionPatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{name: "MCP bridge environment variables", pattern: regexp.MustCompile(`(?i)\$(?:MCP_CUSTOM|MCP_MCP|MCP_AUTH)\b|\bMCP_API_(?:URL|TOKEN)\b`)},
	{name: "api-bridge transport", pattern: regexp.MustCompile(`(?i)\bapi[-_ ]bridge\b`)},
	{name: "Folder Guard implementation", pattern: regexp.MustCompile(`(?i)\bfolder guard\b`)},
	{name: "tool-discovery workaround", pattern: regexp.MustCompile(`(?i)\bget_api_spec\b`)},
	{name: "managed workflow database tool", pattern: regexp.MustCompile(`(?i)\b(?:query|mutate)_workflow_db\b`)},
	{name: "coding-agent session plumbing", pattern: regexp.MustCompile(`(?i)\b(?:tmux|native[-_ ]session)\b.{0,80}\b(?:coding agent|codex|claude|session|terminal)\b|\b(?:coding agent|codex|claude)\b.{0,80}\b(?:tmux|native[-_ ]session)\b`)},
}

// sharedPlatformInstructionMatches returns stable, de-duplicated ownership
// violations. It intentionally does not reject ordinary curl, SQL, API, or CLI
// instructions because those may be intrinsic to the workflow's target domain.
func sharedPlatformInstructionMatches(text string) []string {
	seen := map[string]bool{}
	var matches []string
	for _, candidate := range sharedPlatformInstructionPatterns {
		if candidate.pattern.MatchString(text) && !seen[candidate.name] {
			seen[candidate.name] = true
			matches = append(matches, candidate.name)
		}
	}
	sort.Strings(matches)
	return matches
}

func validateWorkflowArtifactInstruction(field, text string) error {
	matches := sharedPlatformInstructionMatches(text)
	if len(matches) == 0 {
		return nil
	}
	return fmt.Errorf(
		"%s contains shared AgentWorks platform mechanics (%s). Keep the workflow contract semantic: state the domain action, durable result, failure behavior, and verification outcome; do not copy bridge/auth, Folder Guard, managed-tool, or coding-session instructions into planning/plan.json",
		field,
		strings.Join(matches, ", "),
	)
}

// validateWorkflowArtifactMutationArgs checks only fields supplied by the
// mutation. This lets an unrelated edit repair one legacy field at a time
// instead of making every old workflow immediately uneditable.
func validateWorkflowArtifactMutationArgs(args map[string]interface{}) error {
	return validateWorkflowArtifactMutationValue("step", args)
}

func validateWorkflowArtifactMutationValue(path string, value interface{}) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			childPath := path + "." + key
			switch key {
			case "description", "message":
				if text, ok := child.(string); ok {
					if err := validateWorkflowArtifactInstruction(childPath, text); err != nil {
						return err
					}
				}
			case "items", "messages", "predefined_routes", "predefined_route", "new_route", "sub_agent_step", "todo_task_step":
				if err := validateWorkflowArtifactMutationValue(childPath, child); err != nil {
					return err
				}
			}
		}
	case []interface{}:
		for i, child := range typed {
			if err := validateWorkflowArtifactMutationValue(fmt.Sprintf("%s[%d]", path, i), child); err != nil {
				return err
			}
		}
	}
	return nil
}
