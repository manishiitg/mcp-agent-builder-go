# Video Studio Inside AgentWorks

**Status:** Working integrated slice — isolated browser runtime, live Agent
Profile chat, project persistence, QA-gated video presentation, and clean
creator UI implemented and exercised end to end
**Worktree:** `/Users/mipl/ai-work/video-product-worktree`
**Related:** `reusable_vertical_product_platform.md`, `video_studio_local.md`, and `../handover/video_studio_handover.md`

## Decision

Move Video Studio from a standalone local application into AgentWorks as a
built-in product surface.

The target has:

- one AgentWorks Go server;
- one AgentWorks frontend and desktop bundle;
- one authentication and provider-credential system;
- one shared agent, workspace, workflow, event, and file runtime;
- one generic Agent Profile system for specialized prompts, skills, and
  registered tools;
- one durable Tool Presentation Layer shared by native UI and HTML reports;
- a custom Video Studio Projects screen and video-production workspace;
- product-owned video skills, workflows, records, panels, and approval rules.

Video Studio must not become a third AgentWorks agent mode. AgentWorks' current
mode state assumes `workflow` or `multi-agent` throughout the frontend. Video
Studio is a product surface containing its own projects and conversations, not
another interpretation of an AgentWorks chat tab.

Use two levels of selection:

```text
Product surface
├── AgentWorks
│   ├── Automation
│   └── Chief of Staff
└── Video Studio
    ├── Projects
    └── Project workspace
```

## Implementation checkpoint (7 August 2026)

The first usable migration slice is implemented in this worktree:

- `scripts/run-local-instance.sh` starts a named AgentWorks instance with
  dedicated API/frontend ports, Electron data, workspace documents, logs,
  caches, binaries, tmux socket namespace, browser configuration, runtime
  configuration, environment file, and process lock;
- the normal runner supports browser-only and build/preview modes without
  rewriting the tracked frontend runtime configuration;
- Electron accepts an isolated user-data directory and does not import the
  normal desktop profile into an explicitly isolated instance;
- browser sessions are instance-prefixed, and both shell-level and Go-server
  workspace-wide browser cleanup are disabled for isolated instances;
- strict launch checks refuse occupied ports and a second live launcher for the
  same state root; shutdown targets only the processes started by that runner;
- `pkg/agentprofiles` provides profile validation, immutable in-memory
  versions, owner-scoped resolution, safe prompt rendering, capability policy,
  and a code-owned registered tool-factory contract;
- AgentWorks registers the five Video Studio skills, the fixed production
  pipelines, the product workspace initializer, a read-only built-in
  `video-studio` profile, and the QA-gated `video.show-video` tool at startup;
  authenticated generic list/get/validate profile endpoints are available;
- `/api/query` resolves the trusted profile from the project workspace,
  attaches its system prompt, skills, tools, and workspace guard, then reuses
  the normal AgentWorks session, continuation, cancellation, steering, and
  streaming lifecycle;
- project discovery and creation reuse workspace document APIs, product reads
  reuse the managed SQLite query client, and agent-owned presentation writes
  reuse the authorized workflow database mutation surface. No
  `/api/products/video-studio/*` CRUD API was added;
- the generic workspace file response now has correct MIME types, streaming,
  and HTTP range support for playable and seekable media;
- `media.video` presentation records are stored in each project's standard
  `db/db.sqlite`, loaded by the shared frontend presentation layer, and shown
  by the native Video Studio video player;
- the main frontend now has a persisted, top-level AgentWorks / Video Studio
  product selector that is separate from AgentWorks' Automation, Chief of
  Staff, and Org mode selector;
- Video Studio has its own trusted built-in mark, Projects landing screen,
  workspace-backed searchable project grid, new-project interaction, and live
  project workspace under `frontend/src/products/video-studio/`, while
  continuing to use AgentWorks authentication;
- Video Studio deliberately uses a clean product conversation renderer and a
  simplified composer. The tmux-backed coding agent remains an internal
  runtime detail: the product shows no terminal, Raw/Formatted switch, provider
  badge, command syntax, raw tool log, or AgentWorks mode control. It does show
  concise, product-safe thinking, tool, workflow-route, and step status;
- the last selected product and last Video Studio project survive refresh, and
  the fixed project session hydrates its saved conversation after a reload or
  backend restart.

This is not the complete generic product platform. User-authored Agent Profile
CRUD/version management, generic presentation actions, sandboxed
workspace-built UI activation, configurable large-media uploads, and removal
of the standalone Video Studio application remain future slices. The currently
implemented built-in profile is sufficient for live Video Studio projects and
real video production.

Verification completed for this checkpoint:

```text
scripts/test-local-instance.sh
go test ./...                         # agent_go/
go test ./...                         # workspace/
npm test                              # frontend/ (69 files, 426 tests)
npm run build                         # frontend/, bundle budget passes
isolated browser-only product acceptance
```

The isolated browser runtime uses frontend port `52733`, Agent API port `19743`,
Workspace API port `19744`, and a worktree-local state root. It does not launch
Electron or touch the normal application profile. This is important after a
previous Electron run consumed excessive memory on the development laptop.

### Interactive acceptance result (7 August 2026)

The integrated product has been tested as a signed-in user in the isolated
browser runtime. Product switching, last-product persistence, last-project
persistence, conversation hydration, project creation, project opening,
search, back navigation, attachments, cancellation, live steering, and the
Videos/Files/Workflow panels are connected to real shared runtime state.

A human-style acceptance brief requested an eight-second, 16:9 animated
LumaDesk launch teaser with a polished dark-violet treatment, tagline, final
brand card, rendered MP4, final quality checks, and presentation in the UI. The
profile agent reused the project draft, rendered 240 frames at 1920x1080 and 30
fps, encoded H.264/AAC, added tonal audio, sampled six QA frames, generated a
contact sheet, checked duration/audio/black and frozen frames, adjusted audio,
and wrote a passing `quality-report.json`. `show_video` then created the durable
`media.video` record and the native player loaded the 8.000-second 1920x1080
file with no media error. A real range request returned `206` and the requested
byte range.

Two integration bugs were found by that acceptance test and fixed: profile
metadata uses a user-relative workspace path while the agent folder guard uses
the canonical `_users/<id>/...` path, and the authorized SQLite mutation API
expects the user-relative path. Evidence reads now use the canonical guarded
root; database mutation uses the authenticated user-relative root. The project
session ID is also forwarded to workspace evidence reads.

Visible acceptance is intentionally non-technical. During generation the user
sees their brief, a compact Thinking state, named production activities and
workflow progress, cancel/steer controls, and the finished answer. Terminal
bytes, tmux panes, raw tool arguments/results, internal logs, provider/model
labels, and command/skill/server syntax never appear. After refresh the same
conversation, project, passing video, player, download action, and production
workflow remain visible.

## Ownership boundary

### Shared AgentWorks platform

AgentWorks owns:

- users, authentication, authorization, and provider credentials;
- coding-agent construction, continuation handles, provider adapters, tmux
  lifecycle, cancellation, and steering;
- the MCP bridge, workspace service, shell execution, and folder guards;
- workflow execution, routing, run state, and durable human input;
- workspace creation, discovery, access control, and the managed SQLite query
  and mutation surfaces;
- normalized streaming and execution events;
- file and artifact browsing primitives;
- notifications, secrets, costs, and lifecycle management;
- optional services such as schedules, Pulse, and connectors.

### Video Studio product

Video Studio owns:

- the meaning and schema of a Video Studio project inside an AgentWorks
  workspace;
- video-specific system prompts and skills;
- fixed cinematic, explainer/infographic, and QA workflows;
- `show_video` and its deterministic QA evidence gate;
- the semantics of presented videos and their discovery through generic
  `media.video` presentation records;
- the Projects landing screen;
- Videos, Files, and Workflow product panels;
- video-specific empty states, copy, approval rules, and visual design.

Different skills from ordinary AgentWorks are expected. Reuse applies to skill
registration, resolution, projection, and stage attachment, not to the contents
of a product's `SKILL.md` files.

Schedules, Pulse, and connectors remain disabled unless Video Studio gains a
real product requirement for them. A product does not need to expose every
platform service.

## Coding-agent tools and isolation policy

Tool access, approval behavior, and process isolation are independent choices.
`agent_tools.mode` decides which tools the coding agent can call;
`approvals.mode` decides how proposed native actions are reviewed; and
`security.mode` decides what the coding-agent process can access when native
tools are enabled.

| `agent_tools.mode` | Meaning |
|---|---|
| `mcp_only` | Native coding tools are disabled. File and shell work goes through guarded AgentWorks MCP tools. This is the current cross-provider safe mode. |
| `hybrid` | Native Read/Edit/Write/Shell tools handle ordinary project work, while MCP remains available for product capabilities such as `show_video`, browser, secrets, and workflow control. |
| `native_only` | Only provider-native tools are available. This is useful for diagnostics but is not a normal Video Studio mode because product capabilities would be unavailable. |

| `security.mode` | Meaning |
|---|---|
| `compatibility` | The coding CLI uses its normal host environment. MCP calls remain guarded, but native tools are not restricted by AgentWorks FolderGuard. |
| `verified` | The entire coding CLI process is launched under an enforced policy granting only approved workspace and required runtime/auth paths. It retains the user's normal provider login. |
| `isolated` | The CLI receives a private home/account environment plus only approved workspace paths. This is the strongest boundary and requires separate authentication support. |

| `approvals.mode` | Meaning |
|---|---|
| `provider_auto` | Use the coding provider's own guarded automatic reviewer: Claude Auto Mode, Cursor Auto-review, or Codex Guardian/Auto-review. This is the recommended hybrid setting. |
| `approve_all` | Skip provider approval prompts and classifiers. This is an explicit dangerous opt-in for trusted local automation; it does not itself disable the selected security sandbox. |

The current FolderGuard and shell `sandbox-exec` enforcement sit behind MCP
executors such as `execute_shell_command`. They do not automatically constrain
a provider's native tools, and setting the CLI working directory is guidance,
not a security boundary. Consequently, simply enabling native tools produces
`hybrid + compatibility`: better coding-agent ergonomics, with an explicitly
accepted risk that the provider may access files outside the active project.
This is a valid opt-in for a trusted local developer machine and must be shown
as such in configuration and UI; it must never be silently selected for hosted
or multi-user deployments.

`hybrid + verified` requires the sandbox to wrap the whole coding CLI process,
so native tools and their child processes inherit the same path restrictions.
MCP tools continue to apply their own FolderGuard, authorization, and product
checks. The system prompt may guide tool choice but is never an enforcement
mechanism. A provider without certified whole-process enforcement must fail
closed or fall back explicitly to `mcp_only`; it must not silently degrade to
unsandboxed hybrid mode. At present the strict whole-process path is available
for Codex on supported macOS hosts, while Claude Code, Cursor, and Pi still need
independent sandbox and credential-path certification.

The intended reusable manifest shape is:

```yaml
agent_tools:
  mode: hybrid # mcp_only | hybrid | native_only
  native:
    filesystem: workspace_write # read_only | workspace_write
    network: disabled # disabled | enabled
  mcp:
    enabled:
      - show_video
      - agent_browser
      - secrets
      - workflow_control

approvals:
  mode: provider_auto # provider_auto | approve_all

security:
  mode: compatibility # compatibility | verified | isolated
```

`provider_auto` maps to `--permission-mode auto` for Claude Code,
`--auto-review` for Cursor, and `--ask-for-approval untrusted` plus
`approvals_reviewer="auto_review"` for Codex. `approve_all` maps to Claude
Code's `--dangerously-skip-permissions`, Cursor's `--force`, and Codex
`--ask-for-approval never`. Codex's combined
`--dangerously-bypass-approvals-and-sandbox` must not be used for this mapping,
because approval policy is not allowed to silently weaken `security.mode`.

For the initial opt-in, `hybrid + compatibility` exposes the coding provider's
native tools and records that the user accepted host-filesystem risk. The
future preferred interactive default is `hybrid + verified` after the selected
provider is certified. Tightly scoped workflow stages remain `mcp_only` unless
their entire CLI process runs inside a stage-specific verified or isolated
workspace.

## Backend composition

Start with no Video Studio HTTP server and no product-specific HTTP endpoints.
A Video Studio project is an AgentWorks workspace, created and discovered
through the existing workspace APIs. A small product manifest identifies the
workspace as Video Studio and supplies stable project metadata. Structured
product state lives in the workspace's standard `db/db.sqlite`; video and QA
artifacts use the standard durable workspace file locations.

Reuse the existing AgentWorks surfaces directly:

| Product need | Existing AgentWorks surface |
|---|---|
| Login and current user | `/api/auth/*` |
| Start or continue agent work | `/api/query` with a product session ID |
| Steering and cancellation | existing session live-input and cancel routes |
| Streaming, status, and activity | existing session event routes |
| Project creation and discovery | workspace/folder APIs plus product manifest |
| Project files and artifacts | workspace document and file APIs |
| Read structured product state | managed read-only SQLite query surface, including `window.report.query` in report views |
| Agent-owned structured writes | authorized `mutate_workflow_db` |
| Provider credentials and secrets | existing AgentWorks credential and secret APIs |

The main Video Studio React surface may use the existing frontend
`queryWorkflowDB` client; an embedded HTML report uses `window.report.query`.
Both are read-only views of the same managed SQLite database. Deterministic UI
actions such as create, rename, and archive should first use existing
workspace/document operations. Agent-produced records, including presented
videos and QA results, are written through the authorized workflow database
mutation tool.

Before adding any Go route, record the missing capability in a reuse matrix.
Prefer extending a genuinely reusable AgentWorks primitive when more than one
product needs it. Add a thin `/api/products/video-studio/*` endpoint only when
the operation is product-specific, requires a deterministic trusted write, and
cannot be expressed safely through an existing AgentWorks API. Zero custom
product endpoints is the starting target, not an absolute constraint.

Keep the remaining domain implementation in
`agent_go/internal/videoproduct` only for product-owned composition such as
skill registration, pipeline compilation, project schema initialization, and
the deterministic `show_video` QA gate. It must not construct another HTTP
server, authentication system, provider runtime, workspace service, or session
registry.

### Generic main-agent profiles

Video Studio is the first consumer of a generic Agent Profile system. A profile
defines a specialized main agent without creating another server or chat
runtime:

```go
type AgentProfile struct {
    ID           string
    Name         string
    Version      int
    SystemPrompt string
    Skills       []string
    Tools        []ToolBinding
    Runtime      RuntimePolicy
    BuiltIn      bool
    OwnerID      string
}

type ToolBinding struct {
    ID     string
    Config json.RawMessage
}
```

`RuntimePolicy` may pin a shared AgentWorks provider/model when a product needs
a specific coding-agent runtime. The binding selects an existing provider
adapter and the user's existing AgentWorks login; it does not create
product-owned credentials or a second provider integration.

The target generic management surface is:

```text
GET    /api/agent-profiles
POST   /api/agent-profiles
GET    /api/agent-profiles/{id}
PUT    /api/agent-profiles/{id}
DELETE /api/agent-profiles/{id}
POST   /api/agent-profiles/validate
POST   /api/agent-profiles/{id}/instantiate
```

The current slice implements read-only list/get and validation for built-in
profiles. Create/update/delete/instantiate and durable user-owned versions are
future platform work. When implemented, user-owned profiles may choose only
registered skills and tools for which that user has permission. Updating a
profile creates an immutable new version. A workspace and its active sessions
stay pinned to an explicit profile version until an intentional upgrade; a
prompt or tool change must never silently alter an existing continuation.

A workspace manifest binds a trusted workspace to the profile:

```yaml
agent_profile:
  id: video-studio
  version: 2
```

The server resolves this binding only after authenticating the user and
authorizing access to the selected workspace. A client-supplied profile ID is a
hint, never authority to unlock product tools. During the existing `/api/query`
construction path, AgentWorks renders the profile's prompt with a small
allow-listed context, resolves and attaches its skills through the existing
skill system, and constructs its registered tools. The shared AgentWorks
provider adapter, credentials, session, continuation, streaming, workspace,
and event lifecycle then run the agent. A profile may inherit the general
AgentWorks chat model or pin a provider/model; Video Studio uses the latter.

Prompt templates may use server-supplied values such as project title, local
date, and a workspace description. They cannot read arbitrary environment
variables or secrets.

### Declarative product dependencies

Products declare reusable third-party capabilities in their product YAML. The
shared dependency harness provisions them inside each isolated workspace before
the AgentWorks profile resolves selected skills. That gives native coding
agents ordinary `skills/<name>/` folders, keeps the package lock alongside the
project, and avoids one-off install code in every product backend.

```yaml
dependencies:
  skills:
    - id: vendor-skills
      installer: skills-cli
      source: vendor/example
      install: [core-skill, specialist-skill]
      attach: [core-skill]
      refresh_hours: 24

  cli:
    - id: vendor-cli
      package: { ecosystem: npm, name: vendor-cli, version: latest }
      execution: { mode: npx, binary: vendor-cli }
      verify:
        args: [doctor, --json]
        required_json_checks: [Version, Node.js, FFmpeg]
      refresh_hours: 24
      permissions: { network: true, write_paths: [work/, outputs/] }

  mcp_servers: []
```

`install` controls what is kept locally. `attach` is the deliberately small
subset included in every main-agent turn; specialists are read from `skills/`
only when their router skill needs them. `npx` is verified on first use and at
the declared refresh interval, so the selected CLI is fetched without asking a
creator to install it globally. A future MCP entry is declarative too:
`stdio`, `http`, and `sse` transports are validated here, and environment
values must be `secret://<AgentWorks-secret-name>` references. When a product
adds its first enabled MCP server, the shared agent runtime—not product
YAML—will resolve those references and own the connection lifecycle. Video
Studio currently declares none, so no external MCP process is started.

Video Studio is the first concrete configuration: it installs the official
HyperFrames skill catalog, always attaches only the `hyperframes` router, and
verifies the `hyperframes` npx CLI. Other products can use the same section
without creating another dependency installer.

Custom tool code is never accepted through the profile API. Backend packages
register named factories in a generic registry:

```go
type ToolFactory func(RuntimeContext, json.RawMessage) (AgentTool, error)

toolRegistry.Register("workspace.execute-shell", workspace.NewShellTool)
toolRegistry.Register("workflow.run", workflow.NewRunTool)
toolRegistry.Register("video.show-video", videoproduct.NewShowVideoTool)
```

A profile contains only bindings to those registered IDs. Each factory owns and
validates its configuration schema and receives trusted runtime context from
the server. Future products can add backend-only tools or UI-integrated tools
without changing the profile contract.

Video Studio's built-in profile supplies its custom main-agent system prompt,
the five embedded video skills, fixed workflow definitions, and the
`video.show-video` tool. It reuses AgentWorks' shell and workflow tools rather
than registering Video Studio copies.

### Main-agent transport and frontend view

Runtime transport and frontend presentation are independent choices. Normal
AgentWorks chat is transcript-first for every coding CLI: Claude Code, Codex,
and Cursor emit their structured JSONL transcript (assistant messages and tool
starts), even when the CLI itself is running inside tmux for persistence and
live steering. Raw tmux-pane snapshots are deliberately excluded from the
general event stream; they are terminal display chrome, not assistant content.

| Runtime transport | Normal chat event source | Explicit operator terminal |
|---|---|---|
| tmux-backed CLI | provider structured transcript | live tmux pane and control keys |
| structured process | provider structured transcript | unavailable |

This is the AgentWorks default, not a Video Studio special case. An
operator-oriented product may opt into a separate explicit terminal surface;
a creator-facing product may render the same normalized transcript and events
in a purpose-built conversation surface. Exposing Raw, Formatted, or neither
is a product UX decision, not a requirement of the runtime transport. A normal
chat renderer must never reconstruct assistant text from a tmux screen scrape.

The transport capabilities differ:

| Capability | tmux | structured |
|---|---:|---:|
| Formatted transcript | yes | yes |
| Raw terminal and control keys | yes | no |
| Inject user input into a running turn | yes | no |
| Queue a follow-up for the next turn | yes | yes |
| Warm persistent CLI process | yes | no |
| Native continuation handle | yes | yes |
| Cancel the current execution | yes | yes |
| One-shot process lifecycle | no | yes |

Tmux is the only current transport with real live stdin. When a user sends a
message during a running tmux turn, AgentWorks delivers it to the live CLI; the
CLI decides whether to apply it immediately or natively queue it. Structured
transport has no live stdin, so a mid-turn message must remain an explicit,
visible, editable next-turn queue item or the user must cancel and restart the
turn. It must never be presented as successfully steered into the current turn.

An Agent Profile declares required capabilities rather than assuming a
transport from its frontend:

```yaml
runtime:
  transport: auto  # auto | tmux | structured
  provider: claude-code
  model_id: claude-sonnet-5
  capabilities:
    live_input: required       # required | preferred | disabled
    raw_terminal: disabled     # required | optional | disabled
    warm_session: preferred    # required | preferred | disabled
```

Profile validation rejects contradictions such as
`transport: structured` with `live_input: required` or
`raw_terminal: required`. With `transport: auto`, AgentWorks selects a provider
transport satisfying every required capability and as many preferred
capabilities as possible. The resolved session publishes its actual transport
capabilities to the frontend:

```ts
interface AgentTransportCapabilities {
  transport: 'tmux' | 'structured'
  supportsLiveInput: boolean
  supportsRawTerminal: boolean
  supportsInterrupt: boolean
  usesPersistentProcess: boolean
}
```

The shared composer uses those capabilities, not product-specific checks. A
busy live-input session offers immediate delivery; a busy structured session
offers a clearly labelled next-turn queue. A product output renderer consumes
the same normalized event stream for either transport. The Raw toggle may
appear only when `supportsRawTerminal` and product policy both allow it.

Video Studio's initial policy is:

```yaml
runtime:
  transport: auto
  provider: claude-code
  model_id: claude-sonnet-5
  capabilities:
    live_input: required
    raw_terminal: disabled
    warm_session: preferred
```

This selects a tmux-capable runtime for live steering while presenting a clean
creator conversation in Video Studio. It consumes the standard structured
provider transcript, does **not** mount the AgentWorks terminal component, and
does **not** expose a Raw/Formatted switch. The clean renderer keeps user
messages and final assistant answers, maps internal events to short product
progress labels and named production activities, and suppresses terminal text,
raw tool arguments/results, subagent chatter, auto-notifications, provider
details, and technical error duplication. Raw tmux stream text must never be
mounted in the product conversation, including under a Thinking label. The
product composer retains attachments, send, cancel, and live steering but
removes AgentWorks command syntax and provider controls.

AgentWorks may continue to show both Raw and Formatted for operator-oriented
surfaces. Batch, scheduled, or other non-interactive future products may choose
structured transport when live input and a terminal are not required.

### Tool presentation and reporting bridge

Tools must not call React components or construct dashboard-specific HTML. A
generic Tool Presentation Layer separates backend execution from presentation.
A backend-only tool returns only an agent-facing message; a UI-integrated tool
may additionally create, update, or remove versioned presentation records:

```go
type ToolResult struct {
    Message       string
    Presentations []PresentationMutation
}

type Presentation struct {
    ID            string
    Kind          string
    SchemaVersion int
    Scope         PresentationScope
    Title         string
    Payload       json.RawMessage
    Resources     []ResourceReference
    Actions       []PresentationAction
    Status        string
    Revision      int
}
```

AgentWorks provides native renderers for reusable presentation kinds:

```text
media.video       media.audio       media.image
artifact.file     data.table        data.metrics
data.chart        approval.request  form.request
progress.task     notification      report
```

Tools declare the presentation kinds and versioned JSON schemas they are
allowed to emit. The presentation service validates every payload before
accepting it. Unknown kinds use a safe fallback; dynamically created Agent
Profiles may use only generic kinds. A built-in product may register a custom
kind and compiled React renderer only when the generic native components cannot
express the experience. Arbitrary profiles cannot load React or JavaScript
code.

Presentation state is durable in the workspace's standard database:

```sql
CREATE TABLE ui_presentations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  schema_version INTEGER NOT NULL,
  session_id TEXT,
  title TEXT,
  payload_json TEXT NOT NULL,
  resources_json TEXT NOT NULL,
  actions_json TEXT NOT NULL,
  status TEXT NOT NULL,
  revision INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

After a durable mutation, the service emits a normalized AgentWorks
`presentation_created`, `presentation_updated`, or `presentation_removed`
event containing the presentation ID, kind, revision, and session scope. Native
React surfaces subscribe through the existing event stream and render through a
frontend presentation registry. Reports query the same rows through
`window.report.query`; the host forwards presentation-change notifications to
the sandboxed report iframe so it can refresh without polling.

Presentation actions have three behaviors:

- client-only actions such as play, pause, open, and download execute in the
  registered native renderer;
- deterministic backend actions such as approve, reject, retry, or publish use
  the generic `POST /api/presentation-actions` dispatcher;
- agent-mediated actions such as revise or explain send structured presentation
  context through the existing `/api/query` session.

Backend action requests carry presentation ID, action ID, expected revision,
validated input, and an idempotency key. The dispatcher authenticates the user,
authorizes the workspace, resolves an allow-listed action handler, rejects
stale revisions, performs consequential-action confirmation when required,
updates durable state, and emits the next presentation event.

`show_video` uses this layer rather than a product endpoint. It validates that
the candidate and QA report are workspace-relative files, the video exists and
is non-empty, the versioned QA report contains the required evidence and a
passing verdict, and the report names the exact candidate. It then upserts a
`media.video` presentation. The Videos panel and reporting dashboard read that
same durable record, while playback uses the generic workspace media surface.

Presentation scope, user ID, workspace path, and session ID always come from
trusted server context, never model output. Resource paths remain inside the
authorized workspace; payloads are size-bounded and schema-validated; secrets
are forbidden; HTML remains sandboxed; and consequential actions require an
explicit authorization policy.

### Reusable workspace improvements

The generic workspace file endpoint now streams through `http.ServeContent`
with the correct MIME type and HTTP range support, so native and report video
players can seek without a Video Studio media endpoint. The real acceptance
video returned `video/mp4`, `Accept-Ranges: bytes`, and `206` for a range read.

The workspace upload limit is still 10 MB. Making that limit configurable and
streaming larger allowed uploads remains shared platform work. Project creation
currently uses the existing document API to write the small manifest; the
profile initializer safely creates the standard folders, workflow files, and
database schema on first agent use. If future multi-call creation requires
stronger atomicity, add a generic workspace-template instantiation primitive
rather than a Video Studio project endpoint.

The current standalone database, projects, and continuation handles contain
only disposable development data. They do not require an identity, ownership,
or session migration. Integrated Video Studio uses AgentWorks authentication
and AgentWorks user IDs exclusively. The initial integration may either reset
`~/VideoStudio` or keep it for manual development reference, but must not treat
the standalone users, sessions, or ownership records as production data.

After integration, remove or delegate the following standalone responsibilities:

- local `manish / 12345` authentication;
- the Video Studio HTTP listener and CORS policy;
- the separate Claude-token Settings card and vault entry;
- direct provider and MCP bridge startup;
- the `video-*` tmux sweep;
- the standalone `cmd/video-server` entry point;
- ports 3200 and 8200;
- `scripts/run-video-studio.sh`.

Video Studio sessions should use the shared AgentWorks session registry with
product-namespaced logical IDs such as:

```text
video-studio:project:<project-id>
video-studio:workflow:<project-id>
```

For new conversations, AgentWorks creates and owns the provider session and its
continuation handle. A Video Studio project deterministically resolves to its
product-namespaced logical session ID; Video Studio must not persist a second
provider handle. Steering, cancellation, restart recovery, and terminal cleanup
all use the normal AgentWorks session lifecycle. Archiving or deleting a project
must also close or retire its associated AgentWorks sessions according to that
lifecycle.

## Frontend composition

Move the Video Studio frontend into the existing AgentWorks frontend source
tree rather than maintaining another Vite/npm package:

```text
frontend/src/products/video-studio/
```

Add a top-level product-surface selection independent of `ModeCategory`:

```ts
type ProductSurface = 'agentworks' | 'video-studio'
type AgentWorksMode = 'workflow' | 'multi-agent'
```

Every product header shows only the current product's mark and name. Clicking
that current-product control opens one shared dropdown containing AgentWorks
and Video Studio; the selected product is marked, and choosing the other one
switches the entire surface. Other product names are therefore visible only
after deliberate interaction, and an inactive product's branding never appears
in the normal header. The selected `ProductSurface` is persisted locally, so a
full page refresh restores the user's last product instead of reverting to a
hard-coded default.

The product should render through a reusable workspace shell:

```tsx
<ProductWorkspace
  home={<VideoProjects />}
  chat={<ProjectChat />}
  panels={[
    { id: 'videos', component: <VideoPanel /> },
    { id: 'files', component: <WorkspaceFiles /> },
    { id: 'workflow', component: <WorkflowReference /> },
  ]}
/>
```

The shared shell owns runtime behavior, but allows a product-owned presentation
adapter. It owns:

- session submission, normalized events, restoration, and assistant streaming;
- thinking presentation;
- the composer, attachments, steering, and cancellation;
- tool and background-run activity;
- auto-scroll and user-controlled scroll behavior;
- responsive panel/drawer layout;
- the workspace file browser and standard previewers.

Video Studio supplies its Projects home, clean conversation renderer, simplified
composer treatment, Videos panel, fixed workflow reference, and product-specific
visual design. It reuses the shared `ChatArea` lifecycle through its
`contentRenderer` and `inputVariant="product"` extension points; `TerminalCenter`
is not mounted for this product.

The product UI reads projects and product records through the existing
workspace and managed SQLite clients, and sends chat/session actions through
the existing AgentWorks API client. It must not introduce a parallel Video
Studio API client unless the reuse matrix proves a product-specific endpoint is
necessary.

### Frontend file ownership and workspace-built UI

Frontend files have two deliberately different homes:

```text
frontend/src/platform/                      trusted shared React framework
frontend/src/products/video-studio/         trusted built-in Video Studio UI
<workspace>/ui/                             agent-built workspace UI
```

In a conventional AgentWorks workflow this means the generated UI is stored at
`Workflow/<workflow-name>/ui/`, relative to that workflow's selected workspace:

```text
<workspace>/
├── product.yaml
├── plan.json
├── db/
│   ├── db.sqlite
│   └── assets/
└── ui/
    ├── manifest.yaml
    ├── index.html
    ├── app.js
    ├── styles.css
    └── assets/
```

The application shell, product navigation, runtime event adapters, optional
operator Raw/Formatted views, composer behavior, file browser, presentation
renderers, media player, permission boundaries, and error handling remain
compiled React under `frontend/src`. A built-in product may supply a trusted
compiled renderer, as Video Studio does, but workspace files cannot replace
these platform boundaries.

The workflow main agent may build and revise product-specific dashboards,
overview pages, and custom panels under `<workspace>/ui/`. Agent-authored UI is
HTML, CSS, and JavaScript (or compiled static output), never TypeScript/React
imported dynamically into the AgentWorks bundle. AgentWorks renders it through
a sandboxed `WorkspaceUISurface` iframe.

`ui/manifest.yaml` identifies the entry point, revision, available surfaces,
and requested bridge capabilities:

```yaml
schema_version: 1
id: video-studio-dashboard
revision: 1
entry: index.html

surfaces:
  - id: project-overview
    title: Overview
    slot: workspace-main
  - id: production-status
    title: Production
    slot: workspace-panel

requested_capabilities:
  - database.read
  - workspace-files.read
  - presentations.read
  - presentation-actions.dispatch
  - agent.send-message
```

Requested capabilities do not grant themselves. The host intersects them with
the pinned Agent Profile, authenticated user, and workspace permissions. The
iframe receives a restricted `window.agentworks` bridge for managed database
queries, workspace resource URLs, presentation reads/change notifications,
approved presentation actions, and structured main-agent messages. The current
`window.report.query`, `window.report.get`, and `window.report.fileUrl` remain a
compatible subset of that bridge.

Workspace UI cannot mutate SQLite directly, access cookies or Electron/Node
APIs, read arbitrary filesystem paths, or make arbitrary network requests. It
runs without `allow-same-origin` by default under a restrictive content security
policy. Every bridge message validates iframe identity, capability, workspace
scope, and payload schema.

The agent must call generic `validate_workspace_ui` and
`activate_workspace_ui` tools after writing a revision. Validation checks the
manifest, entry and asset containment, requested capabilities, file sizes,
forbidden resources, and required accessibility metadata. Activation records
the validated revision and emits `workspace_ui.updated`; an invalid new
revision leaves the prior active revision available for rollback. AgentWorks
must never render an arbitrary `ui/index.html` merely because the file exists.

For Video Studio, the Projects landing page and main workspace frame remain
native React. The generic `media.video` presentation renderer supplies the
Videos panel, while a workflow agent may add or update custom dashboards and
panels from `<workspace>/ui/`. A future Agent Profile can therefore deliver a
useful custom product surface without shipping arbitrary frontend code inside
the trusted application bundle.

The current implementation reuses `ChatArea` for session lifecycle and
submission, adds a generic `contentRenderer` extension point, adapts normalized
events through `CleanConversationSurface`, and reuses the managed workspace and
presentation clients. The next extraction is a more complete reusable
product-workspace shell; the Video Studio Projects page remains product-owned.

## Product and workflow manifests

A product manifest selects composition; it does not replace product Go and
React code:

```yaml
id: video-studio
name: Video Studio
surface: projects

agent_profile:
  id: video-studio
  version: 2

capabilities:
  chat: true
  files: true
  workflows: true
  schedules: false
  pulse: false
  connectors: false

workspace:
  layout: chat-with-right-panel
  panels:
    - videos
    - files
    - workflow

pipelines:
  - pipelines/cinematic.yaml
  - pipelines/product-explainer.yaml
  - pipelines/video-quality.yaml
```

The fixed workflows can move from `pipelines.go` into validated product-owned
YAML without becoming user editable:

```yaml
id: product-explainer
name: Product explainer / infographic
route_when: >
  Use for feature explanations, pricing, comparisons, statistics, and
  typography-led product videos.

stages:
  - id: infographic-research
    title: Research
    skill: video-creation
    produces:
      - infographic-research.md

  - id: infographic-concept
    title: Concept
    requires:
      - infographic-research.md
    produces:
      - infographic-concept.md

  - id: infographic-copy
    title: Copy
    requires:
      - infographic-concept.md
    produces:
      - infographic-copy.md

  - id: infographic-layout
    title: Layout
    requires:
      - infographic-copy.md
    produces:
      - infographic-layout.md

  - id: infographic-design
    title: Build panels
    skill: html-composition
    requires:
      - infographic-layout.md
    produces:
      - infographic-design.md

  - id: infographic-render
    title: Render
    skill: html-composition
    requires:
      - infographic-design.md
    produces:
      - infographic-render-report.md
      - infographic.mp4

  - id: infographic-check
    title: Quality check
    skill: video-quality
    produces:
      - infographic-delivery.md
      - quality-report.json
      - qa-contact-sheet.jpg
```

Startup performs:

```text
product YAML + pipeline YAML
        ↓ validate
typed product and pipeline definitions
        ↓ compile
AgentWorks routing and execution plan
```

The Workflow panel remains an informational, non-editable view. The agent still
chooses the route and whether to use direct chat, one stage, or the full
workflow.

## Isolated local development and testing

Development happens from the Video Studio Git worktree, but a Git worktree
isolates only source files. It does not isolate ports, Electron state,
workspaces, tmux sockets, browser processes, credentials, or logs. The feature
must therefore run as a named, isolated AgentWorks instance before it is tested
alongside the main-branch application.

Use a single instance identifier, such as `video-product-dev`, to derive every
runtime resource:

| Resource | Isolated development value |
|---|---|
| Source | `/Users/mipl/ai-work/video-product-worktree` |
| Instance state root | a gitignored directory dedicated to `video-product-dev` |
| Agent API | explicit non-default port, for example `19743` |
| Workspace API | explicit non-default port, for example `19744` |
| Frontend | explicit non-default port, for example `52733` |
| Electron `userData` | `<state-root>/electron` |
| Workspace documents and SQLite | `<state-root>/workspace-docs` |
| Logs and caches | `<state-root>/logs` and `<state-root>/cache` |
| tmux | an instance-owned socket directory |
| Browser automation | an instance-owned profile, CDP port, and process registry |

Do not launch the feature instance with the runner's defaults. The current
runner may reclaim the default AgentWorks ports, Electron currently points at
the normal `runloop-desktop` user-data directory, and browser cleanup includes
globally shared process state. Any of those could interfere with a main-branch
instance even though the code is in another worktree.

Add a generic local-instance launcher to AgentWorks rather than a Video
Studio-specific launcher. Its interface should be equivalent to:

```text
./scripts/run-local-instance.sh \
  --instance video-product-dev \
  --state-root <dedicated-state-directory> \
  --agent-port 19743 \
  --workspace-port 19744 \
  --frontend-port 52733 \
  --app-name "Video Studio (Dev)" \
  --favicon-url /video-studio-favicon.svg
```

The launcher must fail closed when a requested port or state directory belongs
to another live instance. Browser-only operation is the default; desktop
Electron is an explicit `--electron` opt-in. An opted-in isolated Electron
process tree has a 3 GB RSS watchdog, and shutdown also removes re-parented
Chromium helpers only when their command line contains the exact isolated
`userData` path. It must pass an overridable Electron user-data path,
an explicit workspace-documents path, and instance-owned tmux and browser
namespaces to all child processes. Shutdown may terminate only PIDs and tmux or
browser sessions recorded by that instance. It must never discover and kill a
process merely because it occupies a default port. The same launcher should
support a production-like mode that builds the frontend before serving it.

The isolated instance uses the real AgentWorks authentication flow, but stores
its local session and credential records in its own user-data directory. The
developer signs in and configures a test provider credential there; the
launcher must not copy, overwrite, or silently share the normal desktop
profile. Workspace projects and SQLite databases created during tests remain
under the isolated state root and are disposable.

Use four testing layers:

1. Run Go and TypeScript unit tests with temporary directories and fake
   provider/tool implementations.
2. Run integration tests against temporary AgentWorks instances to cover
   Agent Profile resolution, workspace/database access, session events,
   presentation replay, and authorization.
3. Run frontend component tests for the shared chat shell, product routing,
   presentation renderers, and workspace UI sandbox.
4. Run one isolated live smoke test with the real AgentWorks login and a test
   provider credential: create a Video Studio project, continue its session,
   steer it, produce and seek a video, restart the instance, and verify replay.

Before and after the live smoke test, verify that the main worktree is
unchanged, the normal AgentWorks data directory has not been written by the
feature process, default ports and the main tmux/browser namespaces were not
touched, and stopping `video-product-dev` leaves the main application running.
Only commits are shared through Git; feature code reaches `main` only through
the normal review and merge process.

## Migration sequence and status

Steps 1, 2, 6, 7, 8, 9, and 10 have a working integrated slice. Steps 3, 4,
5, and 11 are partially implemented for the built-in Video Studio use case but
still need the broader generic capabilities described below. Steps 12 and 14
remain cleanup/follow-up work. Step 13 is partly covered by the live acceptance
test and must grow into restart/authorization automation.

1. Add the generic isolated-instance launcher and the missing runtime overrides
   for Electron user data and browser ownership. Prove that a feature instance
   can start and stop beside the normal AgentWorks application without changing
   its files, ports, sessions, or processes.
2. Add characterization tests for current project chat, file listing, video
   presentation, workflow routing, QA gating, and continuation behavior. Build
   a reuse matrix mapping every current Video Studio operation to an existing
   AgentWorks API; begin with no custom product endpoints.
3. Implement immutable, versioned Agent Profiles, profile validation and
   management APIs, trusted workspace binding, and the registered tool-factory
   registry. Add capability-based tmux/structured transport policy and expose
   resolved session capabilities to the frontend. Integrate profile resolution
   into the existing `/api/query` agent construction path.
4. Implement the Tool Presentation Layer: schema registry, durable
   `ui_presentations`, normalized presentation events, native renderer registry,
   report/workspace-UI iframe bridge, generic presentation-action dispatcher,
   and validated workspace-UI revision activation.
5. Upgrade the generic workspace media and upload surfaces for MIME-aware range
   streaming and configurable large-file uploads.
6. Define a Video Studio project as an AgentWorks workspace with an Agent
   Profile binding, product manifest, standard `db/db.sqlite` schema, and
   standard durable artifact and `ui/` locations. Create and discover it through
   existing workspace APIs or the generic template-instantiation primitive if
   required.
7. Register the built-in Video Studio profile: custom main-agent prompt, five
   embedded skills, fixed pipelines, project schema, and `video.show-video`
   tool. Make `show_video` emit a validated `media.video` presentation.
8. Replace local login, user IDs, provider credentials, sessions, terminal,
   bridge, events, files, and secrets with their existing AgentWorks surfaces.
   Existing standalone data is disposable development data and does not require
   migration.
9. Move the React implementation into
   `frontend/src/products/video-studio/` and use the existing AgentWorks API,
   workspace, managed SQLite, session event, presentation, and
   `WorkspaceUISurface` clients.
10. Add a top-level product-surface selector without adding Video Studio to
   `ModeCategory`.
11. Extract the reusable chat/workspace shell demonstrated by Video Studio and
    AgentWorks, leaving the Projects and Videos experiences product-owned.
12. Convert fixed Go pipeline definitions into validated, read-only YAML that
    compiles into the existing AgentWorks plan model.
13. Verify newly created projects use AgentWorks user IDs, pinned profile
    versions, and deterministic product session IDs, including continuation,
    restart recovery, steering, cancellation, project archival cleanup,
    managed SQLite reads, authorized writes, presentation replay, native video
    playback, and report refresh.
14. Remove `cmd/video-server`, `frontend/video-app`, standalone ports, local
    authentication, and the standalone launcher only after the integrated path
    passes the same end-to-end tests.

## Completion criteria

The migration is complete when:

- a named local feature instance starts and stops beside the normal AgentWorks
  application without modifying its user data, workspaces, ports, tmux
  sessions, browser processes, or source checkout;
- AgentWorks launches Video Studio without another process or frontend dev
  server;
- the same signed-in AgentWorks user can open Video Studio projects;
- one AgentWorks provider credential and session lifecycle serve the product;
- project discovery, reads, files, chat, events, and credentials use existing
  AgentWorks APIs; any custom product endpoint has a documented capability gap;
- the Video Studio main agent is a pinned, versioned Agent Profile whose prompt,
  skills, and registered tools are resolved through the generic profile system;
- profile validation rejects transport/capability contradictions, Video Studio
  runs with live-input support while showing only its clean creator
  conversation, and a structured-transport test profile visibly queues rather
  than claiming to live-steer a busy turn;
- backend-only and UI-integrated tools share one result contract, and tools can
  emit only registered, schema-valid presentation kinds;
- a `media.video` presentation survives restart, updates the native Videos
  panel through normalized events, and is readable from an HTML report through
  `window.report.query`;
- presentation actions enforce authentication, workspace authorization,
  expected revision, idempotency, and consequential-action policy;
- video files play and seek through the generic MIME-aware workspace media
  surface without a Video Studio media route;
- trusted shared and built-in product React stays under `frontend/src`, while
  agent-built frontend files stay under `<workspace>/ui/` and render only after
  validated revision activation in a capability-scoped sandbox;
- a natural-language request can route, build, QA, present, and play a video in
  the integrated UI;
- Video Studio never exposes a terminal, tmux pane, Raw/Formatted switch,
  provider/model badge, raw tool log, or AgentWorks mode controls in its normal
  product surface; normalized tool/workflow activity is shown only through
  concise product-facing labels and statuses;
- Automation and Chief of Staff behavior remain unchanged;
- Video Studio's fixed workflows are visible but not user editable;
- schedules and Pulse remain absent unless explicitly enabled by the product;
- the standalone Video Studio server and frontend package are no longer built
  or shipped.
