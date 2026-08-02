# Reusable Platform for Dedicated Agent Products

**Status:** Proposed architecture  
**Scope:** AgentWorks, SparkQuill/family-server, and future dedicated products such as social-media and trading applications

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

**The second consumer already exists.** The reuse rule below says to extract only
after a second real consumer demonstrates the common behavior. That condition is
already met — not by Social, but by `cmd/server`, which implements the same
capabilities independently today:

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
family-server backend       11,020 lines / 54 non-test Go files
learning frontend            7,224 lines
LearningApp.tsx alone         4,837 lines
family-server HTTP routes        33
embedded Family skills           14
family-server direct tests         0
learning-frontend direct tests     0
```

`go test ./cmd/family-server` compiles, but reports `[no test files]`. The
SparkQuill release workflow builds the frontend, Go server, and Electron
package; it does not exercise their behavior. Characterization tests are a
precondition for extraction, not a cleanup task after it.

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
| Voice/MLX/Parakeet transcription | Family native capability initially |
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
        backend runtime + operations + connectors + frontend shell
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

For the services named in this document that condition is already satisfied by
`cmd/server` and `cmd/family-server` — see Evidence. For anything not on that
list, the rule still binds: wait for the second consumer.

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
product.** SparkQuill's voice code is on-device speech-to-text (MLX, Parakeet,
hardware detection, a persistent Python worker, and settings); the cited
AgentWorks audio/music tools generate media. They share artifact/process
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
discovered expensively. Let Voice and one other product prove the seams first,
then let Trading exercise the approval, audit, and permission boundaries it
actually needs — which is precisely the case this document already makes for when
a dedicated application is justified.

This sequence avoids designing a speculative plugin framework while preventing
new products from copying the current family-server infrastructure.

### Scope realism

The eight steps above are written almost entirely about `cmd/family-server`,
which is the smallest of the three masses involved:

```text
frontend/src           147,006 lines
agent_go/cmd/server     89,225 lines
agent_go/cmd/family-server  11,020 lines
```

Steps 1–6 address roughly 4% of the code this architecture claims. Two questions
are load-bearing and currently unanswered:

**What becomes of `cmd/server`?** At 89,225 lines it holds the Pulse, scheduler,
and workflow machinery this document proposes to share, so it is simultaneously
the largest source of platform code and the largest product. "AgentWorks composes
all enabled products" implies it splits into platform plus a workflow product,
but no step describes that split. Converting family-server first is the easy
direction; it proves the boundary on the smaller consumer without proving it can
carry the bigger one.

**Step 5 is one line for 147,006 lines of frontend.** "Share the frontend chat,
event, schedule, and Pulse packages" describes a package split of the largest
mass in the repository. It needs its own sequence, or it will be the step where
the plan stalls.

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

## Explicit non-goals

- One universal domain model for every product.
- Runtime loading of arbitrary Go plugins.
- Making every product use every platform feature.
- Putting product-specific rules into `mcpagent`.
- Sharing code merely because two functions currently look similar.
- Copying a dedicated server and allowing the copies to drift.
