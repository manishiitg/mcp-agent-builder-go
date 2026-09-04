import { memo, useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { useReportDataApi } from './reportEmbedContext'
import { REPORT_OPEN_ATTR, REPORT_SRC_ATTR } from './reportMarkdownLinks'

// Kept behind Vite's development flag: this lets us distinguish an iframe
// navigation from a normal React render when diagnosing report flicker, without
// adding production console noise.
function debugReportFrame(event: string, title: string, detail?: Record<string, unknown>) {
  if (!import.meta.env.DEV) return
  console.debug('[ReportFrame]', event, { title, ...detail })
}

// The data API is injected at iframe onLoad — i.e. AFTER the report's own inline
// <script> has already parsed and run. So `window.report` does not exist while a
// report's top-level script executes, and every report had to invent its own way
// of waiting for it. They invented it badly and differently: hand-rolled
// `setInterval` polls that give up after N tries and run anyway, `DOMContentLoaded`
// handlers that double-fire alongside the poll, and `load().then(...)` chains with
// no `.catch()`. A report whose script then threw showed its "Loading…"
// placeholders forever — indistinguishable from still-working — because nothing
// surfaced the error. Measured across this workspace: 4 of 5 reports had no error
// handling at all.
//
// Prepending this stub makes `window.report.ready(fn)` available from the report's
// FIRST line, before any data exists. Callbacks queue until the real API is
// injected, then run — and re-run on every subsequent data refresh, so a report
// never needs to know about `report:data` or about injection timing at all. Errors
// thrown inside the callback (sync or async) are routed to the frame's error
// surface instead of vanishing.
//
// `.ready()` only helps a report that actually calls it. In practice most
// agent-authored reports reach for a more instinctive pattern instead —
// `DOMContentLoaded`, `window.onload`, or a bare top-level
// `(async()=>{ await window.report.query(...) })()` — all of which can run
// before injection, when `window.report.query` doesn't exist yet. Previously
// that was a synchronous TypeError (`.query is not a function`). So the stub
// also predefines `query`/`get`/`getText`/`getHtml`/`fileUrl`/`updateField`/`updateFields` itself: called
// before injection, each one queues its call and returns a pending promise
// instead of throwing; `inject()` below replays every queued call against the
// real API once it exists. This makes the wrong-but-instinctive pattern work
// correctly too, not just the documented one — the report doesn't need to know
// injection is asynchronous at all, regardless of which lifecycle hook it used.
//
// This is deliberately platform-owned rather than documented guidance: the report
// HTML is authored per workflow by an agent that never renders its own output, so
// a bootstrap it must write correctly every time is a bug generator. A bootstrap
// it inherits cannot be got wrong.
const REPORT_BOOTSTRAP = `<script>(function(){
  if (window.report && window.report.ready) return;
  var queued = [];
  var pending = [];
  var api = window.report || {};
  api.ready = function(fn){
    if (typeof fn !== 'function') return;
    queued.push(fn);
    if (window.__reportApiReady) window.__runReportCallback(fn);
  };
  function queueCall(name){
    return function(){
      var args = Array.prototype.slice.call(arguments);
      return new Promise(function(resolve, reject){
        pending.push({ name: name, args: args, resolve: resolve, reject: reject });
      });
    };
  }
  ['query', 'get', 'getText', 'getHtml', 'fileUrl', 'updateField', 'updateFields'].forEach(function(name){
    api[name] = queueCall(name);
  });
  api.openFile = function(){
    pending.push({ name: 'openFile', args: Array.prototype.slice.call(arguments), resolve: function(){}, reject: function(){} });
  };
  api.theme = 'light';
  window.__reportQueuedCallbacks = queued;
  window.__reportPendingCalls = pending;
  window.report = api;
})();</script>`

// Placed immediately after <head> when present so it precedes any author script;
// falls back to prepending for a fragment without a full document shell.
export function withReportBootstrap(html: string): string {
  if (html.includes('window.__reportQueuedCallbacks')) return html
  const headOpen = html.match(/<head[^>]*>/i)
  if (headOpen?.index !== undefined) {
    const at = headOpen.index + headOpen[0].length
    return html.slice(0, at) + REPORT_BOOTSTRAP + html.slice(at)
  }
  const htmlOpen = html.match(/<html[^>]*>/i)
  if (htmlOpen?.index !== undefined) {
    const at = htmlOpen.index + htmlOpen[0].length
    return html.slice(0, at) + REPORT_BOOTSTRAP + html.slice(at)
  }
  return REPORT_BOOTSTRAP + html
}

// App theme tokens (HSL triplets) exposed to the HTML report as CSS variables so
// it can match the app palette via hsl(var(--…)) and switch with light/dark. Read
// from the (themed) iframe host element so report-theme overrides are included.
const REPORT_THEME_VARS = [
  'background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground',
  'primary', 'primary-foreground', 'secondary', 'secondary-foreground',
  'muted', 'muted-foreground', 'accent', 'accent-foreground',
  'border', 'input', 'ring', 'destructive', 'destructive-foreground',
  'chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5',
] as const

function injectThemeTokens(host: HTMLElement, doc: Document) {
  const cs = getComputedStyle(host)
  const decls = REPORT_THEME_VARS
    .map((v) => {
      const val = cs.getPropertyValue(`--${v}`).trim()
      return val ? `--${v}:${val};` : ''
    })
    .join('')
  if (!decls) return
  let style = doc.getElementById('__report_theme_tokens') as HTMLStyleElement | null
  if (!style) {
    style = doc.createElement('style')
    style.id = '__report_theme_tokens'
    doc.head?.appendChild(style)
  }
  style.textContent = `:root{${decls}}`
}

// Drop the about:srcdoc UA default `body{margin:8px}` so an HTML report renders
// edge-to-edge inside the report pane (we strip all our own chrome for HTML — the
// 8px UA margin would otherwise read as a stray gap around every side). Inserted
// as the FIRST <head> child so the report's own CSS (later, equal specificity)
// still wins if it sets its own body margin/padding. Idempotent per document.
function injectBaseReset(doc: Document) {
  if (doc.getElementById('__report_base_reset')) return
  const style = doc.createElement('style')
  style.id = '__report_base_reset'
  style.textContent = 'html,body{margin:0;padding:0;}'
  const head = doc.head
  if (head) head.insertBefore(style, head.firstChild)
}

// Default prose style for markdown injected via window.report.getHtml() — the
// helper wraps output in `.report-markdown`, and this gives it readable,
// theme-aware typography (using the app tokens injected by injectThemeTokens) so
// an embedded .md looks right with zero effort. The report can override any of
// these in its own CSS. Inserted after the theme-token style so var(--…) resolve.
function injectMarkdownStyles(doc: Document) {
  if (doc.getElementById('__report_markdown_styles')) return
  const style = doc.createElement('style')
  style.id = '__report_markdown_styles'
  style.textContent = `
.report-markdown{color:hsl(var(--foreground,222 47% 11%));line-height:1.6;font-size:0.95rem}
.report-markdown h1,.report-markdown h2,.report-markdown h3,.report-markdown h4{line-height:1.25;font-weight:650;margin:1.4em 0 .5em}
.report-markdown h1{font-size:1.5em}.report-markdown h2{font-size:1.25em}.report-markdown h3{font-size:1.1em}
.report-markdown p,.report-markdown ul,.report-markdown ol{margin:.6em 0}
.report-markdown ul,.report-markdown ol{padding-left:1.4em}
.report-markdown a{color:hsl(var(--primary,222 89% 55%));text-decoration:underline;text-underline-offset:2px}
.report-markdown code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.88em;background:hsl(var(--muted,210 40% 96%));padding:.12em .35em;border-radius:4px}
.report-markdown pre{background:hsl(var(--muted,210 40% 96%));padding:.8em 1em;border-radius:8px;overflow:auto}
.report-markdown pre code{background:none;padding:0}
.report-markdown blockquote{margin:.6em 0;padding:.2em .9em;border-left:3px solid hsl(var(--border,214 32% 88%));color:hsl(var(--muted-foreground,215 16% 47%))}
.report-markdown table{border-collapse:collapse;width:100%;margin:.7em 0;font-size:.9em}
.report-markdown th,.report-markdown td{border:1px solid hsl(var(--border,214 32% 88%));padding:.4em .6em;text-align:left}
.report-markdown th{background:hsl(var(--muted,210 40% 96%));font-weight:600}
.report-markdown img{max-width:100%;height:auto}
.report-markdown hr{border:0;border-top:1px solid hsl(var(--border,214 32% 88%));margin:1.2em 0}
`.trim()
  doc.head?.appendChild(style)
}

// FORWARDED_APP_SHORTCUT_KEYS are the parent app's own global chords (App.tsx's
// window keydown handler): mode switches 1/2/3, workspace 6, auto-scroll 7,
// K quick switcher, N new chat. Anything outside this set stays with the
// browser so normal in-report keys keep working.
const FORWARDED_APP_SHORTCUT_KEYS = new Set(['1', '2', '3', '6', '7', 'k', 'n'])

// HtmlReportFrame renders an HTML report in a sandboxed iframe and injects a live
// data API onto the iframe's window as `window.report`, then fires a `report:data`
// event. The HTML owns ALL rendering (its own charts/tables/branded CSS) — we
// only deliver data — which is the right model for HTML: full styling freedom,
// no React-in-iframe or theme-matching. Re-injects + re-fires when the report's
// data changes so the HTML can re-render without a reload. Outside a report (no
// data API in context) the HTML renders standalone.
//
// Inside the HTML report:
//   await window.report.query(sql)   // read-only SQL against db/db.sqlite -> array of row objects
//   await window.report.get(path)    // any db/ knowledgebase/ docs file -> parsed JSON (or text)
//   await window.report.getText(path)// raw file text
//   await window.report.getHtml(path) // a markdown file rendered to an HTML string (wrapped in
//                                      // .report-markdown, default prose style injected) — drop a
//                                      // rendered .md inline: el.innerHTML = await window.report.getHtml(p)
//   await window.report.fileUrl(path)// blob URL for <img>/<a>/<iframe> (images, PDFs, …)
//   window.report.openFile(path)     // open a file in the in-report preview modal
//   await window.report.updateField(table, row_id, column, value) // write one cell;
//                                      // table/column validated against the live schema
//                                      // server-side, row matched on its own primary key —
//                                      // no SQL passes through this call. Rejects columns
//                                      // that identify or timestamp the row. Resolves
//                                      // { oldValue, newValue } once committed.
//   await window.report.updateFields(table, row_id, {col1: v1, col2: v2}) // form-style:
//                                      // write several columns on one row atomically (all
//                                      // fields applied, or none). Same validation as
//                                      // updateField, per column. Resolves
//                                      // { oldValues, newValues } keyed by column name.
//   window.report.theme              // 'dark' | 'light' — the APP's current theme
//   window.addEventListener('report:data', render)   // fires on load + on data refresh
//   window.addEventListener('report:theme', restyle) // fires when the app theme toggles
//
// Theme: the iframe is a separate document and `@media (prefers-color-scheme)`
// only sees the OS, not the app's in-app light/dark toggle. So the frame mirrors
// the app theme onto the iframe's <html> as BOTH a `.dark` class and a
// `data-theme="dark|light"` attribute, exposes `window.report.theme`, and keeps
// them in sync via a MutationObserver when the user toggles. Author HTML to key
// off `:root.dark` / `[data-theme="dark"]` (and prefers-color-scheme as a
// standalone fallback).
//
// autoHeight: size the iframe to its content (no inner scrollbar / clipping) and
// keep it in sync via a ResizeObserver as content renders. The outer report
// pane owns scrolling, so this frame must not impose its own scroll boundary.
function HtmlReportFrameComponent({
  html,
  title,
  className,
  autoHeight = false,
  refreshToken = 0,
  focusTarget,
}: {
  html: string
  title: string
  className: string
  autoHeight?: boolean
  // A report's live data is deliberately refreshed only when its owner asks.
  // Background workflow/status polling must never turn into an iframe reload.
  refreshToken?: number
  /** A top-level tab (or section) the report HTML should switch to. Delivered
   * as `report.focus` + a `report:focus` event; a report that does not listen
   * simply stays where it is. */
  focusTarget?: { value: string; token: number }
}) {
  const dataApi = useReportDataApi()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const observerRef = useRef<ResizeObserver | null>(null)
  const appliedHtmlRef = useRef<string | null>(null)
  const loadedDocumentRef = useRef<Document | null>(null)
  const injectedDocumentRef = useRef<Document | null>(null)
  const injectedDataApiRef = useRef<typeof dataApi>(null)
  const injectedRefreshTokenRef = useRef<number | null>(null)
  const appliedThemeRef = useRef<{ document: Document; theme: 'dark' | 'light' } | null>(null)

  // Do not pass srcDoc through React's normal DOM-prop reconciliation. A report
  // frame is a live document: when an outer polling update re-renders its
  // parent, Chromium can treat a reapplied srcDoc as a navigation and visibly
  // restart the report. Assign it imperatively only on initial mount or when
  // the report file's actual HTML changes.
  useLayoutEffect(() => {
    const frame = iframeRef.current
    if (!frame || appliedHtmlRef.current === html) return
    appliedHtmlRef.current = html
    injectedDocumentRef.current = null
    injectedDataApiRef.current = null
    injectedRefreshTokenRef.current = null
    appliedThemeRef.current = null
    loadedDocumentRef.current = null
    debugReportFrame('srcdoc assigned', title, { bytes: html.length })
    frame.srcdoc = withReportBootstrap(html)
  }, [html, title])

  // Mirror the APP's light/dark theme onto the iframe document (the agent's HTML
  // designs its own palette but keys the active mode off this). The app uses a
  // `.dark` (or `.dark-plus`) class on <html>.
  const applyTheme = useCallback((emitChange = true) => {
    const frame = iframeRef.current
    if (!frame) return
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const win = frame.contentWindow as any
    const doc = frame.contentDocument
    if (!win || !doc?.documentElement) return
    const cl = document.documentElement.classList
    const theme: 'dark' | 'light' = cl.contains('dark') || cl.contains('dark-plus') ? 'dark' : 'light'
    const previousTheme = appliedThemeRef.current?.document === doc
      ? appliedThemeRef.current.theme
      : null
    doc.documentElement.classList.toggle('dark', theme === 'dark')
    doc.documentElement.setAttribute('data-theme', theme)
    if (win.report) win.report.theme = theme
    // Expose the app's resolved theme tokens (current light/dark + report theme)
    // as CSS variables inside the iframe so the HTML can use hsl(var(--…)).
    injectThemeTokens(frame, doc)
    appliedThemeRef.current = { document: doc, theme }
    if (emitChange && previousTheme !== null && previousTheme !== theme) {
      try {
        win.dispatchEvent(new win.Event('report:theme'))
      } catch {
        /* iframe may have navigated/reloaded */
      }
    }
  }, [])

  const resize = useCallback(() => {
    if (!autoHeight) return
    const frame = iframeRef.current
    const doc = frame?.contentDocument
    if (!frame || !doc || !doc.body) return
    // PLAT-160-adjacent. `documentElement.scrollHeight`/`body.scrollHeight` can
    // never report LESS than the iframe's own viewport height — the root
    // element fills the viewport by definition — so measuring while the frame
    // is already tall reads back the frame's own height and ratchets upward
    // forever. The previous fix collapsed the frame to 0px before measuring to
    // break that loop, but doing so changes the report's OWN viewport, and any
    // `vh`-sized content inside it — most concretely a report embedding its
    // own nested iframe sized with `min-height: calc(100vh - Npx)` — collapses
    // along with it. Measured live on salesoutreach's reporting dashboard: its
    // GTM-strategy tab's nested iframe genuinely renders at ~1884px, but
    // dropped to ~152px during the collapse window, so the outer frame was
    // sized to a small fraction of the real content with nothing left to
    // scroll to reach the rest — not a slow report, an invisible one.
    //
    // Measuring the bottom of each of body's direct children via
    // getBoundingClientRect instead avoids the viewport-floor problem
    // entirely — an element's own laid-out position reflects real content,
    // not the "root fills viewport" guarantee that requires collapsing
    // anything to sidestep. No frame-height mutation happens before the
    // measurement, so nothing inside the report ever sees an artificial
    // viewport change.
    const scrollY = doc.defaultView?.scrollY ?? 0
    let maxBottom = 0
    for (const child of Array.from(doc.body.children)) {
      const bottom = child.getBoundingClientRect().bottom + scrollY
      if (bottom > maxBottom) maxBottom = bottom
    }
    const content = Math.ceil(maxBottom)
    if (content <= 0) return
    const previousHeight = frame.style.height
    const nextHeight = `${content}px`
    if (previousHeight === nextHeight) return
    // The outer report pane owns scrolling. Let the frame reach its actual content
    // height so an HTML report never creates a second, nested scroll surface.
    debugReportFrame('height changed', frame.title, { from: previousHeight, to: nextHeight })
    frame.style.height = nextHeight
  }, [autoHeight])

  const inject = useCallback(() => {
    const frame = iframeRef.current
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const win = frame?.contentWindow as any
    const doc = frame?.contentDocument
    if (!win || !doc) return
    const firstInjectionForDocument = injectedDocumentRef.current !== doc
    const dataApiChanged = injectedDataApiRef.current !== dataApi

    injectBaseReset(doc)
    injectMarkdownStyles(doc)

    // In a srcDoc iframe the base URL is about:srcdoc, so clicking an in-page
    // `#anchor` link (the report's tab nav) reloads the WHOLE document instead of
    // scrolling. Intercept those clicks and scroll manually. Bound once per loaded
    // document (the flag resets on reload, so a fresh doc re-binds).
    // The app's global shortcuts (Ctrl/Cmd+K quick switcher, mode switches, …)
    // are bound on the PARENT window. While focus is inside this iframe the
    // keydown fires on the iframe's own document and the parent listener never
    // sees it, so the switcher appeared dead on the reporting dashboard.
    // Re-dispatch just those shortcuts upward.
    //
    // Deliberately an allowlist: forwarding every ctrl/meta chord would hijack
    // the browser's own in-frame behaviour (Ctrl+C copy, Ctrl+F find, Ctrl+A,
    // Ctrl+P print), which must keep working while reading a report.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (!(doc as any).__appShortcutsBound) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(doc as any).__appShortcutsBound = true
      doc.addEventListener('keydown', (e: Event) => {
        const key = e as KeyboardEvent
        if (!(key.ctrlKey || key.metaKey) || key.altKey) return
        if (!FORWARDED_APP_SHORTCUT_KEYS.has(key.key.toLowerCase())) return
        key.preventDefault()
        window.dispatchEvent(new KeyboardEvent('keydown', {
          key: key.key,
          ctrlKey: key.ctrlKey,
          metaKey: key.metaKey,
          shiftKey: key.shiftKey,
          altKey: key.altKey,
          bubbles: true,
        }))
      })
    }

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (!(doc as any).__anchorScrollBound) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(doc as any).__anchorScrollBound = true
      doc.addEventListener('click', (e: Event) => {
        const el = e.target as Element | null
        const link = el?.closest?.('a[href]') as HTMLAnchorElement | null
        if (!link) return
        const href = link.getAttribute('href') || ''

        // A workspace file link inside rendered markdown (rewritten by
        // reportMarkdownLinks): open it in the in-report preview modal.
        const openPath = link.getAttribute(REPORT_OPEN_ATTR)
        if (openPath) {
          e.preventDefault()
          dataApi?.openFile(openPath)
          return
        }

        // In-page `#anchor` links (the report's tab nav): the srcDoc base URL is
        // about:srcdoc, so a default click reloads the whole document instead of
        // scrolling. Intercept and scroll manually.
        if (href.startsWith('#')) {
          const target = doc.getElementById(href.slice(1))
          if (!target) return
          e.preventDefault()
          target.scrollIntoView({ behavior: 'smooth', block: 'start' })
          return
        }

        // External links (Notion, Jira, GitHub, docs, …): the iframe sandbox has
        // no allow-popups/allow-top-navigation, so a click would be silently
        // swallowed. Open them in a new browser tab from the parent window
        // instead, keeping the sandbox locked down.
        if (/^https?:\/\//i.test(href)) {
          e.preventDefault()
          window.open(href, '_blank', 'noopener,noreferrer')
        }
      })
    }

    // A report's own script failing used to be completely silent: nothing here
    // listened for `error` or `unhandledrejection`, so a throw inside the page's
    // render left its "Loading…" placeholders on screen forever with no clue why
    // — the report looked like it was still working, indefinitely. Reports are
    // agent-authored per workflow (the `upgrade-direct-html-reports` migration
    // composes them), and validate_report_html is purely static — it parses the
    // markup and checks that scripted element ids exist, but never executes the
    // page, so it cannot catch a script that dies before it writes anything.
    // That combination made a broken report indistinguishable from a slow one.
    //
    // Surfacing the error in place of the spinner is deliberately done in the
    // host rather than in each report: it fixes every existing report at once
    // and cannot be forgotten by whatever authors the next one.
    const showReportError = (message: string, source?: string) => {
      const text = String(message || 'Unknown error').slice(0, 400)
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        if ((doc as any).__reportErrorShown) return
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ;(doc as any).__reportErrorShown = true
        debugReportFrame('report script error', title, { message: text, source })
        const banner = doc.createElement('div')
        banner.setAttribute('data-report-error', '1')
        banner.style.cssText = [
          'margin:12px',
          'padding:12px 14px',
          'border:1px solid #ef4444',
          'border-left-width:4px',
          'border-radius:6px',
          'background:rgba(239,68,68,0.08)',
          'color:#b91c1c',
          'font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace',
          'white-space:pre-wrap',
          'word-break:break-word',
        ].join(';')
        banner.textContent =
          'This report failed to render.\n\n' +
          text +
          (source ? `\n\n(${source})` : '') +
          '\n\nData loaded through window.report is unavailable, so any "Loading…" ' +
          'text below is stale rather than in progress.'
        try {
          doc.body?.insertBefore(banner, doc.body.firstChild)
        } catch {
          /* document may have navigated away mid-error */
        }
      }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (!(doc as any).__reportErrorBound) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(doc as any).__reportErrorBound = true
      win.addEventListener('error', (e: Event) => {
        const err = e as ErrorEvent
        showReportError(
          err?.error?.stack || err?.message || 'Script error',
          err?.filename ? `${err.filename}:${err.lineno}` : undefined,
        )
      })
      win.addEventListener('unhandledrejection', (e: Event) => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const reason = (e as any)?.reason
        showReportError(
          reason?.stack || reason?.message || String(reason ?? 'Unhandled promise rejection'),
          'unhandled promise rejection',
        )
      })
    }

    // Images inside rendered markdown (reportMarkdownLinks) carry the
    // workspace path on a data attribute instead of src, because a relative
    // src resolves to nothing under about:srcdoc. Load each through the
    // authenticated blob channel, now and whenever the report inserts more
    // markdown later (a tab switch, a data refresh).
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (dataApi && !(doc as any).__reportImageResolverBound) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(doc as any).__reportImageResolverBound = true
      const resolveImages = () => {
        doc.querySelectorAll(`img[${REPORT_SRC_ATTR}]:not([src])`).forEach((node) => {
          const img = node as HTMLImageElement
          const path = img.getAttribute(REPORT_SRC_ATTR)
          if (!path || img.dataset.reportSrcPending) return
          img.dataset.reportSrcPending = '1'
          void dataApi.fileUrl(path).then((url) => {
            if (url) img.src = url
            else img.alt = img.alt || `Missing file: ${path}`
          }).catch(() => { /* leave the alt text */ })
        })
      }
      resolveImages()
      try {
        new MutationObserver(resolveImages).observe(doc.documentElement, { childList: true, subtree: true })
      } catch {
        /* MutationObserver unavailable — only the initial pass runs */
      }
    }

    // Run one report.ready() callback with its errors surfaced rather than
    // swallowed. A callback may be sync or return a promise; both are covered,
    // which is the specific gap that made `load().then(...)` with no `.catch()`
    // fail silently.
    const runReportCallback = (fn: unknown) => {
      if (typeof fn !== 'function') return
      try {
        const result = (fn as () => unknown)()
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const maybe = result as any
        if (maybe && typeof maybe.then === 'function') {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          maybe.catch((err: any) =>
            showReportError(err?.stack || err?.message || String(err), 'report.ready()'),
          )
        }
      } catch (err) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const e = err as any
        showReportError(e?.stack || e?.message || String(e), 'report.ready()')
      }
    }
    win.__runReportCallback = runReportCallback

    if (dataApi) {
      win.report = {
        ready: (fn: unknown) => {
          if (typeof fn !== 'function') return
          const queue = win.__reportQueuedCallbacks
          if (Array.isArray(queue) && !queue.includes(fn)) queue.push(fn)
          runReportCallback(fn)
        },
        workspacePath: dataApi.workspacePath,
        query: dataApi.query,
        get: dataApi.get,
        getText: dataApi.getText,
        getHtml: dataApi.getHtml,
        renderMarkdown: dataApi.renderMarkdown,
        fileUrl: dataApi.fileUrl,
        openFile: dataApi.openFile,
        updateField: dataApi.updateField,
        updateFields: dataApi.updateFields,
        theme: 'light',
      }

      // The bootstrap's stub queued any query/get/getText/getHtml/fileUrl/openFile/
      // updateField/updateFields call made before this injection (DOMContentLoaded,
      // window.onload, a bare top-level await — anything that ran before window.report
      // was the real API). Replay each against the real methods now instead of leaving
      // those promises pending forever. Runs once per document: the bootstrap's pending
      // array is drained and cleared here, so it stays empty on every later
      // re-injection (data refresh, theme change).
      const realReportMethods: Record<string, ((...args: unknown[]) => unknown) | undefined> = {
        query: dataApi.query as (...args: unknown[]) => unknown,
        get: dataApi.get as (...args: unknown[]) => unknown,
        getText: dataApi.getText as (...args: unknown[]) => unknown,
        getHtml: dataApi.getHtml as (...args: unknown[]) => unknown,
        fileUrl: dataApi.fileUrl as (...args: unknown[]) => unknown,
        updateField: dataApi.updateField as (...args: unknown[]) => unknown,
        updateFields: dataApi.updateFields as (...args: unknown[]) => unknown,
      }
      type QueuedReportCall = {
        name: string
        args: unknown[]
        resolve: (value: unknown) => void
        reject: (reason: unknown) => void
      }
      const pendingCalls: QueuedReportCall[] = Array.isArray(win.__reportPendingCalls)
        ? [...win.__reportPendingCalls]
        : []
      win.__reportPendingCalls = []
      pendingCalls.forEach(({ name, args, resolve, reject }) => {
        if (name === 'openFile') {
          try {
            dataApi.openFile(...(args as Parameters<typeof dataApi.openFile>))
          } catch {
            /* best-effort — openFile has no result to resolve */
          }
          return
        }
        const fn = realReportMethods[name]
        if (typeof fn !== 'function') {
          reject(new Error(`window.report.${name} is unavailable`))
          return
        }
        Promise.resolve()
          .then(() => fn(...args))
          .then(resolve, reject)
      })

      // Initial theme application is setup, not a theme change. The single
      // report:data event below owns the initial render. This avoids every HTML
      // report doing a full data render once for theme and again for data.
      applyTheme(false)
      // A new iframe document and an explicit report refresh both need fresh
      // data. A normal parent render does not: assigning srcDoc again makes
      // Chromium navigate the iframe and visually "refresh" the report.
      const refreshRequested = injectedRefreshTokenRef.current !== refreshToken
      if (firstInjectionForDocument || dataApiChanged || refreshRequested) {
        debugReportFrame('report:data dispatched', title, {
          firstInjectionForDocument,
          dataApiChanged,
          refreshRequested,
          refreshToken,
        })
        // Mark the API live BEFORE flushing, so a ready() registered from inside
        // a callback runs immediately rather than waiting for the next refresh.
        win.__reportApiReady = true
        try {
          win.dispatchEvent(new win.Event('report:data'))
        } catch {
          /* iframe may have navigated/reloaded */
        }
        // Run everything the page queued via report.ready() before the API
        // existed, and re-run it on each subsequent refresh — the same contract
        // as the report:data listener, but without the page having to know that
        // event exists or that injection is asynchronous. Iterated over a copy:
        // a callback may register another ready() while running.
        const queued = Array.isArray(win.__reportQueuedCallbacks)
          ? [...win.__reportQueuedCallbacks]
          : []
        if (queued.length > 0) {
          debugReportFrame('report.ready callbacks flushed', title, { count: queued.length })
          queued.forEach(runReportCallback)
        }
      }
    }
    injectedDocumentRef.current = doc
    injectedDataApiRef.current = dataApi
    injectedRefreshTokenRef.current = refreshToken

    if (autoHeight) {
      observerRef.current?.disconnect()
      resize()
      try {
        const ro = new ResizeObserver(() => resize())
        if (doc.documentElement) ro.observe(doc.documentElement)
        if (doc.body) ro.observe(doc.body)
        observerRef.current = ro
      } catch {
        /* ResizeObserver unavailable — height stays at last measure */
      }
    }
  }, [dataApi, autoHeight, resize, applyTheme, refreshToken, title])

  // Re-inject when the report data changes (sources refreshed).
  useEffect(() => {
    // The first injection belongs exclusively to iframe onLoad. Calling it
    // before that can target the transient about:blank document and then make
    // the loaded report render a second time moments later.
    if (loadedDocumentRef.current !== iframeRef.current?.contentDocument) return
    inject()
  }, [inject])

  // A focus target from open_workspace_view(view="report", target="<tab>").
  // The report HTML owns its own tabs, so the platform cannot switch one: it
  // hands the name over as `report.focus` and fires `report:focus`, the same
  // shape as `report:theme`. A report that does not listen is unaffected.
  useEffect(() => {
    if (!focusTarget?.value) return
    const deliver = () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const win = iframeRef.current?.contentWindow as any
      if (!win?.report) return false
      try {
        win.report.focus = focusTarget.value
        win.dispatchEvent(new win.Event('report:focus'))
        return true
      } catch {
        return false // the frame navigated or reloaded under us
      }
    }
    if (deliver()) return
    // The report's own API is injected after load; retry briefly rather than
    // dropping a focus that arrived while the frame was still coming up.
    let tries = 0
    const timer = window.setInterval(() => {
      if (deliver() || ++tries > 20) window.clearInterval(timer)
    }, 100)
    return () => window.clearInterval(timer)
  }, [focusTarget])

  // Keep the iframe theme in sync when the user toggles the app's light/dark mode
  // while the report is open (watches the app's <html> class).
  useEffect(() => {
    const mo = new MutationObserver(() => applyTheme())
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
    return () => mo.disconnect()
  }, [applyTheme])

  // Disconnect the observer on unmount.
  useEffect(() => () => observerRef.current?.disconnect(), [])

  return (
    <iframe
      ref={iframeRef}
      title={title}
      sandbox="allow-same-origin allow-scripts"
      onLoad={() => {
        loadedDocumentRef.current = iframeRef.current?.contentDocument || null
        debugReportFrame('iframe loaded', title)
        inject()
      }}
      className={className}
    />
  )
}

// Report frames contain independently-running HTML. Keep their document stable
// while terminal, Pulse, and human-input polling updates the outer React tree.
// The owner passes refreshToken when a deliberate data refresh is wanted.
export const HtmlReportFrame = memo(HtmlReportFrameComponent)
