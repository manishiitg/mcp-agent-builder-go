---
name: seeddance-api
description: Generate Seedance video through the independent Seeddance API at seeddance.io using the deployment's SEEDANCE_API_KEY, separately from fal.ai. Use when choosing between direct Seeddance and fal, checking direct account credits or model access, or submitting, polling, downloading, retrying, and reviewing Seedance 2.0 or 2.5 video tasks. Read with seedance-video, video-provider-capabilities, and video-cinematography before paid generation.
---

# Call the direct Seeddance API safely

This skill covers the service documented at `https://www.seeddance.io/docs`.
It is a separate provider from fal.ai and from ByteDance's own enterprise
platform. Do not send a fal key to it, translate fal endpoint IDs into it, or
assume access and pricing are identical.

Current documentation starting points:

- `https://www.seeddance.io/docs/authentication`
- `https://www.seeddance.io/docs/models`
- `https://www.seeddance.io/docs/createVideoGeneration`
- `https://www.seeddance.io/docs/getTask`
- `https://www.seeddance.io/docs/getCredits`

## Authenticate without exposing the credential

Read the deployment credential only from `SECRET_SEEDANCE_API_KEY`. Send it as
the bearer token required by the `/v1` API. Never print, echo, log, persist,
return, or interpolate its value into a durable command transcript. Do not put
it in a project `.env` file. If the variable is absent, mark the direct provider
unavailable. Fall back to fal only when `SECRET_FAL_KEY` exists and the approved
production plan allows that provider switch.

A read-only `GET https://www.seeddance.io/v1/credits` is the preferred access
check before planning a paid direct run. Record only whether authentication
succeeded and the returned credit count; never record request headers. A valid
key does not guarantee that the account's plan permits every model.

## Discover the model before spending

As checked on 2026-08-17, the public model catalog lists:

```text
seedance-1.0-pro-fast
seedance-1.5-pro
seedance-2.0
seedance-2.0-fast
seedance-2.0-mini
seedance-2.5
```

Re-read the live catalog and create schema before every production. Store the
chosen direct model ID, allowed duration, quality, aspect ratio, media limits,
audio support, price, and plan entitlement in the capability record. Never use
a fal model ID such as `bytedance/seedance-2.5/text-to-video` here.

Current direct documentation describes Seedance 2.5 as accepting 4–30-second
jobs, 480p or 720p quality, up to 30 images, 10 videos, and 10 audio references.
These limits differ from fal's callable schema, which is exactly why provider
capabilities must remain separate.

## Construct exactly one generation request

Submit with `POST https://www.seeddance.io/v1/videos/generations`. Build the
request only from live fields. Current documented fields include `model`,
`prompt`, `duration`, `quality`, `aspect_ratio`, `generate_audio`, `image_urls`,
`video_urls`, `audio_urls`, `output_format`, `content_filter`, `web_search`, and
`reference_mode`.

The service selects mode from the supplied arrays:

- no media: text-to-video;
- one image: image-to-video;
- two images: first and last frame;
- three or more images: reference video generation;
- any video or audio: reference video generation.

Set `reference_mode: true` only when one or two images should be treated as
references instead of boundary frames. Use public HTTPS media URLs accepted by
the current schema; do not send local paths. Validate counts, durations, and
formats locally before the paid request.

Persist the returned `task_id` immediately in the generation ledger. A repeated
POST may create and bill a second job, so never resubmit merely because a client
timed out after creation.

## Poll, recover, and review

Poll `GET https://www.seeddance.io/v1/tasks/{task_id}` every 5–10 seconds with
backoff and jitter. Rejoin the same task after restarts or transient failures.
Respect the documented rate limit. Treat responses deliberately:

- `401`: invalid or missing direct key; stop without exposing it;
- `402`: insufficient credits; stop and report the funding blocker;
- `403`: model not included in the account plan; choose another model only with
  an updated plan;
- `422`: request does not match that model's live schema; correct it before any
  new paid submission;
- `429`: keep the same task and back off when polling; do not duplicate it.

When the task succeeds, download every returned MP4 into the project, verify it
with `ffprobe`, and review identity, motion, audio, prompt adherence, and
continuity boundaries. Call `show_video` once for every distinct clip so the
user can review all results before stitching.

Record the provider (`seeddance-api`), model, sanitized request, `task_id`,
credit change when available, output paths, technical measurements, and review
verdict. Never record authorization headers or credential values.
