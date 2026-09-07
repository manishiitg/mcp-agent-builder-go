# Browser Automation in Workflows

Browser workflows use the `agent-browser` skill and the managed
`agent_browser` tool. Set `capabilities.browser_mode` to `auto`, `headless`, or
`cdp`; use `none` when browsing is not required.

## Authoring sequence

1. Load the installed command guide with
   `agent_browser(command="skills", args=["get", "core"])`.
2. Open or select the workflow's labeled tab.
3. Take an interactive snapshot.
4. Identify the intended control and act with its current ref or a verified locator.
5. Verify the expected state; re-snapshot after navigation or DOM updates,
   switching tabs, or when ref freshness is uncertain.
6. Save stable site knowledge to the workflow's learnings.

## Persisted scripts

Snapshot refs such as `@e1` are valid only for the current page state. A saved
script may resolve a fresh ref from the current snapshot by role, accessible
name, and surrounding context, or use a verified semantic locator or DOM hook.
Save this locating recipe, never a snapshot's literal ref as reusable config.
Runtime snapshots can remain as evidence. CSS discovery is not required for
every browser step.

Candidates include role plus accessible name, labels, test attributes,
hand-written semantic `id`/`name`, and `aria-label`. None guarantees stability:
a Like button may become Unlike. Scope repeated controls to their intended
row/card and require one intended actionable match; do not choose the first
match when the target is ambiguous. Verify the resulting page/business state
before recording success or retrying.

Avoid generated framework IDs, hashed class names, and `nth-child` chains.
When the accessibility snapshot is insufficient, use a scoped read-only `eval`
to inspect relevant DOM attributes. Do not dump the full page, read credentials,
or click/submit through a discovery probe. Treat any discovered locator as a
candidate to verify. The attached `agent-browser` skill's **Selector Discipline**
section is the shared authoring and learning contract.

## CDP workflows

CDP mode attaches to a visible Chrome and can reuse existing login state. Keep a
stable labeled tab for each workflow or account. A single shared CDP browser is
the normal concurrency model; configure multiple CDP ports only when one
workflow genuinely needs independent Chrome profiles, such as testing two
logged-in accounts on the same site.

## Debugging

Use `network`, `console`, `errors`, screenshots, HAR capture, recording, and
tracing through the same managed tool. This preserves the workflow's tab lock
and session identity. Do not use raw CDP calls or shell-launched browser actions.

See [the core browser reference](../core/browser.md) for setup, isolation,
artifact handling, and operational limits.
