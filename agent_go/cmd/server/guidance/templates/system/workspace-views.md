## WORKSPACE VIEWS

UI-control tools belong only to interactive Workflow Builder chats (including
read-only Builders). Scheduled runs, manually triggered schedules, Pulse/child
agents and bot conversations do not receive them. Schedules still use the shared
workflow tools and selected skills; this is an interactive-host capability gate,
not a separate schedule skill profile. Do not ask a schedule to open or manipulate
the foreground workspace. Observing a scheduled conversation does not promote it;
an explicit supported interactive continuation is required.

The right-hand pane of the workflow page shows one view at a time; the toolbar above the chat switches between them. The legacy `open_workspace_view(view)` and `refresh_workspace_view(view)` emit unverified presentation requests. Prefer the acknowledged UI-control tools described below where supported. Request the view that holds what you are talking about instead of describing where to click, but report only what the receipt actually confirms.

### Views cluster
| View id | Shows | Open it when |
|---------|-------|--------------|
| `report` | The workflow's live HTML report: `db/reports/index.html` rendered against `db/db.sqlite` through `window.report` | You built or edited the report, or the user asks to see results, numbers, the dashboard |
| `flow` | The plan as a canvas: steps, routes, branches, sub-agents, with each step's status for the selected run | You added, removed or reordered steps, or the user asks how the workflow is structured |
| `costs` | Token and cost usage per run, per group and per step, from the cost ledger | The user asks what a run cost or which step is expensive |
| `execution-logs` | The run log for the selected run folder: per-step outputs, pre-validation, timing | A step failed or the user asks what happened during a run |
| `learnings` | `learnings/_global/SKILL.md` and per-step learnings: the HOW the workflow has accumulated | You updated learnings, or the user asks what the workflow has learned |
| `knowledgebase` | `knowledgebase/context/` (user-provided rules) and `knowledgebase/notes/` (what the workflow found) | You captured context or wrote notes, or the user asks what the workflow knows |
| `database` | The tables in `db/db.sqlite` with their rows, and `db/README.md` contracts | You wrote or changed rows, or the user asks about stored data |
| `evaluation` | Evaluation results for the selected run against `evaluation/evaluation_plan.json` | You ran or edited the evaluation, or the user asks how a run scored |
| `schedules` | Scheduled runs: cadence, next run, last run, run history | You created or changed a schedule |
| `files` | The workspace file browser | The user wants to open a specific file, or you wrote a file they should see |

### Pulse cluster
| View id | Shows | Open it when |
|---------|-------|--------------|
| `pulse` | Pulse status: work areas, open findings, who owns the next move, gate decisions | The user asks how the workflow is doing or what Pulse found |
| `backup` | Backup status and history | You set up or ran a backup |
| `publish` | The published page and its status | You published or refreshed the public report |
| `notify` | Notification settings and recent deliveries | You changed how the workflow notifies people |

### Setup cluster
| View id | Shows | Open it when |
|---------|-------|--------------|
| `skills` | Skills attached to the workflow | You installed or attached a skill |
| `secrets` | Secret names attached to the workflow (never values) | You set or attached a secret |
| `mcp` | MCP servers and tool allowlists for the workflow | You added or changed a server |
| `browser` | Browser automation settings | You changed browser mode or connections |
| `llm` | The workflow's LLM configuration: tiers and per-step models | You changed which model runs what |
| `bots` | Connected bots (Slack, WhatsApp) for this workflow | You connected or changed a channel |
| `folders` | Folders attached to the workflow | You attached a folder |

### Focusing something inside a view: `target`

Both tools take an optional `target` — what to focus once the view is up. A view with nothing to focus ignores it, so passing one is always safe.

| View | `target` means |
|------|----------------|
| `report` | The top-level tab to switch to, named as the report's own HTML labels it. Delivered to the report as `report.focus` plus a `report:focus` event; a report that does not listen stays where it is. |
| `flow` | A step id, scrolled to and highlighted on the canvas. |
| `files` | A workspace-relative file path, opened in the pane instead of just showing the tree. |
| `database` | A table name. |
| `execution-logs` | A step id. |
| `schedules` | A schedule id or name. |

Views are independent of the chat: opening one changes nothing in the workflow. Prefer one open per reply, the view that best answers what the user asked.
# Acknowledged UI actions (PLAT-292 baseline)

Prefer `list_ui_capabilities`, `get_ui_state`, `perform_ui_action` and
`get_ui_action_result` when available. Discover first; only advertise the
actions in the returned contract. Initial coverage is AgentWorks workspace
view-shell opening and Notify instruction expansion (`run_summary` or
`pulse_review`). Other deep targets, refresh receipts and product adapters
are not yet supported by this protocol.

`applied` means the browser acknowledged the presentation, not that a human
read it. Opening acknowledges the mounted shell, not all underlying data.
`accepted`, `applying`, `expired`, and legacy `requested` are not success.
When a result is uncertain, retrieve it by request ID; never blindly replay
with a new idempotency key. Reuse a key only for the same intended action.
Disconnected or ambiguous browsers require user attention, not broadcasting.

The older open/refresh tools remain for compatibility but return unverified
requests. Do not say their contents were verified or that a refresh succeeded
from that response alone. Presentation tools cannot send notifications, edit
settings, run workflows, reveal secrets, or establish MCP connections.
