# Video cost guidance

Use this reference when comparing models or seeking approval for a paid plan.
Rates are snapshots, not promises: record the live source, timestamp, currency,
billing unit, and account-specific result in the capability record.

## Give the user a decision, not a rate dump

Before the first paid call, present at most three viable options. For each,
state the model/provider, the creative reason to choose it, output duration and
resolution, current unit rate, planned successful-output cost, and maximum
approved cost including the retry allowance. Recommend one option and explain
the meaningful tradeoff (continuity, native audio, control, resolution, or
speed). Do not ask users to infer total cost from a per-second rate.

Calculate both numbers:

```text
base = sum(rate × requested output units for every planned successful output)
maximum = base + cost of the explicitly approved quality-retry allowance
```

This calculation is valid only when the provider bills the same unit that the
plan specifies (for example, output seconds or a completed video). Do not
substitute output seconds for **compute seconds**, **tokens**, credits, or GPU
time. Those units have no trustworthy conversion from a requested clip length.
For such a route, show the live unit rate and mark the production total as
**not reliably estimable before a bounded test**. Ask for a small dollar cap
for that test; after it completes, record the billed amount and use that
observed evidence only for a similar next call, not as a universal rate.

Provider server failures and queue time may be unbilled, but a completed clip
the user rejects is a paid quality retry. Keep those two cases separate. Do not
include assembly, QA, download, or local rendering as generation cost.

## Current public anchors — checked 2026-08-17

| Option | Published rate | Useful interpretation |
| --- | --- | --- |
| Google Veo 3.1 Lite | $0.05/s at 720p; $0.08/s at 1080p | Lowest-priced direct Veo route for iteration. |
| Google Veo 3.1 Fast | $0.10/s at 720p; $0.12/s at 1080p; $0.30/s at 4K | Balance speed and output quality. |
| Google Veo 3.1 Standard | $0.40/s at 720p/1080p; $0.60/s at 4K | Use only where its quality is worth the premium. |
| fal Seedance 2.0 Standard | $0.3034/s at 720p with audio | Higher-cost continuity/reference option. |
| fal Seedance 2.0 Fast | $0.2419/s at 720p with audio | Lower-cost Seedance iteration route. |

## Current routing comparison — checked 2026-08-21

Use this only to form a shortlist, then confirm the exact endpoint's live
schema and account-facing price. Do not present the table as an independent
quality benchmark: fal hosts the models and publishes the cited comparisons.

| Route | Current measurable advantage | Current measurable limitation | Published 720p audio rate |
| --- | --- | --- | --- |
| fal Veo 3.1 Lite image-to-video | Lowest-cost synchronized-audio starting point; 4s, 6s, or 8s at 720p or 1080p | Eight-second maximum and a simpler reference/control surface than the continuity-heavy routes | $0.05/s at 720p |
| fal Kling V3 Standard | Structured multi-shot generation and native audio at a lower Kling tier | Confirm that Standard has every element/reference control the shot needs | $0.126/s |
| fal Kling V3 Pro image-to-video | Up to 15 seconds, custom elements, structured multi-shot prompts, 1080p, native audio and optional voice control | Costs more than Lite; Pro is unjustified when Standard or Lite meets the shot | $0.168/s; $0.196/s with voice control |
| fal Seedance 2.5 reference-to-video | One continuous 4--30 second take; up to 30 images, 10 videos and 10 audio references; synchronized audio | 480p/720p only and expensive for ordinary short shots | about $0.4730/s at 720p; token formula is authoritative |

Practical selection rule:

- Start with Veo Lite for an ordinary short shot when its controls are enough.
- Prefer Kling Standard/Pro when structured cuts, character/object elements,
  voice control, 1080p, or a 15-second take provide a concrete benefit.
- Prefer Seedance 2.5 when its 30-second continuous generation or mixed
  reference surface replaces multiple independent calls and seam risk.
- Compare the total planned production and retry cost, not only the nominal
  per-second rate. A more expensive continuous take can still be economical
  when it replaces several uncertain shots, but write that arithmetic down.

Official sources used for this snapshot:

- `https://fal.ai/models/fal-ai/veo3.1/lite/image-to-video`
- `https://fal.ai/models/fal-ai/kling-video/v3/standard/text-to-video`
- `https://fal.ai/models/fal-ai/kling-video/v3/pro/image-to-video`
- `https://fal.ai/models/bytedance/seedance-2.5/reference-to-video`
- `https://fal.ai/learn/tools/seedance-2-0-vs-kling-3-0`
- `https://fal.ai/learn/tools/seedance-2-0-vs-veo-3-1`
- `https://fal.ai/learn/devs/seedance-2-5-vs-seedance-2-0`

For example, 15 seconds of Seedance 2.0 Standard is about $4.55 and Fast is
about $3.63 before any quality retry. Thirty seconds of Veo 3.1 Lite at 720p is
about $1.50; Fast is about $3.00; Standard is about $12.00. These examples are
only math from the dated rates above, not estimates for a whole production.

## Live pricing is authoritative

For fal, retrieve pricing for every candidate endpoint before planning:

```bash
curl --fail-with-body --silent --show-error \
  "https://api.fal.ai/v1/models/pricing?endpoint_id=<comma-separated-endpoint-ids>" \
  -H "Authorization: Key $FAL_KEY"
```

The response reports `unit_price`, `unit`, and `currency`; account discounts
or custom terms apply there. Never print the authorization header or the key.
Treat `unit` as a contract, not a label: `seconds` supports output-duration
math only when the endpoint documentation confirms they are output seconds;
`compute seconds` and `1000 tokens` do not. Never turn either into a
per-video-second price by assumption.
For Google, recheck the official pricing page because preview model rates and
availability can change. For direct Seeddance, the public service bills credits
based on model, duration, resolution, and settings rather than a stable public
USD-per-job table; show the current credit balance and quote the direct
provider's current per-job credit requirement if it exposes one. If it does
not, state that a precise USD estimate is unavailable and offer a bounded
low-cost test rather than fabricating a conversion.
