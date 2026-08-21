# Video model capability snapshot

Use this reference to form a shortlist and catch impossible plans before they
reach the user. It is a dated discovery aid, not an API contract. For the exact
selected provider, endpoint, mode, and tier, re-open the official schema and
copy its allowed values into the production capability record.

## Verified fal routes — checked 2026-08-21

| Exact route | Duration accepted | Resolution | Input/control surface | Native audio notes |
| --- | --- | --- | --- | --- |
| `fal-ai/veo3.1/lite/image-to-video` | `4s`, `6s`, `8s` | 720p, 1080p | One starting image; `auto`, 16:9, or 9:16; negative prompt and seed | Optional; verify dialogue and lip-sync in the returned clip |
| `fal-ai/veo3.1/image-to-video` | `4s`, `6s`, `8s` | 720p, 1080p, 4K | One starting image; `auto`, 16:9, or 9:16; negative prompt and seed | Optional; exact audio price depends on tier/resolution |
| `fal-ai/kling-video/v3/standard/image-to-video` | integer 3–15 seconds on the V3 request | Verify live tier output | Start/end images, structured `multi_prompt`, elements, intelligent/custom shot type | Optional; current schema documents Chinese/English voice output and translation behavior for other languages |
| `fal-ai/kling-video/v3/pro/image-to-video` | integer 3–15 seconds | Verify live tier output | Start/end images, structured `multi_prompt`, custom image/video elements and voice controls where supported | Optional; voice binding is route- and element-type-dependent |
| `bytedance/seedance-2.5/image-to-video` | `auto` or integer 4–30 seconds | 480p, 720p | Start image and optional end image; aspect follows input | Optional synchronized audio; same token rate with audio on or off |
| `bytedance/seedance-2.5/reference-to-video` | `auto` or integer 4–30 seconds | 480p, 720p | Up to 30 images, 10 videos and 10 audio files, 50 total; modality duration limits apply | Optional synchronized audio; reference audio is separate from generated audio |
| `bytedance/seedance-2.0/image-to-video` | `auto` or integer 4–15 seconds | 480p, 720p, 1080p, 4K | Start image and optional end image; broader aspect-ratio choices | Optional synchronized audio; check Standard versus Fast separately |

## Planning invariants

- Model family names are not capabilities. Resolve the exact endpoint first.
- Treat duration as an enum, not an arbitrary target. Build the shot list from
  supported values before estimating calls or cost.
- Do not transfer resolution, language, reference, audio, or duration claims
  between Lite, Fast, Standard, Pro, text, image, reference, and extension
  routes.
- If a route is missing from this table—including MiniMax H3, Gemini Omni,
  direct Google, or direct Seeddance—say that its limits are not verified in
  this snapshot and inspect its live official schema before quoting them.
- When the provider page and callable schema disagree, the callable schema is
  the execution contract. Record the discrepancy rather than choosing the more
  attractive value.

## Official sources

- `https://fal.ai/models/fal-ai/veo3.1/lite/image-to-video/api`
- `https://fal.ai/models/fal-ai/veo3.1/image-to-video/api`
- `https://fal.ai/models/fal-ai/kling-video/v3/standard/image-to-video/api`
- `https://fal.ai/models/fal-ai/kling-video/v3/pro/image-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.5/image-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.5/reference-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.0/image-to-video/api`
