package step_based_workflow

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents"
)

// learningsGlobalFileMutex serializes direct-mode writes to learnings/_global/
// across parallel steps. Parallel sub-agents under a todo_task each have their
// own MCP session + folder guard, but the _global skill file is shared — without
// a mutex they'd race each other's diff_patches. Held for the duration of the
// direct-learnings continuation turn (see controller_execution.go).
//
// Uses a simple in-process mutex since the turn is inline and short.
//
// LIMITATION (intentional for v1): this is an in-process mutex. It does NOT
// serialize writes across multiple orchestrator processes sharing the same
// workspace (e.g. a multi-node deployment). If that topology becomes real,
// this needs to upgrade to a file lock (flock on learnings/_global/SKILL.md)
// or equivalent cross-process primitive. Not addressed in v1.
var learningsGlobalFileMutex sync.Mutex

// prepareDirectLearningTurn temporarily makes the shared learnings folder
// writable for a direct-learning continuation. It intentionally does not change
// the shell working directory; the learning prompt uses explicit absolute paths
// so normal step/runtime cwd behavior stays untouched.
func (hcpo *StepBasedWorkflowOrchestrator) prepareDirectLearningTurn(agent agents.OrchestratorAgent, addedPaths []string) func() {
	if agent == nil {
		return func() {}
	}

	var restoreFns []func()
	if cfg := agent.GetConfig(); cfg != nil {
		prevRead := append([]string{}, cfg.FolderGuardReadPaths...)
		prevWrite := append([]string{}, cfg.FolderGuardWritePaths...)
		cfg.FolderGuardReadPaths = common.DeduplicateStrings(append(cfg.FolderGuardReadPaths, addedPaths...))
		cfg.FolderGuardWritePaths = common.DeduplicateStrings(append(cfg.FolderGuardWritePaths, addedPaths...))
		restoreFns = append(restoreFns, func() {
			cfg.FolderGuardReadPaths = prevRead
			cfg.FolderGuardWritePaths = prevWrite
		})

		subSessionID := strings.TrimSpace(cfg.MCPSessionID)
		if subSessionID != "" {
			prevCfg := common.GetSessionShellConfig(subSessionID)
			hadPrevCfg := prevCfg != nil
			prevSessionRead := []string{}
			prevSessionWrite := []string{}
			prevSessionWorkingDir := ""
			if prevCfg != nil {
				prevSessionRead = append([]string{}, prevCfg.ReadPaths...)
				prevSessionWrite = append([]string{}, prevCfg.WritePaths...)
				prevSessionWorkingDir = prevCfg.WorkingDir
			}
			widenedRead := common.DeduplicateStrings(append(append([]string{}, prevSessionRead...), addedPaths...))
			widenedWrite := common.DeduplicateStrings(append(append([]string{}, prevSessionWrite...), addedPaths...))
			common.SetSessionFolderGuard(subSessionID, widenedRead, widenedWrite)
			hcpo.grantSessionCDPHostDownloadsReadOnly(subSessionID)
			restoreFns = append(restoreFns, func() {
				if hadPrevCfg {
					common.SetSessionFolderGuard(subSessionID, prevSessionRead, prevSessionWrite)
					if prevSessionWorkingDir != "" {
						common.SetSessionWorkingDir(subSessionID, prevSessionWorkingDir)
					}
					hcpo.grantSessionCDPHostDownloadsReadOnly(subSessionID)
				} else {
					common.ClearSessionShellConfig(subSessionID)
				}
			})
		}
	}

	return func() {
		for i := len(restoreFns) - 1; i >= 0; i-- {
			restoreFns[i]()
		}
	}
}

func (hcpo *StepBasedWorkflowOrchestrator) directLearningsPromptTargetPath() string {
	workflowPath := strings.TrimSpace(hcpo.GetWorkspacePath())
	rel := filepath.Join(workflowPath, LearningsFolderName, GlobalLearningID)
	docsRoot := strings.TrimSpace(GetPromptDocsRoot())
	if docsRoot == "" {
		return rel
	}
	return filepath.Join(docsRoot, rel)
}

func (hcpo *StepBasedWorkflowOrchestrator) buildLearningsContributionTurn(stepID, stepDescription, learningObjective string, isScriptedMode bool) string {
	return BuildLearningsContributionTurnWithTargetAndBrowser(stepID, stepDescription, learningObjective, isScriptedMode, hcpo.directLearningsPromptTargetPath(), hcpo.HasBrowserCapability())
}

// BuildLearningsContributionTurn returns the scripted user message that fires
// one-shot after pre-validation (and after any KB review turn) when the step
// is configured for direct-mode learnings writes. All SKILL.md guidance lives
// in this message — the step's system prompt deliberately says nothing about
// direct-mode learnings, so the agent can focus on the main task during
// execution and switch context cleanly when this turn arrives.
//
// Writes target learnings/_global/SKILL.md — the single global workflow skill
// shared across all steps. Multiple direct-mode steps contribute scoped sections
// to the same file; the serialization mutex prevents parallel writes from
// racing.
//
// Scripted note: the step's main.py is copied into the learnings/<stepID>/ root
// automatically by Go code (saveScriptedScriptToLearnings), independent of this
// direct-mode turn. The step agent is NOT asked to do that copy
// here — that would double-write a shared file and open needless write access
// to learnings/<stepID>/. Direct-mode learnings only targets _global/ for
// author-authored domain knowledge beyond what main.py encodes.
//
// Returns empty when the step shouldn't enter direct-learnings — callers decide
// via shouldDirectWriteLearnings before invoking this.
func BuildLearningsContributionTurn(stepID, stepDescription, learningObjective string, isScriptedMode bool) string {
	return BuildLearningsContributionTurnWithTarget(stepID, stepDescription, learningObjective, isScriptedMode, "")
}

func BuildLearningsContributionTurnWithTarget(stepID, stepDescription, learningObjective string, isScriptedMode bool, targetPath string) string {
	return BuildLearningsContributionTurnWithTargetAndBrowser(stepID, stepDescription, learningObjective, isScriptedMode, targetPath, false)
}

// BuildLearningsContributionTurnWithTargetAndBrowser adds browser-specific
// persistence guidance only when the workflow exposes agent_browser/CDP.
func BuildLearningsContributionTurnWithTargetAndBrowser(stepID, stepDescription, learningObjective string, isScriptedMode bool, targetPath string, hasBrowserAccess bool) string {
	_ = isScriptedMode // retained in the signature in case future behavior diverges by mode; not currently referenced
	description := strings.TrimSpace(stepDescription)
	objective := strings.TrimSpace(learningObjective)
	if stepID == "" || objective == "" {
		return ""
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		targetPath = "learnings/_global"
	}
	skillPath := filepath.Join(targetPath, "SKILL.md")
	referencesPath := filepath.Join(targetPath, "references")

	var b strings.Builder
	b.WriteString("## Learnings Contribution (dedicated turn)\n\n")
	b.WriteString("Your main-step work is complete and pre-validation passed. In this turn only, you have WRITE access to the shared learnings folder: capture HOW to run this task so future runs don't rediscover it.\n\n")

	b.WriteString("**Target:** `")
	b.WriteString(skillPath)
	b.WriteString("` plus linked files under `")
	b.WriteString(referencesPath)
	b.WriteString("/` — the global runbook shared by every step. Use these exact paths; do not rely on your shell working directory. You are appending your contribution, not owning the folder.\n\n")

	if description != "" {
		b.WriteString("**Current step description (source of truth for stale-learning cleanup):**\n")
		b.WriteString(description)
		b.WriteString("\n\n")
	}

	b.WriteString("**Frontmatter:** preserve existing; if creating it fresh, set `name`, `description`, `disable-model-invocation: true`, `user-invocable: false`.\n\n")

	b.WriteString("**Write rules (critical — you are writing to a shared file):**\n")
	b.WriteString("1. **Read first.** `cat '")
	b.WriteString(skillPath)
	b.WriteString("'` and `ls '")
	b.WriteString(referencesPath)
	b.WriteString("'` to see which topics already exist before writing. Use the exact target paths; never write under `runs/`.\n")
	b.WriteString("2. **Preserve existing content.** Use `execute_shell_command` carefully for every write, including creating a new `")
	b.WriteString(filepath.Join(referencesPath, "<topic>.md"))
	b.WriteString("` file. **Do not use shell redirection, heredocs, tee, Python, or built-in file-edit tools to create or edit learning files.** **Never rewrite SKILL.md wholesale** — you'd destroy other steps' contributions.\n")
	b.WriteString("3. **SKILL.md is only the index (~80-100 lines max):** frontmatter, a brief scope note, links to topic files. Every detail from this run — selectors, auth flows, API quirks, timing, retry patterns, format notes — goes in `")
	b.WriteString(referencesPath)
	b.WriteString("/<topic>.md`; link any new file from SKILL.md.\n")
	b.WriteString("4. **Reconcile stale guidance.** Where content you touch describes behavior this run contradicts — obsolete selector, changed API path — replace it in the same patch. Don't delete unrelated guidance this step simply didn't use.\n")
	b.WriteString("5. **Merge, don't duplicate.** If your lesson overlaps a pattern another step already captured, extend that file rather than creating a second home for it.\n")
	b.WriteString("6. **No ephemeral refs.** Session-local browser handles (`@e1`, `e68`) are useless across runs.\n")
	b.WriteString("7. **No fabrication.** Capture only patterns you actually used. If unsure a pattern is reliable, say so in the note.\n")
	b.WriteString("8. **HOW only — not facts or results.** Reusable execution technique; not discovered facts, run results, current values, preferences, or status — those go to the knowledgebase or `db/db.sqlite`, never in learnings. Never write secrets.\n")
	b.WriteString("9. **Never copy an owner constraint VALUE** (caps, limits, thresholds). They live in `soul/soul.md` and are injected every run; a number copied here goes stale the moment the owner changes it. Name the constraint, never its value — and strip any you find, with a `CONCERNS:` line.\n\n")
	if hasBrowserAccess {
		b.WriteString(BuildBrowserLearningRules())
		b.WriteString("\n")
	}

	b.WriteString("**Objective for this step's contribution (the contract):**\n")
	b.WriteString(objective)
	b.WriteString("\n\n")

	b.WriteString("**Raising a concern.** You alone see the execution trail, step description, binding constraints and existing learnings together, and learnings is the only store you can write — so a problem anywhere else is lost unless you report it. Add a line before your summary:\n")
	b.WriteString("`CONCERNS: <what contradicts what, naming both sources and the evidence path>`\n")
	b.WriteString("Use it when the step description contradicts the constraints; when existing learnings contradict the description, the constraints, or this run; when learnings/KB/`db/` state the same fact differently; or when a path, table or field the description names does not exist. Never leave a \"this is stale, use X\" caveat beside the wrong content — fix what you own, report the rest, and state both sides rather than guessing. Not for routine progress or for something the workflow simply hasn't learned yet.\n\n")

	b.WriteString("**Important:**\n")
	b.WriteString("- This is your final learnings turn for this step — there is no second pass.\n")
	b.WriteString("- If there's genuinely nothing new worth capturing (e.g. the step was trivial and the existing SKILL.md already covers it), do NOT force an edit. Reply briefly that no learning changes were needed and why — a `CONCERNS:` line still applies on a no-op turn.\n")
	b.WriteString("- If you did update files, end with exactly one summary line: `Learnings updated: files changed: <comma-separated file list>`.\n")
	b.WriteString("- Available tools: `execute_shell_command` for inspection and permitted writes under `")
	b.WriteString(targetPath)
	b.WriteString("/`, including new files.\n")

	return b.String()
}
