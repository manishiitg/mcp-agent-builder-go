# Lightweight Pulse journal skeleton

Use this only when creating `builder/improve.html` or migrating an older Pulse
report. The saved file is a short published executive journal. The Pulse popup
and SQLite own issue lists, evidence, verification, costs, diagnostics, and
filtering.

Copy the structure and CSS, fill real values, and omit these instructions and
example comments from the saved file. Keep exactly one
`<!-- LOG ENTRIES: newest first -->` anchor. Keep Activity concise through
editorial judgment: retain important active history, avoid duplicate standing
state, and archive only genuinely safe resolved history by month. Never omit or
fail the dashboard just to hit an item count. Do not impose a byte, character,
or token budget.

```html
<!doctype html>
<html lang="en" data-theme="dark" class="dark" data-pulse-schema="5">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title><!-- WORKFLOW NAME --> · Pulse</title>
<style>
  :root{color-scheme:dark;--bg:#090b0f;--surface:#11151b;--surface2:#171c24;--line:#27303b;--ink:#eef2f7;--muted:#98a3b3;--ok:#52d39a;--warn:#efbd63;--bad:#f27983;--goal:#73a8ff;--accent:#65d6df;--r:14px;--sans:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;--mono:"SFMono-Regular",Consolas,monospace}
  *{box-sizing:border-box}html,body{margin:0;min-height:100%;max-width:100%;overflow-x:hidden;background:var(--bg);color:var(--ink);font-family:var(--sans)}
  body{font-size:14px;line-height:1.5}.wrap{width:min(820px,100%);margin:0 auto;padding:22px 16px 72px}
  h1,h2,p{margin:0}h1{font-size:26px;line-height:1.1;letter-spacing:-.025em}.eyebrow,.when,.as,.meta,.daydate{font-family:var(--mono);color:var(--muted)}
  .eyebrow{font-size:10px;letter-spacing:.12em;text-transform:uppercase;margin-bottom:7px}.top{display:flex;flex-direction:column;gap:14px}.verdicts{display:flex;flex-wrap:wrap;gap:8px}
  .pill{display:inline-flex;align-items:center;gap:7px;border:1px solid var(--line);border-radius:999px;padding:7px 10px;background:var(--surface);font-weight:650}.pill .lbl{color:var(--muted);font-weight:550}.pill .dot{width:7px;height:7px;border-radius:50%;background:currentColor}.pill.ok{color:var(--ok)}.pill.warn{color:var(--warn)}.pill.bad{color:var(--bad)}.pill .as{font-size:10px}
  .status{display:flex;flex-direction:column;gap:4px;margin-top:16px;padding:14px 15px;border:1px solid var(--line);border-left:3px solid var(--ok);border-radius:var(--r);background:var(--surface);font-weight:650}.status.warn{border-left-color:var(--warn)}.status.bad{border-left-color:var(--bad)}.status .when{font-size:10.5px;font-weight:500}
  .brief{margin-top:14px;border:1px solid var(--line);border-radius:var(--r);background:var(--surface);padding:13px}.sectionhead{display:flex;justify-content:space-between;gap:12px;margin-bottom:10px;font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}
  .briefgrid{display:grid;grid-template-columns:1fr;gap:8px}.briefitem{min-width:0;padding:11px;border-radius:11px;background:var(--surface2);border:1px solid transparent}.briefitem.ok{border-color:color-mix(in srgb,var(--ok) 28%,transparent)}.briefitem.warn{border-color:color-mix(in srgb,var(--warn) 32%,transparent)}.briefitem.bad{border-color:color-mix(in srgb,var(--bad) 32%,transparent)}.briefitem .k{font-size:10px;font-weight:750;letter-spacing:.08em;text-transform:uppercase;color:var(--muted);margin-bottom:5px}.briefitem p{overflow-wrap:anywhere}
  .activity{margin-top:24px}.activity>h2,.archive>h2{font-size:12px;letter-spacing:.1em;text-transform:uppercase;color:var(--muted);margin-bottom:10px}.daygroup{margin-bottom:14px}.daydate{font-size:10px;margin:0 2px 6px}.run,.entry{position:relative;margin-bottom:8px;border:1px solid var(--line);border-radius:12px;background:var(--surface);padding:12px 13px;min-width:0}.run{display:grid;grid-template-columns:1fr auto;gap:5px 10px}.run .note{grid-column:1/-1;color:var(--muted);overflow-wrap:anywhere}.run .st{font-weight:650}.run .st.ok{color:var(--ok)}.run .st.warn{color:var(--warn)}.run .st.bad{color:var(--bad)}
  .entry{padding-left:17px}.entry::before{content:"";position:absolute;left:0;top:11px;bottom:11px;width:3px;border-radius:3px;background:var(--accent)}.entry.decision::before{background:var(--goal)}.entry.fix::before{background:var(--ok)}.ehead{display:flex;align-items:center;gap:7px;flex-wrap:wrap;margin-bottom:7px}.tag,.kind,.worklabel{white-space:nowrap;border-radius:999px;padding:3px 7px;font:700 9px/1 var(--mono);letter-spacing:.05em;text-transform:uppercase;background:var(--surface2);color:var(--muted)}.kind.bug{color:var(--bad)}.kind.goal{color:var(--goal)}.etitle{flex:1 1 100%;min-width:0;font-size:14px;line-height:1.3}.ehead>.when{flex-basis:100%;font-size:10px}.entry p{overflow-wrap:anywhere}.entry p+p{margin-top:6px}.entry .takeaway{font-weight:620}.entry .impact,.entry .meta{color:var(--muted)}
  .archive{margin-top:24px}.arow{display:flex;justify-content:space-between;gap:12px;padding:11px 12px;border:1px solid var(--line);border-radius:10px;background:var(--surface);color:var(--ink);text-decoration:none}.arow .n{color:var(--muted);font-size:11px}
  footer{margin-top:30px;padding-top:14px;border-top:1px solid var(--line);color:var(--muted);font:10.5px/1.5 var(--mono)}
  @media (min-width:640px){.wrap{padding:30px 26px 88px}.top{flex-direction:row;justify-content:space-between;align-items:flex-start}.status{flex-direction:row;align-items:center}.status .when{margin-left:auto}.briefgrid{grid-template-columns:repeat(3,minmax(0,1fr))}.etitle{flex-basis:auto}.ehead>.when{flex-basis:auto;margin-left:auto}}
</style>
</head>
<body><main class="wrap">
  <header class="top">
    <div><div class="eyebrow">workflow · pulse</div><h1><!-- WORKFLOW NAME --></h1></div>
    <div class="verdicts">
      <div id="pulse-bug-verdict" class="pill warn"><span class="lbl">Bug</span><span class="dot"></span>Not measured<span class="as">no run yet</span></div>
      <div id="pulse-goal-verdict" class="pill warn"><span class="lbl">Goal</span><span class="dot"></span>Not measured<span class="as">no run yet</span></div>
    </div>
  </header>

  <div class="status warn"><span><!-- One plain-language verdict sentence. --></span><span class="when"><!-- run/date freshness --></span></div>

  <section class="brief">
    <div class="sectionhead"><span>Latest Pulse</span><span><!-- as of run/date --></span></div>
    <div class="briefgrid">
      <div class="briefitem ok"><div class="k">Outcome</div><p><!-- What the run and Pulse pass accomplished. --></p></div>
      <div class="briefitem"><div class="k">Goal movement</div><p><!-- Toward, flat, or away from success, with freshness. --></p></div>
      <div class="briefitem"><div class="k">Next</div><p><!-- One most important next action or evidence boundary. --></p></div>
    </div>
  </section>

  <section class="activity">
    <h2>Activity</h2>
    <!-- LOG ENTRIES: newest first -->

    <!-- Example material date group; omit examples from the saved file. -->
    <div class="daygroup">
      <div class="daydate">YYYY-MM-DD</div>
      <div class="run" data-date="YYYY-MM-DD" data-kind="run" data-pulse-section="reflection" data-module="run_summary">
        <span class="st ok">Completed</span><span class="when">run freshness</span>
        <span class="note">One material user-visible run outcome.</span>
      </div>
      <article class="entry" data-date="YYYY-MM-DD" data-kind="monitor" data-pulse-section="signals" data-module="workflow_review">
        <div class="ehead"><span class="tag">Needs attention</span><span class="kind bug">Bug</span><h2 class="etitle">Short material issue title</h2><span class="when">YYYY-MM-DD</span></div>
        <p class="takeaway"><b>What happened:</b> Plain-language transition only.</p>
        <p class="impact"><b>Why it matters:</b> User-visible impact.</p>
        <p class="meta"><b>Next:</b> One next step; full evidence stays in Pulse.</p>
      </article>
      <article class="entry decision" data-date="YYYY-MM-DD" data-kind="input" data-pulse-section="improvements" data-module="goal_advisor" data-status="answered">
        <div class="ehead"><span class="tag">User decision</span><span class="worklabel">Question + answer</span><h2 class="etitle">Decision applied to the strategy</h2><span class="when">YYYY-MM-DD</span></div>
        <p class="takeaway"><b>What happened:</b> The answered decision and resulting outcome.</p>
      </article>
      <article class="entry fix" data-date="YYYY-MM-DD" data-kind="decision" data-pulse-section="improvements" data-module="pulse_fixer">
        <div class="ehead"><span class="tag">Fixed</span><h2 class="etitle">Material repair verified</h2><span class="when">YYYY-MM-DD</span></div>
        <p class="takeaway"><b>What happened:</b> User-visible repair and proof state.</p>
      </article>
    </div>
  </section>

  <div id="pulse-agent-handoff" data-pulse-run-id="" hidden></div>

  <section class="archive">
    <h2>Archive</h2>
    <a class="arow" href="improve-archive/YYYY-MM.html"><span>YYYY-MM</span><span class="n">date range · moved item count</span></a>
  </section>

  <footer>Published Pulse journal · full operational detail is available in Pulse.</footer>
</main></body>
</html>
```
