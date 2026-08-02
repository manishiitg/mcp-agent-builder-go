# Refactor records

Implementation migrations: what was changed structurally, why, and how far it
got. Unlike `docs/bugs/`, these describe deliberate rewrites rather than
defects — but each was motivated by defects, and the motivating incidents are
linked from the entries below.

Status is load-bearing here. An in-progress refactor describes an API that does
not exist yet; a shipped one describes the current system.

| Document | Status |
|---|---|
| [mcpagent_public_api_simplification.md](mcpagent_public_api_simplification.md) | **Complete and verified (2026-08-02).** Reduced `mcpagent.Agent` from 70 exported methods and 64 exported fields to 4 methods and 0 fields, and package functions from 148 to 45, with AST golden tests pinning the exact names. Motivated by [custom_tool_category_as_agent_addressing.md](../bugs/custom_tool_category_as_agent_addressing.md): the large surface let callers sequence internal lifecycle themselves, so the same fact lived in several independently stale copies. The final registry follow-up is also complete: redundant `Agent.customTools` state was removed, and direct-tool discovery, bridge schemas, timeout selection, and execution now share one canonical record. |
| [lazy_per_terminal_event_loading.md](lazy_per_terminal_event_loading.md) | **Implemented and verified (2026-08-02).** Child terminals load their transcript on open via a cursor API instead of every session event being pulled and filtered client-side. The duplication hazard this note was written to avoid — reimplementing `getOwnedTerminalOwnerKeys` in Go — was resolved properly: `ResolveTerminalOwnerID` is one write-time resolver shared by the event store and terminal store, and main became a positive owner (`main:<session_id>`) rather than a frontend complement. The retention-policy follow-up is complete: a cross-language contract test pins the frontend set to the backend set plus exactly `streaming_start`, `streaming_chunk`, and `streaming_end`. |
| [native_streaming_stt.md](native_streaming_stt.md) | **In progress (2026-08-02).** Replacing the Python/MLX voice worker with a native Swift helper on FluidAudio (Apache-2.0), whose streaming ASR keeps encoder state across appends. Motivated by measured latency that grows with recording length: live dictation re-transcribes the whole clip every preview tick, and the installed `mlx_audio` has no stateful incremental API to migrate onto (`stream_generate` needs a finished recording; `decode_chunk` is stateless — both verified against the installed package). Carries a large secondary win: it would delete the ~3.1GB Python venv, its install UI, and the MLX unbounded-cache workaround. Python path remains default and untouched until parity is measured. |
| [terminal_live_attach_transport.md](terminal_live_attach_transport.md) | Shipped on `main`; sole transport. |
| [live_attach_app_vs_demo_debug.md](live_attach_app_vs_demo_debug.md) | Resolved, then re-architected in-band. Read with the entry above; alone it describes a superseded design. |
| [cli_live_input_unification.md](cli_live_input_unification.md) | Core implemented surgically. |

## Reading order for the agent-API work

The public-API refactor is the largest change in this folder and it does not
stand alone. In order:

1. [../bugs/README.md](../bugs/README.md) — the shared shape of the defects that
   motivated it: one fact, two sources, nothing checking they agree.
2. [../bugs/custom_tool_category_as_agent_addressing.md](../bugs/custom_tool_category_as_agent_addressing.md)
   — the specific evidence, including the "Why this now warrants a refactor"
   section.
3. [mcpagent_public_api_simplification.md](mcpagent_public_api_simplification.md)
   — the plan, its review, and the open questions.

## Convention

State a status in the first lines. A refactor doc without one is worse than no
doc: a reader cannot tell whether it describes the system, a plan, or an
abandoned direction, and the code will not tell them either.
