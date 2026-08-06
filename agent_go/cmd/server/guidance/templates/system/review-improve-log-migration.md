# Upgrading an old-format Pulse journal

One-time migration contract for `builder/improve.html`. Load this only when a
log is old-format and must be upgraded before anything is appended to it. The
steady-state format rules live in `references/review-improve-log.md`; this file
covers only the upgrade.


An existing `builder/improve.html` is **old-format** — and must be upgraded, not appended to — if it has **any** of:

- a title like "Improvement Ledger";
- `## Active Improvement Index` / `## Recent Entries` / `## Archive Index` headings;
- ```improve-decision``` fenced/`<script>` JSON blocks;
- `F-…` / `I-…` ids;
- legacy Markdown improve logs;
- its own ad-hoc CSS (`.summary` / `.badge` / `.stats`, system-ui body) instead of the skeleton's;
- no `<meta name="viewport">`;
- missing `data-pulse-schema="5"` on the root `<html>` element;
- missing mobile-first stacked `.status` / `.run` / `.entry` layouts or prose-safe overflow rules;
- an `.etitle` rule missing `flex:1 1 auto`, or an `.ehead > .when` rule that keeps `margin-left:auto` / `white-space:nowrap` in the base mobile CSS. That older skeleton collapses entry titles and body text into narrow columns beside timestamp metadata, leaving the card half-empty in the right panel.
- any recent-runs table/flex/grid whose date/status/type/age metadata can shrink into one-character columns. This usually comes from global `overflow-wrap:anywhere` on `body`, `td`, or metadata cells. Rewrite those rows as stacked/mobile-first cards or keep metadata/chips non-wrapping (`white-space:nowrap; overflow-wrap:normal; word-break:normal`) while only prose/evidence fields use `overflow-wrap:anywhere`.
- any recent-runs desktop layout that puts the long `.note`/evidence text beside date/status/type/age metadata. The note must sit on a full-width second row so the run list stays readable in both the right panel and a wide browser.
- any visible `.filters`, `.technical`, `.workqueue`, `.workitem`, signal-tile, cost-tile, or Maintenance Radar block. Remove it from the active journal; its operational detail belongs in Pulse. Preserve only material lifecycle history.
- missing `data-date`, `data-kind`, `data-pulse-section`, or `data-module` attributes on run rows and timeline entries. Backfill dates/kinds/modules/sections from visible dates, run folders, entry labels, or best available evidence. Do not silently default every unclassified historical card to Bug Review; preserve it as `run_summary`/`reflection` when no specific reviewer can be established.
- missing `.worklabel` CSS/action-label examples. Current logs need action chips such as `Bug fix`, `Improvement`, `Advisor idea`, `Artifact drift`, `Report fix`, `Eval fix`, `Cost/time`, `Backup/publish`, `Needs input`, and `Manual` so the user can scan what kind of work happened.
- a separate "Recent runs" strip followed by a separate flat timeline, instead of one date-grouped Activity section (`.daygroup` wrapping that date's `.run` plus its `gate`/module/Fixer entries together). Upgrade to the current Activity structure — see review-improve-log-skeleton.md.
- a text-heavy first screen, a summary other than the three-cell `Latest Pulse` contract, no hidden `#pulse-agent-handoff` recovery marker, or recent runs rendered as a dense table. Upgrade it to the lightweight journal shell before appending new entries.
- a visible `.coverage`, `.covitem`, `.assumptions`, `.worksummary`, `.workstat`,
  `.modfields`, or `.agentlog` block. Reviewer activity, product assumptions,
  backlog counts, and operational details now belong only to the Pulse popup.
- standing `.entry.open` cards in the active timeline. Preserve them in a
  monthly archive and keep current state/evidence in SQLite.

Missing `#pulse-bug-verdict` or `#pulse-goal-verdict` alone does **not** require a full old-format rewrite when the rest of the current skeleton is intact. Insert the standard `.verdicts` block in place and preserve all existing cards and history.

**Do NOT append your new entry into the old structure** — that produces good content in a stale, off-brand shell. Instead, **rewrite the entire document using `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log-skeleton.md"}])`** as a one-time upgrade:

1. Read the old file in full.
2. Load `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log-skeleton.md"}])` and write the skeleton fresh: header + verdict pills, one status headline, the three-cell Latest Pulse brief, the `<!-- LOG ENTRIES: newest first -->` anchor, hidden recovery marker, and archive section. Omit skeleton instructions and example comments from the saved HTML. Goal remains in `soul/soul.md`, rendered by Runloop's Goal tab.
3. Carry still-relevant material decisions, issue transitions, fixes, and runs forward as a concise newest-first timeline. Preserve important active history; archive only genuinely safe resolved history in the matching monthly archive rather than deleting it to meet an item count.
4. Delete any legacy `.md` (`execute_shell_command`) so nothing is duplicated.

After this one rewrite the file is in skeleton format; from then on refresh the compact projection and prepend only material lifecycle events. The structured JSON schema and the dual `F-/I-` id system are retired.
