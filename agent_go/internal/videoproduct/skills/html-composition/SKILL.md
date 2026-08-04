---
name: html-composition
description: Design and render video frames as HTML/CSS, then assemble them into a video with headless Chrome and ffmpeg. Use when building an infographic, product explainer, stat card, title sequence, or any piece whose visuals are laid-out typography, charts, and shapes rather than photographic footage.
---

# Compose video frames in HTML

For laid-out visuals — type, charts, product cards, stat panels — HTML and CSS are a better authoring surface than ffmpeg filters. You get real typography, flexbox/grid layout, gradients, shadows, and SVG, then screenshot the result and let ffmpeg handle time.

The rule this skill exists to enforce: **HTML owns the look, ffmpeg owns the timeline.** Do not try to lay out a panel with `drawtext`, and do not try to sequence scenes in CSS.

## Canvas discipline

- Set the page to the exact output size. For 9:16 vertical that is `body { width:1080px; height:1920px; margin:0 }`; render at that viewport so no scaling is needed later.
- Use `box-sizing: border-box` globally — padding must not silently push content past the canvas.
- Keep every element inside a safe area: at least 120px from the top and 220px from the bottom on a 1920px canvas. Platform UI covers those bands.
- Never depend on a webfont being installed. Use a stack that degrades honestly (`-apple-system, Helvetica, Arial, sans-serif`), or embed the font as a data URI.
- One idea per frame. A panel that needs a scrollbar is two panels.

## Render frames with headless Chrome

Screenshot through Playwright's Chromium with an explicit viewport, from `execute_shell_command`:

```python
from playwright.sync_api import sync_playwright
with sync_playwright() as p:
    b = p.chromium.launch()
    page = b.new_page(viewport={'width':1080,'height':1920})
    page.goto('file:///abs/path/panel.html')
    page.screenshot(path='panel.png')
    b.close()
```

- Always pass an absolute `file://` path; a relative one silently loads nothing and screenshots a blank page.
- Set the viewport explicitly rather than relying on a default — the default is 1280x720 and will letterbox or crop your canvas without erroring.
- After each screenshot, verify the PNG exists and is not near-zero bytes. A blank page still writes a valid file, so file existence alone is not evidence the panel rendered.
- When type must not shift between frames, render once and animate with ffmpeg rather than screenshotting each frame — re-layout can nudge glyphs by a pixel and read as jitter.

## Turn stills into motion

Give a still frame time and movement with ffmpeg:

- **Hold:** `-loop 1 -i panel.png -t <seconds>`
- **Slow push-in (Ken Burns):** `zoompan=z='min(zoom+0.0008,1.15)':d=<frames>:s=1080x1920:fps=30`
- **Cross-dissolve between panels:** `xfade=transition=fade:duration=0.4:offset=<seconds>`
- **Always finish with** `-pix_fmt yuv420p -c:v libx264` so the result plays everywhere.

Keep motion slow and singular. A panel that pushes in *and* slides *and* fades reads as noise; pick one.

For per-frame animation (a counter ticking, a bar growing), render a numbered sequence — `frame_0001.png` … — driving the value from a variable, then `ffmpeg -framerate 30 -i frame_%04d.png`. Reach for this only when a CSS/ffmpeg move cannot express the idea; it is far slower than animating one still.

## Where your files go

- **Running as a workflow stage:** HTML, PNGs, and rendered clips all live inside your own step folder under `runs/<iteration>/<group>/execution/<stage>/`. Record the exact paths in your stage's artifact so the next stage resolves them without guessing.
- **Working directly in chat:** intermediates under `work/`, finished videos under `outputs/`.

Keep the `.html` next to what it produced. A panel that can be re-rendered from its source is fixable; a stray PNG is not.

## Quality gate

- Every panel's PNG exists, is non-trivial in size, and was visually confirmed — not assumed from a zero exit code.
- Rendered dimensions match the target exactly (`ffprobe`), with no letterboxing or unintended scaling.
- No text sits inside the reserved top/bottom bands.
- Nothing is clipped at a canvas edge, and no scrollbar appears in any screenshot.
- Total duration matches the plan; each panel is on screen long enough to read (roughly 1s per 8–10 words).

## Common pitfalls

- Screenshotting a relative path, getting a blank page, and reporting success because a file was written.
- Leaving the default viewport, so the canvas is cropped or letterboxed without any error.
- Assuming a font is present; the fallback silently changes every line length and breaks the layout.
- Stacking three simultaneous motions on one panel and calling it polish.
- Re-screenshotting per frame when one still plus a filter would have been identical and far faster.
- Text placed in the bottom band, which the platform's own UI then covers.
