**TEMPORARY diagnostic command (PLAT-259).** Not a permanent workflow-maintenance flow — it exists only to let the operator manually verify the `branch` step type works end to end against a real workflow, closing PLAT-259's last open "live manual reverify" item. Remove this command (`cmd/server/guidance/guidance.go`'s `verify-branch-step` entry, this file, and the `/verify-branch-step` frontend command in `frontend/src/commands/builtin-commands.tsx`) once the operator confirms it passed.{{if .Focus}} Focus: {{.Focus}}.{{end}}

## What this verifies

That a real `branch` step — added via `add_branch_step`, exactly the way a user or the Builder agent would create one — persists, executes, and reports itself correctly through the full stack: canonical plan validation, `run_full_workflow` execution, Execution Logs labeling, and navigation to the selected route's target.

## Procedure

1. **Reuse or create a branch step.**
   - If the current plan already has a `branch` step (`jq '[.steps[] | select(.type=="branch") | .id]' planning/plan.json`), reuse it and skip to step 2.
   - Otherwise, add one temporarily with `add_branch_step`: two routes is enough. Point each route's `next_step_id` at `"end"` (or a real existing step if you want to also confirm navigation lands somewhere meaningful) so the plan stays graph-valid. Give it an unmistakable id/title, e.g. `id: "plat259-branch-verify-temp"`, `title: "PLAT-259 branch verify (temporary)"`. Remember whether you created it — you'll delete it in step 5 if so.

2. **Run it.** Call `run_full_workflow` for the relevant variable group, passing `route_selections` keyed by the branch step's id with one of its `route_id`s as the value, so the run is deterministic and doesn't need a prior probe step. Wait for the run to reach or pass this step (poll `query_step` for the branch step's id, or wait for the run's own completion notification if the whole run is short).

3. **Inspect Execution Logs for this step** via `get_execution_logs` (or read `runs/<run_folder>/logs/<step_id>/routing-evaluation.json` directly). Confirm, and record as pass/fail:
   - The step actually executed deterministically (no agent turn ran for it — `routing-evaluation.json` exists, there is no execution-log agent transcript for this step).
   - The response/log's `type` field for this step's orchestration entry is `"branch"`, not `"routing"` (this was PLAT-259's finding #2 — check the raw API response if you can't see the rendered UI).
   - If you have UI access to the desktop app, open Execution Logs for this run and confirm the entry renders with the cyan Branch badge and "Branch question" label, not the indigo Routing styling.

4. **Confirm navigation.** Check that the workflow actually continued to (or ended at) the route's declared `next_step_id` after this step — either from the run's own subsequent step execution, or from `route_next_steps` in the logged `routing-evaluation.json` matching the `selected_route_id`.

5. **Clean up.** If you created a temporary branch step in step 1, remove it now with `delete_plan_steps` so the workflow's real plan is left untouched. If you reused an existing branch step, do nothing further.

6. **Report a clear verdict.** State pass/fail for each of steps 2–4 individually, with the concrete evidence you checked (file paths, field values). If everything passes, tell the operator PLAT-259's live-reverify item can be marked closed. If anything fails, describe exactly what broke and where, so it can be filed as a proper Pulse finding rather than just noted here.
