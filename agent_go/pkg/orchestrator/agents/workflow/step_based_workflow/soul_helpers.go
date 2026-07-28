package step_based_workflow

import (
	"context"
	"strings"
)

// Soul.md is the canonical source of truth for the workflow's objective and
// success criteria. The builder writes it directly via shell (no dedicated tool)
// and runtime consumers — server template vars, orchestrator hcpo.SetObjective,
// learning prompt injection — read from here, not from plan.json.
//
// Required structural convention:
//
//	# <Workflow display name>
//
//	## Objective
//	<one-paragraph statement of what the workflow is for>
//
//	## Success Criteria
//	<bullet list or paragraph describing when the workflow is "done right">
//
//	## Constraints  (optional — only explicit user-approved boundaries)
//	## Notifications  (optional — user preference for Pulse notifications)
//
// Architecture, implementation choices, agent-inferred assumptions, historical
// decisions, and references do not belong in soul.md. They are revisable and
// should live in planning/plan.json, step descriptions, changelog, learnings, or
// knowledgebase as appropriate. Extra H2 sections are allowed and ignored by the
// extractor. Section order is not significant, but `## Objective` and
// `## Success Criteria` MUST exist for the workflow to be considered ready to
// optimize.

const (
	soulObjectiveSection       = "Objective"
	soulSuccessCriteriaSection = "Success Criteria"
	// soulConstraintsSection holds explicit owner-approved boundaries (risk limits,
	// capacity caps, policy rules). Extracted and injected into agent prompts as
	// BINDING context — see ResolveWorkflowConstraints. Steps have read access to
	// soul/ but never write access, so a constraint has exactly one author (the
	// builder) and many readers. Restating a constraint value in a step description
	// or a learnings file creates a copy that drifts; reference this section instead.
	soulConstraintsSection      = "Constraints"
	soulDefaultScaffoldTemplate = `# %s

## Objective
<TODO: one-paragraph statement of what this workflow is for.>

## Success Criteria
<TODO: bullet list or paragraph describing when the workflow is "done right".>
`
)

// ReadWorkflowObjectiveFromSoul loads soul/soul.md and extracts the Objective and
// Success Criteria sections. Missing file or missing sections return empty strings
// (not errors) so callers can treat "not yet written" as a valid intermediate state.
// A true I/O failure (network, permission) returns the error.
func ReadWorkflowObjectiveFromSoul(
	ctx context.Context,
	workspacePath string,
	readFile func(context.Context, string) (string, error),
) (objective, successCriteria string, err error) {
	objective, successCriteria, _, err = ReadWorkflowSoulSections(ctx, workspacePath, readFile)
	return objective, successCriteria, err
}

// ReadWorkflowSoulSections loads soul/soul.md once and extracts Objective,
// Success Criteria, and Constraints together. Callers that need more than one
// section should use this rather than calling the single-section helpers twice —
// soul.md is fetched over the workspace API, so each call is a round trip.
//
// Same missing-file semantics as ReadWorkflowObjectiveFromSoul: absent file or
// absent section yields empty strings, not an error.
func ReadWorkflowSoulSections(
	ctx context.Context,
	workspacePath string,
	readFile func(context.Context, string) (string, error),
) (objective, successCriteria, constraints string, err error) {
	path := normalizePathForWorkspaceAPI(SoulFolderName+"/"+SoulFileName, workspacePath)
	content, readErr := readFile(ctx, path)
	if readErr != nil {
		// "file not found" is expected for workflows that haven't scaffolded soul.md yet.
		lower := strings.ToLower(readErr.Error())
		if strings.Contains(lower, "not found") || strings.Contains(lower, "no such file") {
			return "", "", "", nil
		}
		return "", "", "", readErr
	}
	return extractSoulSection(content, soulObjectiveSection),
		extractSoulSection(content, soulSuccessCriteriaSection),
		extractSoulSection(content, soulConstraintsSection),
		nil
}

// extractSoulSection returns the body of `## <heading>` until the next H2 or EOF.
// Heading match is case-insensitive on trimmed content. Returns empty string if
// the heading is absent. Any leading/trailing blank lines in the body are trimmed.
// Nested H3+ headings within the section are preserved verbatim in the body.
func extractSoulSection(markdown, heading string) string {
	if markdown == "" {
		return ""
	}
	target := strings.ToLower(strings.TrimSpace(heading))
	var collected []string
	inSection := false
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		// H2 detection: `## Foo` but NOT `### Foo`
		isH2 := strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ")
		if isH2 {
			if inSection {
				// Next H2 ends the section we're collecting.
				break
			}
			title := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "##")))
			if title == target {
				inSection = true
			}
			continue
		}
		if inSection {
			collected = append(collected, line)
		}
	}
	return strings.TrimSpace(strings.Join(collected, "\n"))
}

// SoulScaffold returns a default soul.md body for a brand-new workflow with the
// given display label. Used by workflow_creator_tool to seed the file so the
// builder has an obvious structure to fill in.
func SoulScaffold(workflowLabel string) string {
	label := strings.TrimSpace(workflowLabel)
	if label == "" {
		label = "Workflow"
	}
	return stringsReplace(soulDefaultScaffoldTemplate, "%s", label, 1)
}

// stringsReplace wraps strings.Replace with a bounded count — kept as a tiny
// helper so the template above can use %s-style substitution without pulling in
// fmt (which formats differently for markdown-bearing templates).
func stringsReplace(s, old, new string, n int) string {
	return strings.Replace(s, old, new, n)
}

// ResolveWorkflowObjective is the canonical accessor for the workflow's objective
// and success criteria. All runtime consumers — server template vars, learning
// prompt injection, readiness checks — MUST go through this rather than reading
// plan.Objective / plan.SuccessCriteria directly.
//
// soul/soul.md `## Objective` and `## Success Criteria` are the SINGLE source of
// truth. The old workflow.json root `objective` / `success_criteria` fallback was
// removed — there is one place now.
//
// Scaffold placeholders (`<TODO: ...>`) are treated as empty so a freshly
// created soul.md doesn't leak literal TODO text into prompts.
func (hcpo *StepBasedWorkflowOrchestrator) ResolveWorkflowObjective(ctx context.Context) (objective, successCriteria string) {
	soulObj, soulSC, err := ReadWorkflowObjectiveFromSoul(ctx, hcpo.GetWorkspacePath(), hcpo.ReadWorkspaceFile)
	if err == nil {
		objective = stripSoulTodoPlaceholder(soulObj)
		successCriteria = stripSoulTodoPlaceholder(soulSC)
	}
	return objective, successCriteria
}

// ResolveWorkflowConstraints is the canonical accessor for the workflow's
// owner-approved constraints (`## Constraints` in soul/soul.md).
//
// Why this exists: constraint VALUES (risk limits, capacity caps, policy rules)
// were previously restated as literals in step descriptions, learnings files, and
// step code, and the copies drifted from the owner's decision. soul.md is the
// single author-side source; every agent reads it from here instead of holding a
// copy. Returns "" when the section is absent — most workflows won't have one.
func (hcpo *StepBasedWorkflowOrchestrator) ResolveWorkflowConstraints(ctx context.Context) string {
	_, _, constraints, err := ReadWorkflowSoulSections(ctx, hcpo.GetWorkspacePath(), hcpo.ReadWorkspaceFile)
	if err != nil {
		return ""
	}
	return stripSoulTodoPlaceholder(constraints)
}

// BuildWorkflowConstraintsBlock renders the constraints section as a binding
// prompt block, or "" when there are none (so templates can `{{if}}` on it).
//
// The read-only wording is deliberate and matches the folder guard: steps and
// post-step agents have soul/ on their read paths and never on their write paths.
// An agent that believes a constraint is wrong must surface it, not edit it —
// the CONCERNS channel is the supported route.
func BuildWorkflowConstraintsBlock(constraints string) string {
	trimmed := strings.TrimSpace(constraints)
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## BINDING CONSTRAINTS (owner-approved, from soul/soul.md)\n")
	b.WriteString(trimmed)
	b.WriteString("\n\nThese are explicit owner decisions and they bind this run. Rules:\n")
	b.WriteString("- Use these values as authoritative. If a step description, learnings file, or saved script states a DIFFERENT value for the same constraint, the constraint above wins — that other copy is stale.\n")
	b.WriteString("- Do NOT restate a constraint value as a literal in anything durable you write (learnings, knowledgebase, saved scripts). Read it from the constraint at run time so it cannot drift.\n")
	b.WriteString("- soul/soul.md is READ-ONLY to you. Never edit it. If a constraint looks wrong, contradicts your instructions, or blocks the task, report it with a `CONCERNS:` line — do not silently work around it or substitute your own value.\n")
	return b.String()
}

// stripSoulTodoPlaceholder treats scaffolded `<TODO: ...>` single-line placeholders
// as empty. Multi-line content that happens to mention TODO is preserved.
func stripSoulTodoPlaceholder(v string) string {
	t := strings.TrimSpace(v)
	if !strings.Contains(t, "\n") && strings.HasPrefix(t, "<TODO:") && strings.HasSuffix(t, ">") {
		return ""
	}
	return t
}
