## WORKSPACE VIEWS

The right-hand pane of the workflow page shows one view at a time; the toolbar above the chat switches between them. `open_workspace_view(view)` puts a view on the user's screen, `refresh_workspace_view(view)` reloads one you changed. Open the view that holds what you are talking about instead of describing where to click.

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
| `files` | The workspace file browser | The user wants to open a specific file, or you wrote a file they should see |

### Pulse cluster
| View id | Shows | Open it when |
|---------|-------|--------------|
| `pulse` | Pulse status: work areas, open findings, who owns the next move, gate decisions | The user asks how the workflow is doing or what Pulse found |
| `evaluation` | Evaluation results for the selected run against `evaluation/evaluation_plan.json` | You ran or edited the evaluation, or the user asks how a run scored |
| `schedules` | Scheduled runs: cadence, next run, last run, run history | You created or changed a schedule |
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
