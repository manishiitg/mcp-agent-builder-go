# Execution, retry, and review

## Preflight

Before spending:

1. Confirm the capability record still matches the chosen endpoint.
2. Confirm all local media exists and is the expected type.
3. Validate required fields, enums, ranges, role counts, reference order,
   mutually exclusive fields, and mode-specific requirements.
4. Estimate cost when supported and compare paid-call count plus retry allowance
   with the approved budget.
5. Upload each reusable asset once and retain its provider handle.
6. Save the exact request and reference manifest before submission.

Do not rely on a provider silently clamping an invalid duration, ratio, or
resolution. If an adjustment changes creative intent or continuity, stop and
revise the plan rather than accepting it as equivalent.

## Durable job lifecycle

Record the provider job ID immediately. Distinguish:

- local wait timeout: reconnect to the same job;
- provider validation failure: fix the documented field and resubmit once;
- rate limit/transient failure: back off and rejoin or retry within budget;
- terminal generation/safety failure: report the reason and request a creative
  change when needed;
- completed job with bad footage: quality failure, not transport failure.

Never create a duplicate paid job merely because polling, the shell, or the
browser failed. Retry only the failed scene/unit and preserve successful work.
Record every attempt, changed field, reason, and cost.

## Output verification

For each result:

1. Download to a durable project-relative path.
2. Run `ffprobe` and confirm playable streams, duration, dimensions, frame rate,
   codec, and expected audio.
3. Inspect frames and motion against the request, approved references, and
   continuity state—not from memory.
4. Check identity, wardrobe/objects, location, temporal coherence, motion,
   artifacts, lip sync, audio, and boundary state.
5. Call `show_video` with presentation type `Preview` for that individual clip.
6. Record presentation ID and `pending_user_review`, `approved`, `rejected`, or
   `approved_with_notes`.

Do not assemble or mark final any clip that has not been presented and
approved. Do not make a stitched video the user's first opportunity to inspect
the paid components.

## Derived assets

Create thumbnails or social covers only after the truthful topic and visual
direction are known. Prefer a real compelling frame from the approved video;
otherwise generate a truthful concept from approved character/product
references. Keep generated artwork text-free and apply short headline text as
a deterministic overlay when possible. Inspect identity, spelling, watermark,
truthfulness, and readability at small size, then present all passing variants.
Thumbnail creation is a separate derivative workflow and never substitutes for
reviewing the underlying video clips.
