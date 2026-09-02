# Reusable Platform for Dedicated Agent Products

**Status:** Proposed architecture  
**Scope:** AgentWorks (workflow engine), SparkQuill/family-server (first vertical),
and 3-10 further dedicated products, each substantially custom. See
"Designing for ten products, not two" — the target count changes the bar for
every abstraction here.

## Decision

Build a small shared backend and frontend platform, then build each dedicated
product as a normal application on top of it.

Dedicated products must reuse operational infrastructure without being forced
into one shared business model. A social-media product may have accounts,
campaigns, discovery workers, and content calendars. A trading product may have
market streams, positions, risk gates, and order approval. SparkQuill may have
parents, children, learning activities, and educational reports. These concepts
belong to their products, not to the platform.

Do not copy `cmd/family-server` to create `social-server` or `trading-server`.
The current family server is useful evidence for which infrastructure should be
extracted, but it should become a consumer of that infrastructure rather than a
template to duplicate.

## Decision record — SparkQuill does not migrate to `product.yaml` (2026-08-16)

**Question asked:** produce a plan to move SparkQuill onto the
`product.yaml` / `agentprofiles.Profile` format that Video Studio and Chief
of Staff now use.

**Decision: do not migrate SparkQuill.** Keep it a separate application.
Let the *next* product be the one built against a platform boundary.

This does not contradict the architecture above — it applies it. This
document already says a dedicated product is "a full application, not a
data-only manifest", and records `Configuration-only fit: no`.
`agentprofiles.Profile` is precisely a data-only manifest, and is not the
platform this document proposes.

### Why

**1. The child-safety boundary would regress.** This alone decides it.
`StrictAllowlist: true` appears in exactly two places in the repo, both in
`cmd/family-server/shell_tool.go` (child `:103`, parent `:187`).
`cmd/server/**` never sets it, and never sets `AllowNetwork`. The child's
shell today is deny-by-default, scoped to one activity folder, with no
network and no secrets; the main server's shell is allow-by-default with
`SECRET_*` injected and unrestricted network. `product_tool_gate.go` cannot
close that gap — `Admit(name string) bool` sees a tool *name*, filters once
at registration, and is fail-open without `mode: allowlist`; its only lever
is removing `execute_shell_command` entirely, which is worse than a jailed
shell. `agent_tools: mode: hybrid` (Video Studio's setting) additionally
hands the model the CLI's own unsandboxed `Bash`/`Read`/`Write`, which the
gate never sees. Concretely: injected text from an uploaded worksheet today
lands in a sandbox with a one-folder blast radius; post-migration it would
land next to `~/.ssh` and the open internet.

**2. Risk is borne entirely by the mature product.** SparkQuill is stable
and in daily real-family use. The benefit of a platform accrues to products
#3+, which would inherit it instead of copying `cmd/family-server`.
SparkQuill gains nothing it lacks today and absorbs all the regression risk.

**3. Near-zero characterization coverage to migrate against.** 12 test
files / 23 test functions, one skipped by default. Zero direct tests for
`chat.go` (772), `whatsapp_bot.go` (1,379), `conversation_store.go` (447),
`parent_tools.go` (405), `child.go` (347), `shell_tool.go` (209),
`handoff.go`, `child_workspace.go`, `whatsapp_routing.go`. Step 1 of the
migration sequence below remains unstarted, and it is a precondition.

**4. The Video Studio precedent does not transfer.** Video Studio was
absorbed successfully (`d4efd631`, 2026-08-08, "remove the standalone Video
Studio application", −11,072 lines; `cmd/video-server` + `frontend/video-app`
deleted after the product surface worked). But
`video_studio_inside_agentworks.md` step 8 states its standalone data was
"disposable development data and does not require migration" — the sentence
carrying the whole argument, and false for SparkQuill's real activity
history, attempts, memory files, and WhatsApp session. Video Studio was also
migrated while being built, not while stable: standalone backend ~3,700
lines vs family-server's 12,104. Only the frontends are comparable
(`video-app` 5,731 vs `learning-app` ~6,300–7,900), and Video Studio had no
Electron shell or native voice helper to consolidate.

**5. This document's own criteria justify a separate app.** "A separate app
is justified by a different user experience, trust boundary, permission
model, release lifecycle, or always-on service topology." SparkQuill has all
five: children vs. professionals; a child-safety trust boundary enforced by
an in-process filesystem sandbox; a PIN/no-auth model vs. three auth modes
and JWT; its own `sparkquill-v*` release cadence; and a server deliberately
kept alive when the window closes, for Pulse and WhatsApp.

**6. It would define the boundary against one consumer** — which the reuse
rule forbids, and which "Open contradictions" below already flags.

### The root cause worth naming

Three of the largest blockers are single-tenancy in three costumes: global
mutable state (`familyState.Child` is a single pointer with no ID field;
`currentActivityDir()` takes no session parameter yet scopes the child
sandbox), process-global env collisions (`MCP_API_URL`/`MCP_API_TOKEN` are
`os.Setenv`'d by both `cmd/server/server.go:1787-1789` and
`internal/agentsession/agentsession.go:453,474-476`; last writer wins), and
one-warm-CLI-session-per-process (`agentsession.go:349-366`). They share one
root: **`internal/agentsession` configures MCP through `os.Setenv`.** Until
that is fixed, "SparkQuill as a profile" means either a mutex serializing
every user in the server, or a race on process-global credentials.
SparkQuill already holds that global turn lock for minutes at a time
(`chat.go:444-453` records an 8-minute hold and a 207s wait).

This is step 4 of the migration sequence below ("move generic
bridge/session/resume behavior out of the family-only `internal/agentsession`
adapter") — acknowledged there, still not done.

### What to do instead

1. **Set `agentsession.Config.Skills`.** family-server never sets it
   (verified: zero assignments in the package), and instead hand-copies
   embedded skills to disk each boot (`skills.go:39-63`) and instructs the
   model to `cat skills/<name>/SKILL.md` (`chat.go:192-201`). mcpagent
   already projects `SKILL.md` for coding-CLI transports and exposes
   `read_skill` plus an "Available Skills" prompt listing. This deletes
   `skills.go` and gains progressive disclosure. `skills/_shared/*.md` needs
   a home either way — `skillIDPattern` rejects `_shared`.
2. **Fix the `reservedTopLevel` footgun.** `activity.go:59-72` omits
   `_users`, `Workflow`, `pulse`, and `memories`; `archiveStaleActivities()`
   `os.Rename`s any non-reserved top-level dir idle for 7 days into
   `archive/`. A misconfigured `FAMILY_DATA_DIR` would silently relocate the
   main server's `Workflow/` and `pulse/` trees. Live today.
3. **Reconcile the folder-guard docs with the code.**
   `docs/core/folder_guard_system.md:42` states the `_users/` directory
   "(which contains authentication data, OAuth tokens, and session history)
   is **strictly blocked** from all read and write access."
   `agent_go/cmd/server/tool_setup.go:556` and `:753` both set
   `protectedFolders := []string{}` with the comment "No protected folders —
   all users share the same filesystem", which makes the `isPathProtected`
   checks at `:598` and `:660` inert. A grep for an explicit `_users` block
   elsewhere in Go finds none; the only cross-user rejection found is in
   `workspace/handlers/query.go:43`, which covers document/query access, not
   the shell folder guard. **Whether any layer actually enforces the
   documented guarantee was not established** — resolve it in one direction
   or the other, because a reviewer trusting this doc would approve an
   unsafe change. Independent of the SparkQuill question.
4. **Add characterization tests** for parent chat, child chat, handoff,
   activity isolation, streaming, and WhatsApp routing — valuable on their
   own merits for a product families use daily, and the precondition for any
   future extraction.
5. **Do not consolidate voice yet.** The main server's `pkg/voicestt` looks
   stronger on paper (per-connection streams, JWT auth, capability gating,
   and `RuntimeCapabilities.Voice` already wired) versus family-server's
   Apple-Silicon-only stack with no auth on 12 endpoints and one speaker
   server-wide. But `PLAT-120` is `implemented_pending_live_reverify` with no
   confirmed pass on real human speech, and `voicestt` cannot decode audio
   containers, so WhatsApp voice notes have no path. Verify before acting.
   Note the frontend went the other way — `frontend/src/voice/` is an
   explicit port *from* learning-app, so two dictation implementations (356
   vs 958 lines) are now diverging.

### Revisit trigger — status 2026-09-02

Trigger (a) below is now met: `internal/agentsession` no longer writes
`MCP_API_URL` / `MCP_API_TOKEN` / `MCP_BRIDGE_API_URL` / `MCP_BRIDGE_BINARY`
into the process environment. Its one shared executor is handed to every
agent as explicit `mcpagent` configuration (`MCPRuntimeConfig.APIBaseURL`,
`APIToken`, `BridgeAPIBaseURL`, `CodingRuntimeConfig.BridgeBinary`), and
mcpagent now prefers explicit values over the `MCP_*` variables, so a second
executor in the same process cannot clobber it. Remedy 5 (voice) is also
done: one engine (`pkg/voicestt`) serves both apps, including WhatsApp voice
notes. Remedies 1–4 remain open, and the decision itself is unchanged until
characterization tests exist; see the migration plan agreed the same day
(step 0 done, steps 1–5 pending).

### Revisit trigger

Reopen this decision when **either**: (a) `internal/agentsession` no longer
configures MCP via process-global env, removing the single-tenancy root
cause; or (b) a second product is built against the platform boundary and
independently demonstrates the seams — at which point SparkQuill becomes a
candidate for adoption rather than the specimen the boundary is shaped
around. Absent either, a migration plan is premature regardless of how the
manifest format evolves.

## Evidence (measured 2026-08-02)

This is not an anticipatory generalization. Two measurements make the case.

**Much of the family server is infrastructure-shaped.** `agent_go/cmd/family-server` is
11,020 non-test lines across 54 files. Classifying by filename — crude, but the
files are named for what they do and the split is not close:

```text
infrastructure-named files   7,093   (21 files: whatsapp_bot, migrate, chat,
                                      image_search_tool, pulse, conversation_store,
                                      secrets_store, browser_*, shell_tool,
                                      status_stream, steer, turntrace, voice_*, …)
family-domain-named files    1,928   (10 files: parent_tools, child*, week,
                                      activity, learning_package_tool, materials, …)
remainder                    1,999   (main/composition and unclassified)
```

This is an upper bound on extraction opportunity, not proof that all 7,000 lines
are generic. `pulse.go`, `whatsapp_bot.go`, and the voice files contain both
transport/runtime machinery and Family product behavior. Copying this server
would duplicate them, but extracting them correctly requires separating those
two responsibilities first.

**A second implementation already exists.** The reuse rule below says to extract
only after a second real consumer demonstrates common behavior. `cmd/server`
provides evidence of duplicate capability areas today, although the audit below
shows that each common primitive still has to be proven rather than assumed:

```text
capability   family-server   cmd/server
whatsapp             1,445        3,143
pulse                  521        3,127
secrets                444          702
browser                526          104
```

These measurements prove duplicated capability areas, not interchangeable
semantics. For example, SparkQuill Pulse is a proactive learning check-in while
AgentWorks Pulse is a finding/fix/verification system; SparkQuill voice is
speech-to-text while the cited AgentWorks tools are generation. Extraction must
find the common operational primitive beneath the same-name features rather
than treating line counts as evidence that one implementation can replace the
other.

### SparkQuill feasibility audit (2026-08-02)

The architecture can represent every current SparkQuill capability, but only
because a product is allowed to retain a full native module. SparkQuill cannot
be reduced to a content-only workflow/skill bundle, and the current AgentWorks
Pulse, scheduler, frontend, and connectors cannot simply be plugged into it.

Measured current shape:

```text
family-server backend       11,296 lines / 56 non-test Go files
LearningApp.tsx alone         4,837 lines
family-server HTTP routes        44  (all but one under /api)
family-server direct tests         1  (opt-in; see below)
learning-frontend direct tests     0
```

`go test ./cmd/family-server` now passes rather than reporting `[no test
files]`, but that is one opt-in integration test for the native voice path
(`SPARKQUILL_VOICE_STREAM_TEST=1`), skipped by default. It is not
characterization coverage of parent chat, child chat, handoff, activity
isolation, streaming, or WhatsApp routing — none of which any test exercises.
The SparkQuill release workflow builds the frontend, Go server, Swift voice
helper, and Electron package; it does not exercise their behavior.
Characterization tests remain a precondition for extraction, not a cleanup
task after it.

The audited ownership split is:

| Current SparkQuill capability | Correct target ownership |
|---|---|
| `mcpagent`, provider selection, resume, steering | Shared agent runtime |
| Browser executor, sandbox, diff patch | Shared platform; already partly reused |
| Parent/child roles, handoff, PIN boundary | Family product |
| Activities, materials, reports, teaching modes | Family product |
| Family teaching skills | Versioned Family experience assets |
| Proactive learning/site/memory check-ins called Pulse | Family automation plan on shared automation runtime |
| Finding/fix/verification Pulse | Optional shared quality-review service; not a replacement for Family check-ins |
| Pulse cadence and background execution | Shared execution scheduler |
| School/tuition/sports weekly calendar | Family domain data, not an execution schedule |
| WhatsApp pairing, session, media, delivery | Shared connector transport |
| `@parent`/`@child`, self-chat, activity ingestion | Family routing and policy |
| Desktop/WhatsApp notification mechanics | Shared delivery interfaces |
| Hugging Face backup target and Family summary policy | Family adapters and policy |
| Voice transcription (native Swift/CoreML; MLX/Python for WhatsApp notes) | Family native capability initially |
| Conversation/event transport | Shared only after a normalized product-facing contract exists |
| SparkQuill navigation, activity viewer, academic map | Fully custom Family frontend |

Four boundaries are load-bearing:

1. **Pulse is two services, not one.** A proactive automation service runs
   scheduled domain check-ins and writes results into the product experience. A
   quality-review service owns findings, fixes, verification, and recurrence. A
   product may use either or both.
2. **Scheduling executes work; it does not own domain calendars.** The platform
   owns triggers, locking, retries, recovery, and run history. Family owns a
   child's weekly commitments; Social owns a content calendar; Trading owns
   market-session rules.
3. **Connector transport is shared; conversation meaning is not.** Pairing,
   inbound normalization, attachments, delivery, retries, and receipts belong
   to the platform. Product routing, permissions, and side effects stay local.
4. **Frontend reuse starts below the visual product.** SparkQuill currently uses
   a small `status|delta|tool_call` SSE shape while AgentWorks uses a much larger
   generated event model. Share a normalized protocol, API client, state
   machines, and React hooks first. Keep the SparkQuill UI fully custom.

This yields a precise confidence statement:

- **Functional fit:** yes; no current SparkQuill feature falls outside the
  platform-plus-native-product model.
- **Configuration-only fit:** no; voice, activity isolation, Family routing,
  domain storage, and custom UI require native product code.
- **Reuse without refactoring:** no; several current implementations have
  incompatible contracts or mixed platform and product responsibilities.
- **Safe incremental migration:** yes, after behavior is pinned with tests and
  each shared seam is extracted independently.

## Architecture

```text
                               mcpagent
                  agent execution, tools, skills, sessions
                                   |
                    Shared AgentWorks Platform
       backend runtime + operations + connectors + frontend runtime
                                   |
             +---------------------+---------------------+
             |                     |                     |
        Family product       Social product        Trading product
        own domain/API       own domain/API        own domain/API
        own data and UI      own data and UI       own data and UI
```

The dependency direction is strict:

```text
products -> platform -> mcpagent
```

- The platform never imports a product.
- Products never import one another.
- `mcpagent` remains product- and transport-agnostic.
- A product may use all shared services, only some of them, or provide a
  product-specific adapter where its requirements genuinely differ.

## Shared backend platform

### 1. Agent runtime

The shared runtime owns:

- `mcpagent` construction and lifecycle;
- model-provider configuration;
- tools and skills;
- sessions, continuation handles, resume, and steering;
- streaming and structured events;
- background-agent execution;
- tool-error normalization;
- output truncation and full-output artifact retention;
- shared MCP bridge lifecycle.

Products provide prompts, tool implementations, skill bundles, permission
intent, and domain-specific agent roles. They do not rebuild session or bridge
machinery.

### 2. Proactive automation and quality review

The platform exposes two composable services rather than one universal Pulse.

The **proactive automation service** owns:

- scheduled and manual domain check-in execution;
- locking, deferral, cancellation, and recovery;
- ordered check plans and per-check status;
- delivery of results into the product's conversation or dashboard;
- invocation of the shared post-run pipeline when configured.

SparkQuill's current Pulse is a Family-owned plan on this service: review
learning activity, check saved sites, update preferences/interests, back up, and
send a parent summary in the single parent conversation.

The **quality-review service** owns:

- review module lifecycle;
- findings, deduplication, status transitions, fixes, and verification;
- human decisions and approval states;
- backlog and recurrence handling;
- review and fix audit history;
- dashboard projections;
- final-command state and recovery.

Products may contribute domain evidence and quality modules. For example:

- Social may contribute account health, audience strategy, engagement quality,
  and platform-policy reviews.
- Trading may contribute data freshness, execution quality, exposure, risk, and
  strategy-drift reviews.
- Family may contribute learning progress, content quality, safety, and parent
  follow-through reviews.

The finding lifecycle and UI remain shared when a product opts into quality
review, even though the meaning of a finding is product-specific. A proactive
check-in does not have to manufacture findings merely to use the automation
runtime.

### 3. Scheduling

The shared scheduler owns:

- cron and manual triggers;
- next-run calculation;
- concurrency and locking;
- retries and timeout handling;
- missed-run reconciliation;
- run status and history;
- asynchronous worker lifecycle.

Products register jobs and their business behavior. They do not implement
another scheduling engine.

This service schedules execution only. Product calendars remain product data:
SparkQuill's school/tuition/sports week, a Social content calendar, and Trading
market-session rules are not platform scheduler records unless they actually
trigger executable work.

### 4. Communication connectors

WhatsApp, Slack, email, and future channels are shared transports. The platform
owns:

- authentication and connection state;
- inbound message and attachment normalization;
- outbound delivery;
- retries, rate limits, and delivery receipts;
- channel health and diagnostics;
- secret-safe credential handling.

Products own message interpretation and routing. A WhatsApp message may be a
parent request in Family, a campaign approval in Social, or a risk alert in
Trading; that meaning must not leak into the connector package.

### 5. Post-run operations

Backup, publish, and notify form a reusable post-run pipeline:

```text
work completed -> backup -> publish -> notify
```

This pipeline must not be inseparably coupled to Pulse. Pulse, a scheduled job,
or a manual operation may invoke it. The platform owns ordering, status,
recovery, and truthful partial failure. Products provide policy and adapters:

- which artifacts are backed up;
- where they are published;
- who receives a notification;
- which actions require approval.

### 6. Other shared services

The platform should also own:

- browser/CDP execution;
- shell sandboxing and folder guards;
- file and artifact handling;
- secrets and credential injection;
- authentication and authorization primitives;
- storage and migration utilities;
- event logging and observability;
- costs, tokens, and usage accounting;
- approvals and durable human input;
- health checks and lifecycle management.

## Shared frontend platform

The frontend platform owns headless execution state and optional reusable UI
primitives. It does not mandate one application shell, navigation model, design
system, or event-detail density for every product. Shared areas include:

- chat messages and composers;
- streaming assistant text;
- tool-call arguments, results, and failures;
- background-agent and workflow-run status;
- steering and cancellation;
- uploads and attachments;
- Pulse findings, reviews, fixes, and verification;
- schedules and run history;
- connector settings and delivery state;
- human-decision cards;
- shared navigation, typography, colors, dialogs, and accessibility behavior.

The shared event contract should include at least:

```text
message_started
message_delta
message_completed
message_failed
tool_started
tool_completed
tool_failed
status_changed
human_input_required
run_completed
run_failed
```

Products may introduce domain events, but they should render through explicit
product-owned components rather than changing the meaning of core events.

The listed contract is a target normalization layer, not the current wire
format. SparkQuill's `status|delta|tool_call` SSE stream and AgentWorks' generated
event inventory need adapters into this contract before components are shared.
The first extraction should therefore be API clients, reducers/state machines,
and React hooks; visual components are optional consumers.

In the current AgentWorks frontend, `ProductChatSurface` is the canonical
consumer of that normalized conversation contract. `ChatArea` installs it
automatically whenever a product selects `inputVariant="product"`. Its adapter
maps the existing `agent_error`, `conversation_error`, failed completion, and
cancel events into one `message_failed` state with a stable code, safe user
copy, retryability, optional retry time, and collapsed technical details. A
product may replace the visual renderer, but it must consume this shared state
rather than parsing provider strings or inventing a product-local error event.

Suggested frontend structure:

```text
frontend/packages/platform-api
frontend/packages/design-system
frontend/packages/chat-runtime
frontend/packages/tool-events
frontend/packages/pulse-ui
frontend/packages/schedules-ui
frontend/packages/connectors-ui
frontend/products/family
frontend/products/social
frontend/products/trading
```

Use compile-time composition initially. Runtime-loaded frontend plugins would
add deployment, compatibility, and debugging complexity before there is a
demonstrated need for them.

## Product application boundary

A dedicated product is a full application, not a data-only manifest. It owns:

- its domain model and rules;
- HTTP endpoints and commands;
- database schema and repositories;
- workers and external integrations;
- prompts, tools, skills, and agents;
- product-specific Pulse modules;
- product-specific screens and components;
- security and approval policy beyond the shared minimum.

A small composition contract is sufficient:

```go
type Services struct {
    Agents        agent.Factory
    Sandbox       sandbox.Factory
    Browser       browser.Service
    Secrets       secrets.Store
    Events        events.Bus
    Scheduler     scheduler.Service
    Automation    automation.Service
    QualityReview qualityreview.Service
    Notifications notifications.Service
    Storage       storage.Factory
}

type Application struct {
    HTTP    http.Handler
    Workers []Worker
    Close   func(context.Context) error
}

func Build(ctx context.Context, services Services) (*Application, error)
```

This is a composition boundary, not a requirement that every product expose the
same features. A product can ignore services it does not need. Domain-specific
APIs stay inside the product.

Avoid a large plugin interface with methods such as `RegisterTools`,
`RegisterSkills`, `RegisterRoutes`, `RegisterJobs`, and dozens more. That would
recreate the public-API problem recently removed from `mcpagent`. Prefer an
immutable service bundle passed to one product constructor.

## Data ownership

The platform owns common operational records:

- runs and sessions;
- schedules;
- costs and usage;
- Pulse reviews, findings, fix attempts, and verification;
- decisions, approvals, and notifications;
- connector and delivery status.

Products own domain records:

- `social_*` for campaigns, accounts, posts, audiences, and attribution;
- `trading_*` for instruments, market observations, positions, orders, and
  risk decisions;
- `family_*` for profiles, activities, materials, progress, and reports.

Each product supplies versioned migrations for its own schema. Agents should use
registered query and mutation tools instead of unrestricted direct SQLite shell
access. Shared operational tables must not accumulate product-specific columns.

## Deployment modes

The same architecture supports two deployments:

### One AgentWorks application with several products

AgentWorks composes all enabled products into one control plane. This is best
when one operator manages several kinds of agent work.

### Dedicated branded applications

A dedicated executable composes the shared platform with one product:

```go
func main() {
    host := platform.New(config)
    app, err := social.Build(context.Background(), host.Services())
    if err != nil {
        log.Fatal(err)
    }
    host.Run(app)
}
```

The executable should remain thin. A separate app is justified by a different
user experience, trust boundary, permission model, release lifecycle, or
always-on service topology—not merely by different prompts or workflows.

## Reuse rule

Share infrastructure; keep business meaning local.

| Shared platform | Product-owned |
|---|---|
| Deliver a WhatsApp message | Decide what the message means |
| Run and resume an agent | Define the agent's domain job |
| Schedule and recover a job | Define what the job does |
| Record a Pulse finding | Decide which evidence is a domain problem |
| Execute a browser tool safely | Define the permitted business action |
| Store and inject a secret | Define which credential a product requires |
| Stream a tool result | Render a product-specific result card when needed |
| Back up and notify | Choose artifacts, destination, audience, and policy |

Extract a shared abstraction after a second real consumer demonstrates the
common behavior. Do not generalize a Family-only, Social-only, or Trading-only
concept in anticipation of reuse.

For the capability areas named in Evidence, `cmd/server` and `cmd/family-server`
justify investigating extraction. A service enters the platform only after its
common contract is demonstrated; similar names and line counts are not enough.
For anything else, wait for the second consumer.

## Enforcement

Three rules in this document are currently stated as prose. Prose holds until
the first deadline. Each needs a mechanism that fails loudly, because all three
share a failure mode: nothing errors, the system just quietly stops being what
the document says it is.

**The event contract must be pinned, not listed.** Products ship on separate
release lifecycles, so a platform that renames or repurposes `tool_failed` finds
out from a user, not a test. Pin the exact event-name inventory with an AST or
schema golden test — the same ratchet used for the `mcpagent` public surface,
which pins sorted names rather than a count so a deleted event cannot be
silently replaced by a different one. This project has already paid for the
alternative: `docs/refactor/lazy_per_terminal_event_loading.md` documents a Go
list and a TypeScript list that must agree, held together by two comments, where
a drifted copy does not error — it silently drops events from a transcript.

**"Shared operational tables must not accumulate product-specific columns" needs
a schema assertion.** A migration-time check that rejects unknown columns on
platform-owned tables converts the rule from a convention into a property. As
written it is enforced only by review attention.

**The conformance suite needs an owner and an entry point.** "Every product must
run the same platform conformance suite" is a wish unless the suite is an
importable package a product's CI executes, failing the product build when a
contract regresses. Name the package. Given that unit tests over agent behavior
count for little here, at least the bridge, streaming, and tool-failure cases
should run against a real coding agent rather than a mock.

### Two smaller notes

`Services` correctly answers the `mcpagent` lesson: an immutable bundle passed
once, not a mutable registration surface invoked in an order the caller must get
right. The failure mode it remains exposed to is *growth* — nine fields become
twenty, and every product carries services it never uses. Add an admission rule:
a service earns a slot when two products need it, and is removed when one does.
That is the same reuse rule applied to the composition boundary itself.

The runtime section lists "tool-error normalization." Name the contract
explicitly, because its entire value is that one command works everywhere:
`[TOOL_ERROR]` for reported failures, `[TOOL_ERROR_SUSPECT]` for reported
successes whose payload reads like a failure, both carrying `layer=`, tool,
session, args, and result, so `grep '\[TOOL_ERROR'` covers every product,
provider, and transport. If products are allowed to invent their own error
logging, cross-product operability is lost on the first one that does.

## Testing contract

Every product must run the same platform conformance suite:

- agent construction and prompt/skill visibility;
- MCP bridge tool discovery and invocation;
- streaming, steering, completion, and resume;
- normalized message failure, retry, and safe technical-detail rendering;
- tool-success and tool-failure rendering;
- large-output truncation plus full-artifact retention;
- folder-guard and secret boundaries;
- schedule execution and recovery;
- proactive automation execution, deferral, and recovery;
- quality-review finding/fix/verification lifecycle for products that enable it;
- backup/publish/notify partial-failure behavior;
- frontend event compatibility.

Products add domain tests for their own behavior. At least one real coding-agent
E2E should exercise the same bridge and event path used in production.

## Migration from the current family server

1. Add characterization tests for parent chat, child chat, handoff, activity
   isolation, streaming, WhatsApp routing, proactive Pulse, and packaging.
2. Freeze the first contracts being extracted for the duration of each slice.
3. Extract shared shell execution and large-output handling.
4. Move generic bridge/session/resume behavior out of the family-only
   `internal/agentsession` adapter and into the appropriate shared runtime.
5. Replace the family browser HTTP shim with a reusable in-process browser
   execution adapter.
6. Extract connector transport while retaining Family routing and media policy.
7. Extract the execution scheduler and post-run pipeline; keep Family calendars
   and backup targets product-owned.
8. Separate proactive automation from quality review rather than replacing
   SparkQuill Pulse with AgentWorks Pulse.
9. Normalize conversation events, then extract headless frontend clients,
   reducers, and hooks before any visual components.
10. Convert Family into the first product using the platform boundary while
    preserving its custom frontend and native voice/activity capabilities.
11. Build the second product against the platform boundary and adjust only
   abstractions proven insufficient by that real implementation.
12. Build Trading last, after the platform has survived two distinct products.

### Which product goes second

The second consumer defines the boundary, so it should be the one that stresses
it honestly at the lowest cost of being wrong. **Voice is not that second
product.** SparkQuill's voice code is on-device speech-to-text — as of
2026-08-02 a native Swift/CoreML helper for live dictation, with the MLX/Python
worker retained only for WhatsApp voice notes (see
`docs/refactor/native_streaming_stt.md`) — while the cited AgentWorks
audio/music tools generate media. They share artifact/process
primitives but not one product capability, so adding their line counts would
repeat the same-name/same-semantics mistake this audit found in Pulse.

Use a real second end-user product. Social is the leading candidate because
existing social workflows and data provide concrete behavior to migrate, while
its actions can begin read-only or approval-gated. The choice should be made
after the first platform slice is defined, using the product that exercises the
most uncertain seam without introducing irreversible risk.

**Trading should be last, and the reason is not sequencing convenience.** A grep
for trading concepts across `agent_go` returns nothing — it is greenfield, so it
supplies no duplication evidence and cannot demonstrate which abstraction is
genuinely shared. More importantly, its failure modes are categorically different
from the other products: real money, latency budgets, regulatory retention, and
irreversible actions. A platform boundary discovered under those constraints is
discovered expensively. Let Family and Social prove the seams first,
then let Trading exercise the approval, audit, and permission boundaries it
actually needs — which is precisely the case this document already makes for when
a dedicated application is justified.

This sequence avoids designing a speculative plugin framework while preventing
new products from copying the current family-server infrastructure.

### Scope realism

The twelve steps above are written primarily around `cmd/family-server`,
which is the smallest of the three masses involved:

```text
frontend/src               141,664 lines
agent_go/cmd/server         89,241 lines
agent_go/cmd/family-server  11,296 lines
```

All three counts exclude test files; an earlier revision compared a
tests-included frontend number (147,006) against a tests-excluded server
number, which is not a like-for-like ratio. Those steps begin with roughly 5%
of the code this architecture ultimately touches. Two questions are load-bearing and currently unanswered:

**What becomes of `cmd/server`?** At 89,241 lines it holds the Pulse, scheduler,
and workflow machinery this document proposes to share, so it is simultaneously
the largest source of platform code and the largest product. "AgentWorks composes
all enabled products" implies it splits into platform plus a workflow product,
but no step describes that split. Converting family-server first is the easy
direction; it proves the boundary on the smaller consumer without proving it can
carry the bigger one.

**Frontend extraction needs its own implementation plan.** Step 9 deliberately
starts with event normalization and headless clients/reducers/hooks, but that is
still only the boundary for a package split of the largest mass in the
repository. Do not interpret it as authorization for a broad component move.
Inventory consumers, pin the normalized contract, extract one state machine at
a time, and keep both visual applications unchanged until each slice passes its
conformance tests.

Neither question changes the architecture, which the evidence supports. They
change the estimate. This is a multi-quarter program, not a restructuring pass,
and the plan should say so before anyone commits to a date.

**The target is moving.** Both servers are under active development —
folder-guard normalization, tool-error instrumentation, event ownership, and the
`mcpagent` public surface all changed within a single day in August 2026.
Extracting shared infrastructure from code that is still changing means the
extraction rebases continuously. Either freeze the interfaces being extracted for
the duration of each step, or accept that steps 1–4 will be redone. Naming which
is the point; discovering it mid-migration is not.

## Designing for ten products, not two

**Stated goal (2026-08-02):** AgentWorks remains the workflow engine; SparkQuill
is the first vertical; the intent is 3-10 more like it. Every one of them will
be genuinely custom, with substantial unique features of its own — none is a
skin over a shared product.

That target changes what "success" means here. The Evidence section above
justifies extraction from a *second* consumer. A tenth consumer is a different
bar: an abstraction that is merely tolerable is paid for nine more times.

### The metric: unique domain as a share of product size

Products being large is not the problem. Products being large *because they
rebuilt the plumbing* is. SparkQuill's backend, classified by filename
(2026-08-02, crude but directional):

```text
mechanism that belongs to a platform   7,260   64%
genuinely Family domain                1,633   14%
main / composition / other             2,403   21%
total                                 11,296
```

A rich, fully custom product turns out to contain roughly **1,633 lines of
genuinely unique backend**. Everything else is WhatsApp, Pulse plumbing,
secrets, browser, shell, streaming, steering, and voice — rebuilt because there
was no platform to inherit them from.

So ten custom products should not cost ten times SparkQuill:

```text
wrong    10 x 17,500 lines  (server + frontend, each product standalone)
right    platform once  +  10 x (~2,000 domain lines + its own screens)
```

**This is the number to hold the migration to.** If a new product approaches
SparkQuill's current size, the platform boundary is in the wrong place. Track
the domain share per product; it should rise toward 100% of what a product
team actually writes, not sit at 14%.

Nothing here argues for thinner or more uniform products. It argues that
"custom" should mean *its own domain*, not *its own copy of the mechanism*.

### The frontend is where this is least true

`LearningApp.tsx` is 4,837 lines of a 6,210-line product frontend — 78% in one
file. That file is not 4,837 lines of unique teaching behavior: it interleaves
streaming, the composer, mic capture, tool cards, SSE subscriptions, scroll
management, and file trees with Family-specific UI. It is the mechanism/meaning
split violated at file level, which is why unrelated changes keep landing in it.

Step 9's ordering (normalize events, then extract clients, reducers, and hooks
before visual components) is right, and "keep the SparkQuill UI fully custom" is
right for SparkQuill. Neither is sufficient at ten products, because it leaves
each new product writing chat mechanics again. The platform additionally needs a
composable **application shell** — chat surface, composer, tool-result rendering,
run status — so a product's UI is hundreds of lines of arrangement and its own
screens, with personalization living in theme, layout, copy, and domain
components rather than in a re-implemented chat surface.

### Shipping is platform surface, and this is already proven

The document covers composition (`Deployment modes`) but not distribution. At
ten products that gap is larger than it looks. Two products today already carry
duplicated shipping surface:

```text
.github/workflows/desktop-release.yml      .github/workflows/sparkquill-desktop.yml
install.sh                                 install-sparkquill.sh
desktop/                                   desktop-sparkquill/
tag namespace  v*                          tag namespace  sparkquill-v*
```

This is not hypothetical risk. On 2026-08-02, publishing the first real
SparkQuill releases silently broke AgentWorks' updater in production: GitHub's
`/releases/latest` returns whichever app shipped most recently regardless of
which app is asking, so AgentWorks began reading a `sparkquill-v*` tag, parsing
it to a garbage version, concluding "not newer", and never reporting its own
updates again. `install.sh` had the same fault, where a *fresh install* would
chase a dmg that does not exist. Both failed silently; neither errored.

Two products produced that with one shared endpoint. Ten products have
forty-five pairs to collide in. Release pipeline, installer, updater, tag
discipline, signing, and icon/branding pipeline should be one parameterized
platform capability with a per-product manifest — not per-product shell scripts
maintained by copy.

### What this implies for sequencing

The reuse rule ("extract after a second real consumer") and the ten-product goal
pull in opposite directions, and the tension should be named rather than
averaged away. Two consumers prove an abstraction is *possible*; they do not
prove it is *right* for the eight after them. The practical resolution is to
keep the rule, but choose the second consumer for how differently it stresses
each seam — and to treat the first two products as still-provisional, budgeting
one deliberate revision of the boundary after product three rather than
discovering the need for it at product six. See also "Open contradictions"
below, which records the sharper problem that the current step order reaches
step 10 with only *one* consumer.

## Open contradictions

Two places where the plan argues against itself. Both matter because the point
of this document is to make the *second* product cheap, not to tidy the first.

**The migration defines the boundary with one consumer, which the reuse rule
forbids.** The rule is explicit: extract a shared abstraction *after* a second
real consumer demonstrates the common behavior. But step 10 converts Family
onto the platform boundary and step 11 builds the second product after it. A
boundary drawn against Family alone will be Family-shaped, and the second
product pays for that — exactly the outcome this document exists to prevent.
"Scope realism" half-concedes this ("proves the boundary on the smaller
consumer without proving it can carry the bigger one") without resolving it.
Either state an explicit first-consumer exemption and accept one rewrite after
product two, or interleave steps 10 and 11 so the first shared seam is proven
against both consumers before it is called a platform.

**Characterization tests are scheduled before the freeze that makes them
stable.** Step 1 writes characterization tests; step 2 freezes the contracts
being extracted. "The target is moving" then warns that steps 1-4 will
otherwise be redone. Tests written against a contract that is still changing
are the first thing invalidated, so the freeze belongs before the tests, per
slice, not after.

## Explicit non-goals

- One universal domain model for every product.
- Runtime loading of arbitrary Go plugins.
- Making every product use every platform feature.
- Putting product-specific rules into `mcpagent`.
- Sharing code merely because two functions currently look similar.
- Copying a dedicated server and allowing the copies to drift.
