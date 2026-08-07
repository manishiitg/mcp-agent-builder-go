# Video Studio Inside AgentWorks

**Status:** Proposed migration target
**Worktree:** `/Users/mipl/ai-work/mcp-agent-builder-video`
**Related:** `reusable_vertical_product_platform.md`, `video_studio_local.md`, and `../handover/video_studio_handover.md`

## Decision

Move Video Studio from a standalone local application into AgentWorks as a
built-in product surface.

The target has:

- one AgentWorks Go server;
- one AgentWorks frontend and desktop bundle;
- one authentication and provider-credential system;
- one shared agent, workspace, workflow, event, and file runtime;
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

## Ownership boundary

### Shared AgentWorks platform

AgentWorks owns:

- users, authentication, authorization, and provider credentials;
- coding-agent construction, continuation handles, provider adapters, tmux
  lifecycle, cancellation, and steering;
- the MCP bridge, workspace service, shell execution, and folder guards;
- workflow execution, routing, run state, and durable human input;
- normalized streaming and execution events;
- file and artifact browsing primitives;
- notifications, secrets, costs, and lifecycle management;
- optional services such as schedules, Pulse, and connectors.

### Video Studio product

Video Studio owns:

- video projects and project membership rules;
- video-specific system prompts and skills;
- fixed cinematic, explainer/infographic, and QA workflows;
- `show_video` and its deterministic QA evidence gate;
- presented-video records and video discovery;
- the Projects landing screen;
- Videos, Files, and Workflow product panels;
- video-specific empty states, copy, approval rules, and visual design.

Different skills from ordinary AgentWorks are expected. Reuse applies to skill
registration, resolution, projection, and stage attachment, not to the contents
of a product's `SKILL.md` files.

Schedules, Pulse, and connectors remain disabled unless Video Studio gains a
real product requirement for them. A product does not need to expose every
platform service.

## Backend composition

Keep the domain implementation in `agent_go/internal/videoproduct`, but stop it
from constructing and listening on a separate HTTP server. Expose a product
constructor that receives AgentWorks services:

```go
type Config struct {
    Agents        agent.Factory
    Workspace     workspace.Service
    Secrets       secrets.Store
    Events        events.Bus
    Workflow      workflow.Service
    Notifications notifications.Service
    Auth           auth.Service
    Storage        storage.Factory
}

type Application struct {
    HTTP    http.Handler
    Workers []Worker
    Close   func(context.Context) error
}

func Build(ctx context.Context, cfg Config) (*Application, error)
```

This is one composition function, not a large plugin interface containing
dozens of registration methods.

AgentWorks mounts the returned routes under a product namespace:

```text
/api/products/video-studio/projects
/api/products/video-studio/projects/{id}/messages
/api/products/video-studio/projects/{id}/files
/api/products/video-studio/projects/{id}/videos
/api/products/video-studio/projects/{id}/workflows
```

The first migration should keep `~/VideoStudio/video-studio.db` and
`~/VideoStudio/projects/` even though the AgentWorks process opens them. Running
inside one server does not require an immediate data rewrite. Keeping the
working directories stable also preserves Claude Code continuation and existing
workflow artifact paths.

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

The shared shell owns:

- message rendering and assistant streaming;
- thinking presentation;
- the composer, attachments, steering, and cancellation;
- tool and background-run activity;
- auto-scroll and user-controlled scroll behavior;
- responsive panel/drawer layout;
- the workspace file browser and standard previewers.

Video Studio supplies its Projects home, Videos panel, fixed workflow reference,
and product-specific visual treatment.

The current implementation already reuses `ChatRenderer`,
`ProjectFileBrowser`, and the normalized `execution-events` package. The next
frontend extraction is the surrounding chat/workspace shell, not the Video
Studio Projects page.

## Product and workflow manifests

A product manifest selects composition; it does not replace product Go and
React code:

```yaml
id: video-studio
name: Video Studio
surface: projects

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

skills:
  - video-creation
  - video-shot-generation
  - video-editing
  - video-quality
  - html-composition

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

## Migration sequence

1. Add characterization tests for current project chat, file listing, video
   presentation, workflow routing, QA gating, and continuation behavior.
2. Refactor `videoproduct.Server` into a product `Build` function and mount its
   routes inside `cmd/server` under `/api/products/video-studio`.
3. Keep the existing Video Studio database and workspace paths, but replace
   local login and provider credentials with AgentWorks services.
4. Use the shared AgentWorks session, terminal, workspace, bridge, and event
   lifecycle; remove Video Studio's process-global bootstrap and tmux sweep.
5. Move the React implementation into
   `frontend/src/products/video-studio/` and point it at same-origin product
   routes.
6. Add a top-level product-surface selector without adding Video Studio to
   `ModeCategory`.
7. Extract the reusable chat/workspace shell demonstrated by Video Studio and
   AgentWorks, leaving the Projects and Videos experiences product-owned.
8. Convert fixed Go pipeline definitions into validated, read-only YAML that
   compiles into the existing AgentWorks plan model.
9. Verify existing `~/VideoStudio` projects, videos, QA reports, and Claude
   continuation handles still open correctly.
10. Remove `cmd/video-server`, `frontend/video-app`, standalone ports, local
    authentication, and the standalone launcher only after the integrated path
    passes the same end-to-end tests.

## Completion criteria

The migration is complete when:

- AgentWorks launches Video Studio without another process or frontend dev
  server;
- the same signed-in AgentWorks user can open Video Studio projects;
- one AgentWorks provider credential and session lifecycle serve the product;
- existing Video Studio projects remain readable;
- a natural-language request can route, build, QA, present, and play a video in
  the integrated UI;
- Automation and Chief of Staff behavior remain unchanged;
- Video Studio's fixed workflows are visible but not user editable;
- schedules and Pulse remain absent unless explicitly enabled by the product;
- the standalone Video Studio server and frontend package are no longer built
  or shipped.
