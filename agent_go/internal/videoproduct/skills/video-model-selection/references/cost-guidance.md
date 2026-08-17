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
For Google, recheck the official pricing page because preview model rates and
availability can change. For direct Seeddance, the public service bills credits
based on model, duration, resolution, and settings rather than a stable public
USD-per-job table; show the current credit balance and quote the direct
provider's current per-job credit requirement if it exposes one. If it does
not, state that a precise USD estimate is unavailable and offer a bounded
low-cost test rather than fabricating a conversion.
