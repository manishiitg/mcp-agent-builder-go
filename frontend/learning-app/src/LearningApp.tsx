import { useState, useEffect, useRef, useCallback, useMemo, lazy, Suspense, type FormEvent, type ChangeEvent } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize'
import { visit } from 'unist-util-visit'
import type { Element as HastElement } from 'hast'

// Lazy-loaded the same way AgentWorks does it — react-syntax-highlighter's
// language grammars are large, so keep them out of the initial bundle and
// only fetch when a reply/file actually contains a fenced code block.
const SyntaxHighlightedCode = lazy(() => import('./SyntaxHighlightedCode'))

let mermaidModule: Promise<typeof import('mermaid').default> | null = null
function loadMermaid() {
  mermaidModule ??= import('mermaid').then((m) => m.default)
  return mermaidModule
}
import {
  Activity as PulseIcon,
  ArrowLeft,
  ArrowRight,
  Bell,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronDown,
  ExternalLink,
  FileArchive,
  FileCode,
  FileSpreadsheet,
  FileText,
  FileType,
  Film,
  Folder,
  FolderOpen,
  HardDrive,
  Type,
  Image as ImageIcon,
  Info,
  LockKeyhole,
  Maximize2,
  Mic,
  Minimize2,
  Music,
  Presentation,
  Paperclip,
  Printer,
  RefreshCw,
  Send,
  Settings as SettingsIcon,
  Sparkles,
  Star,
  Sun,
  Zap,
} from 'lucide-react'
import './learning-app.css'
import {
  useSetupStore,
  useFamilyStore,
  useParentChatStore,
  useWorkspaceStore,
  useChildChatStore,
  useWhatsAppStore,
  usePinGateStore,
  toParentMsg,
  type Screen,
  type ApiEngine,
  type ParentMsg,
  type StoredMsg,
  type DebugToolCall,
  type TreeNode,
  type WsFile,
  type Activity,
  type VoiceStatus,
} from './stores'
import { FAMILY_API } from './apiBase'
import { VoiceSettings } from './voice/VoiceSettings'
import { readReminderSoundPref, persistReminderSoundPref, playReminderChime } from './notifySound'
import { MicButton } from './voice/MicButton'

// autoGrowTextarea lets a composer grow with a long message instead of
// staying a single row — resets to natural height first so it can shrink
// back down too (e.g. after deleting text), then grows to fit content up to
// a cap, beyond which the textarea's own CSS overflow-y:auto takes over.
const COMPOSER_MAX_HEIGHT = 160
// How many of the parent's most recent messages render by default — see the
// parentVisibleCount comment further down for why.
const PARENT_HISTORY_PAGE_SIZE = 40
function autoGrowTextarea(el: HTMLTextAreaElement) {
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, COMPOSER_MAX_HEIGHT) + 'px'
}

// Several WhatsApp photos sent together arrive as consecutive 'photo' tool
// messages — grouped here into one row so they render side by side (a
// horizontal strip) instead of one full-width block per photo stacked
// vertically. indexOffset carries visibleParentMessages' windowing offset
// (see hiddenParentCount) through to the original message's stable index.
type PhotoRenderGroup<T> = { kind: 'photos'; paths: string[] } | { kind: 'msg'; msg: T; index: number }
function groupConsecutivePhotos<T extends { role: string; tool?: string; path?: string }>(messages: T[], indexOffset = 0): PhotoRenderGroup<T>[] {
  const groups: PhotoRenderGroup<T>[] = []
  messages.forEach((m, idx) => {
    if (m.role === 'tool' && m.tool === 'photo' && m.path) {
      const last = groups[groups.length - 1]
      if (last && last.kind === 'photos') {
        last.paths.push(m.path)
      } else {
        groups.push({ kind: 'photos', paths: [m.path] })
      }
      return
    }
    groups.push({ kind: 'msg', msg: m, index: indexOffset + idx })
  })
  return groups
}

// The child/file viewer iframe is deliberately sandbox="allow-scripts" with
// NO allow-same-origin (adding that would let a srcDoc page's script escape
// the sandbox and touch the parent page/cookies) — which makes it a
// cross-origin frame from the app's own perspective. So the app can never read
// or set that frame's scroll from outside; anything positional has to be done
// by a script injected INTO the document (see withViewerPositionScript).
function engineStatus(e: ApiEngine): { label: string; ready: boolean } {
  if (e.usable) return { label: 'Ready', ready: true }
  if (!e.runtime_available) return { label: 'Not set up', ready: false }
  if (!e.auth_configured) return { label: 'Needs sign-in', ready: false }
  return { label: 'Unavailable', ready: false }
}

// Parent-friendly presentation, keyed by the technical engine id.
// Order reflects the product preference: ChatGPT → Claude → Cursor → Pi.
const ENGINE_PRESENTATION: Record<string, { name: string; blurb: string; order: number; preferred?: boolean }> = {
  'codex-cli': { name: 'ChatGPT', blurb: 'Uses your ChatGPT account · can also create images', order: 1, preferred: true },
  'claude-code': { name: 'Claude', blurb: 'Careful, patient, step-by-step teaching', order: 2 },
  'cursor-cli': { name: 'Cursor', blurb: 'Uses your Cursor account', order: 3 },
  'pi-cli': { name: 'Pi', blurb: 'Lets you pick from many other AI models', order: 4 },
}
function pres(id: string, fallbackName: string) {
  return ENGINE_PRESENTATION[id] ?? { name: fallbackName, blurb: 'Available on this computer', order: 99, preferred: false }
}

// Child profile options — edit here to adjust the setup form.
// Targeting grades 6–12, with 4–5 also offered.
const GRADES = ['4', '5', '6', '7', '8', '9', '10', '11', '12']
const BOARDS = ['CBSE', 'ICSE / ISC (CISCE)', 'State Board', 'NIOS', 'IB', 'Cambridge / IGCSE', 'Other', 'Not sure']

// Absolute date + time label for a package, e.g. "21 Jul 2026, 5:42 PM".
function dateTimeLabel(iso?: string): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  return new Date(t).toLocaleString(undefined, { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit', hour12: true })
}

// dateOnlyKey/dateOnlyLabel back the Workspace tab's "group by date" view —
// key groups by calendar day (local time, so late-evening activities don't
// slip into the next day's group), label is what's actually shown as the
// section heading.
function dateOnlyKey(iso?: string): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  return new Date(t).toLocaleDateString('en-CA')
}
function dateOnlyLabel(iso?: string): string {
  if (!iso) return 'Undated'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return 'Undated'
  return new Date(t).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })
}

// activityMode turns an activity's teaching_mode into the one word a parent
// actually cares about when scanning their library: is this something she'll
// be taught, something she'll practise, or something she's being tested on?
// It's already recorded on every activity but was invisible in the UI until
// now. Colours reuse the app's own long-standing guides/tests/reports tints.
function activityMode(mode?: string): { label: string; cls: string } | null {
  switch (mode) {
    case 'beginner': return { label: 'Learn', cls: 'is-learn' }
    case 'graduated': return { label: 'Practice', cls: 'is-practice' }
    case 'strict': return { label: 'Test', cls: 'is-test' }
    default: return null // older activities predate the field — say nothing rather than guess
  }
}

// Which side of the handoff the browser should land on after a refresh.
// Without this, a refresh always falls back to Parent Mode — letting a child
// bypass the PIN gate entirely just by reloading the page. Persisted in
// localStorage (this is a single local-machine app, same as the rest of its
// state) and flipped explicitly at every real hand-off/PIN-unlock point.
const HANDOFF_SIDE_KEY = 'sparkquill.handoff-side'
function persistHandoffSide(side: 'tutor' | 'parent') {
  try { localStorage.setItem(HANDOFF_SIDE_KEY, side) } catch { /* best-effort */ }
}
function readHandoffSide(): 'tutor' | 'parent' {
  try { return localStorage.getItem(HANDOFF_SIDE_KEY) === 'tutor' ? 'tutor' : 'parent' } catch { return 'parent' }
}

// How wide the child's right-hand pane is, in px, dragged by the divider between
// her chat and her worksheet. Persisted because it's a per-child working
// preference, not a per-session one: whoever likes a big worksheet and a narrow
// chat should get that on every visit without re-dragging it.
//
// Bounds keep both panes usable — a pane dragged to nothing looks like a broken
// layout rather than a deliberate choice, and there's no affordance to get it
// back once the handle is off-screen.
const CHILD_SIDE_WIDTH_KEY = 'sparkquill.child-side-width'
const CHILD_SIDE_MIN = 320
const CHILD_SIDE_DEFAULT = 592 // the previous fixed width (348 + 244)
// The chat's floor is a PIXEL minimum, not a fraction of the window. A fraction
// looks fine on a large display and collapses on a small one — 20% of 1100px is
// 220px, which cannot hold a readable message bubble plus the composer. This is
// the width below which the chat stops being usable, so the worksheet never gets
// to claim it however wide it is asked to be.
const CHILD_CHAT_MIN = 400

// childSideMax is how wide the worksheet may get for a given window width.
// Never negative: on a very narrow window the min wins and the layout falls back
// to the single-column media query anyway.
function childSideMax(windowWidth: number): number {
  return Math.max(CHILD_SIDE_MIN, windowWidth - CHILD_CHAT_MIN)
}

function readChildSideWidth(): number {
  try {
    const n = Number(localStorage.getItem(CHILD_SIDE_WIDTH_KEY))
    return Number.isFinite(n) && n >= CHILD_SIDE_MIN ? n : CHILD_SIDE_DEFAULT
  } catch { return CHILD_SIDE_DEFAULT }
}
function persistChildSideWidth(px: number) {
  try { localStorage.setItem(CHILD_SIDE_WIDTH_KEY, String(Math.round(px))) } catch { /* best-effort */ }
}

// Parent Mode's own drag-to-resize for the workspace drawer — same mechanism
// as the child's worksheet, but capped at HALF the window rather than nearly
// all of it: the drawer here is a reference panel beside the conversation
// (Academics/Progress/Files), not the primary thing being read, so the chat
// should never be squeezed to a sliver the way the child's worksheet is
// allowed to claim most of the screen.
const PARENT_SIDE_WIDTH_KEY = 'sparkquill.parent-side-width'
const PARENT_SIDE_MIN = 320
const PARENT_SIDE_DEFAULT = 592 // the previous fixed width
function parentSideMax(windowWidth: number): number {
  return Math.max(PARENT_SIDE_MIN, Math.floor(windowWidth * 0.5))
}
function readParentSideWidth(): number {
  try {
    const n = Number(localStorage.getItem(PARENT_SIDE_WIDTH_KEY))
    return Number.isFinite(n) && n >= PARENT_SIDE_MIN ? n : PARENT_SIDE_DEFAULT
  } catch { return PARENT_SIDE_DEFAULT }
}
function persistParentSideWidth(px: number) {
  try { localStorage.setItem(PARENT_SIDE_WIDTH_KEY, String(Math.round(px))) } catch { /* best-effort */ }
}

// Reading size for the worksheet. Applied as CSS `zoom` inside the viewer
// iframe rather than a font-size override: the generated pages size everything
// in px (headings, card padding, SVG diagrams), so scaling only the body font
// would grow the paragraphs and leave headings and diagrams behind. Zoom scales
// the whole page coherently, which is what "bigger" should mean on a worksheet.
const CHILD_ZOOM_KEY = 'sparkquill.child-zoom'
const CHILD_ZOOM_STEPS = [1, 1.15, 1.35, 1.6]
function readChildZoom(): number {
  try {
    const n = Number(localStorage.getItem(CHILD_ZOOM_KEY))
    return CHILD_ZOOM_STEPS.includes(n) ? n : 1
  } catch { return 1 }
}
function persistChildZoom(z: number) {
  try { localStorage.setItem(CHILD_ZOOM_KEY, String(z)) } catch { /* best-effort */ }
}

// Reading size for the CHAT side, kept separate from the worksheet's: she may
// want big text on a dense study sheet but not on the conversation, or the
// reverse. Real DOM here, not a sandboxed iframe, so this scales the actual
// font size (via --chat-scale) rather than zooming a whole page.
const CHILD_CHAT_ZOOM_KEY = 'sparkquill.child-chat-zoom'
function readChildChatZoom(): number {
  try {
    const n = Number(localStorage.getItem(CHILD_CHAT_ZOOM_KEY))
    return CHILD_ZOOM_STEPS.includes(n) ? n : 1
  } catch { return 1 }
}
function persistChildChatZoom(z: number) {
  try { localStorage.setItem(CHILD_CHAT_ZOOM_KEY, String(z)) } catch { /* best-effort */ }
}

// Dark/light theme — follows the OS/browser's own preference (or a
// previously-stored explicit choice, from when there was an in-app toggle).
// No in-app toggle for now; kept read-only.
type Theme = 'light' | 'dark'
const THEME_KEY = 'sparkquill.theme'

// "This Week" tab types — mirror week.go's ScheduleEntry/ActivityLogEntry/
// SchoolDeadline/weekResponse Go structs exactly (JSON field names match).
type ScheduleEntry = { day: string; start: string; end: string; label: string }
type WeekActivityEntry = { date: string; activity_dir: string; title: string }
type WeekDeadline = { title: string; subject?: string; due_date?: string; kind?: string }
type WeekDay = { date: string; weekday: string; schedule?: ScheduleEntry[]; activities?: WeekActivityEntry[]; deadlines?: WeekDeadline[] }
type WeekResponse = { week_start: string; week_end: string; days: WeekDay[]; upcoming_deadlines?: WeekDeadline[] }
function readTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_KEY)
    if (stored === 'light' || stored === 'dark') return stored
  } catch { /* best-effort */ }
  return (typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light'
}

// rewriteRelativeAssetURLs fixes a bug in how generated HTML pages are
// previewed: the viewer renders HTML via <iframe srcDoc={...}> (raw markup
// injected directly, not loaded as a real document at its own URL), so it has
// no base URL matching the file's actual folder on disk. A page that does
// exactly what its own skill tells it to — save an illustration next to the
// page and reference it with a plain relative `<img src="foo.png">` — silently
// fails to load the image (the browser resolves "foo.png" against the SPA's
// own URL, gets a 404, and falls back to showing the alt text in its place).
// Rewrites bare-relative src="..." references (skipping absolute/data/anchor
// URLs, which are already fine) into the real /api/workspace/raw endpoint for
// that exact file, resolved against the HTML file's own directory.
// rewriteImgSrcsRelativeTo resolves every relative src="..." in html against
// dir and points it at the raw-file API — the one place this actually
// happens, shared by the full-page viewer and a show_scene snippet alike.
function rewriteImgSrcsRelativeTo(html: string, dir: string): string {
  return html.replace(/\bsrc=(["'])(.*?)\1/gi, (whole, quote: string, ref: string) => {
    if (/^(https?:)?\/\//i.test(ref) || ref.startsWith('/') || ref.startsWith('data:') || ref.startsWith('#')) return whole
    const resolved = dir ? `${dir}/${ref}` : ref
    const url = `${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(resolved)}`
    return `src=${quote}${url}${quote}`
  })
}

function rewriteRelativeAssetURLs(html: string, filePath: string): string {
  if (!/^\s*<(!doctype|html)/i.test(html)) return html
  const dir = filePath.includes('/') ? filePath.slice(0, filePath.lastIndexOf('/')) : ''
  return rewriteImgSrcsRelativeTo(html, dir)
}

// withSceneResizeScript appends a tiny bootstrap script to a show_scene
// snippet so the iframe reports its own content height via postMessage —
// SceneFrame below sizes itself to that instead of a fixed guess, so content
// never gets clipped regardless of how tall a given scene turns out to be.
// (contentWindow.scrollHeight can't be read from the outside — the iframe is
// a cross-origin sandboxed frame — so the height has to self-report.)
function withSceneResizeScript(html: string): string {
  return html + `
<script>(function(){
  function report(){
    var h = document.documentElement.scrollHeight;
    parent.postMessage({ __sq: 1, op: 'scene-resize', height: h }, '*');
  }
  window.addEventListener('load', report);
  if (window.ResizeObserver) new ResizeObserver(report).observe(document.documentElement);
  setTimeout(report, 50);
  // Same SQ.choose as the shared full-page template (skills/_shared/html-design.md)
  // — a scene snippet is a SEPARATE srcDoc document, so it doesn't inherit that
  // page's own <script>; without this, a scene's SQ.choose button would throw
  // "SQ is not defined" the instant it's clicked. Disables the button immediately
  // so a slow reply can't be mistaken for a missed tap and answered twice.
  window.SQ = { choose: function (text, el) {
    if (el && el.disabled) return;
    if (el) el.disabled = true;
    parent.postMessage({ __sq: 1, op: 'choose', text: text }, '*');
  } };
})();</script>`
}

// withViewerPositionScript keeps the child's place in her worksheet across the
// re-opens the tutor triggers, and jumps to a specific question only when the
// tutor deliberately asks for one.
//
// Why it has to run inside the frame: the viewer iframe is sandboxed
// allow-scripts WITHOUT allow-same-origin, so it is cross-origin and the parent
// can neither read nor write its scroll position. An earlier attempt restored
// scroll from outside via contentWindow.scrollY, which silently returned 0 every
// time (the read throws and is caught) — that is why re-opening a page after the
// tutor recorded an answer always jumped back to the top.
//
// So the script reports its own scroll position out to the app as it changes, and
// the app hands the last known offset back in on the next load. The iframe is
// recreated whenever srcDoc changes, so the value has to live outside it.
//
// Priority, highest first:
//  1. focusId — open_file's explicit `focus`. A deliberate act by the tutor
//     ("let's look at that money one again"), so it outranks her scroll position.
//  2. savedY — where she actually was. This is the common case: the tutor records
//     an answer, re-opens the file to refresh it, and she should not lose her
//     place. Restoring a position is strictly better than guessing a question,
//     which is why the old "scroll to the first unanswered question" behaviour was
//     dropped: it moved the page under her whenever she was reading elsewhere.
//  3. Nothing — a genuinely new file opens at the top, as expected.
function withViewerPositionScript(html: string, focusId?: string, savedY = 0, zoom = 1): string {
  const wanted = (focusId ?? '').replace(/[^A-Za-z0-9_-]/g, '')
  // Zoom shrinks the effective viewport, so a page whose .wrap is capped at a
  // fixed px width would overflow sideways instead of reflowing. Releasing that
  // cap lets the content just fill the pane at any size.
  //
  // Applied to <html>, NOT <body> — confirmed live: zooming body left the page
  // completely stuck, unable to scroll past whatever fit in one un-zoomed
  // screen's worth of height. Chromium's `zoom` grows the zoomed element's own
  // rendered box, but doesn't reliably widen its PARENT's scrollable extent to
  // match — so with body zoomed, <html> (the actual scrolling element here)
  // kept thinking the document was only as tall as it was at 100%, and
  // silently clipped everything past that with no way to reach it. Zooming
  // the root itself sidesteps the mismatch: there's no parent left to fall
  // out of sync with.
  const zoomStyle = zoom === 1 ? '' : `
<style>
  html { zoom: ${zoom}; }
  .wrap { max-width: none !important; }
</style>`
  return html + zoomStyle + `
<script>(function(){
  var wanted = ${JSON.stringify(wanted)};
  var savedY = ${Math.max(0, Math.round(savedY))};
  function restore(){
    try {
      var target = wanted ? document.getElementById(wanted) : null;
      if (target && !target.classList.contains('q')) {
        // Allow pointing at anything with an id (a study-sheet section), but
        // prefer its enclosing question so the highlight frames the whole thing.
        target = target.closest('.q') || target;
      }
      if (target) {
        target.scrollIntoView({ behavior: 'auto', block: 'center' });
        target.classList.add('is-current');
        setTimeout(function(){ target.classList.remove('is-current'); }, 2600);
        return;
      }
      if (savedY > 0) window.scrollTo(0, savedY);
    } catch (e) { /* never break the page over a nice-to-have */ }
  }
  window.addEventListener('load', restore);
  setTimeout(restore, 60);
  // Report position out so the next load can restore it. Throttled via rAF:
  // scroll fires far too often to postMessage on every event.
  var pending = false;
  window.addEventListener('scroll', function(){
    if (pending) return;
    pending = true;
    requestAnimationFrame(function(){
      pending = false;
      parent.postMessage({ __sq: 1, op: 'viewer-scroll', y: window.scrollY }, '*');
    });
  }, { passive: true });
})();</script>
<style>
  /* A brief, calm pulse so she can see WHERE the page landed when the tutor
     pointed at a specific question. Respects reduced-motion. */
  .q.is-current{animation:sqFocus 2.6s ease-out both}
  @keyframes sqFocus{
    0%{background:#fdeecb;box-shadow:0 0 0 6px #fdeecb}
    100%{background:transparent;box-shadow:0 0 0 6px transparent}
  }
  @media (prefers-reduced-motion:reduce){
    .q.is-current{animation:none;background:#fdeecb}
  }
</style>`
}

// StartBurst is the moment the child begins an activity: a ring of stars flies
// out from the centre, spins, and fades. Short (1.5s) and non-blocking — it sits
// over the screen with pointer-events: none so it can never swallow a tap, and it
// removes itself when finished.
//
// Two things here are deliberate, both learned from getting it wrong:
//
//  1. The timer is armed ONCE. onDone is a fresh closure on every parent render,
//     so depending on it re-armed the timeout continuously — and a child turn
//     re-renders on every streamed delta, so the burst never removed itself and
//     the stars just sat on the page. The callback is held in a ref instead.
//  2. Each star's direction is a static inline transform on a wrapper, NOT a
//     custom property read inside @keyframes. Animating a transform built from
//     var() is fragile; keeping the keyframe free of var() means the only thing
//     animating is a plain translate/rotate/scale, which always works.
//
// Purely decorative, so it is skipped entirely under prefers-reduced-motion
// rather than shown frozen.
function StartBurst({ onDone }: { onDone: () => void }) {
  const doneRef = useRef(onDone)
  doneRef.current = onDone
  useEffect(() => {
    const t = window.setTimeout(() => doneRef.current(), 1500)
    return () => window.clearTimeout(t)
  }, [])
  const count = 10
  return (
    <div className="fl-start-burst" aria-hidden="true">
      {Array.from({ length: count }, (_, i) => (
        <span
          key={i}
          className="fl-start-burst-ray"
          style={{ transform: `rotate(${(360 / count) * i}deg)` }}
        >
          <Star
            className="fl-start-burst-star"
            size={i % 3 === 0 ? 30 : 20}
            fill="currentColor"
            strokeWidth={1}
            style={{ animationDelay: `${(i % 5) * 0.06}s` }}
          />
        </span>
      ))}
      <Sparkles className="fl-start-burst-core" size={54} />
    </div>
  )
}

// ToolCallSummary shows what the agent actually called this turn — collapsed
// to "N tools" by default so a busy turn doesn't clutter the thread; expand to
// see each call, then click one to see its actual response. Used for BOTH the
// live in-flight indicator (calls have no result/err yet — still streaming)
// and the final persisted summary (result/err filled in once the turn
// completes) — same shape, so nothing has to change when it switches over.
function ToolCallSummary({ calls }: { calls: DebugToolCall[] }) {
  const [open, setOpen] = useState(false)
  const [openIdx, setOpenIdx] = useState<number | null>(null)
  if (calls.length === 0) return null
  return (
    <div className="fl-tool-summary">
      <button type="button" className="fl-tool-summary-toggle" onClick={() => setOpen((v) => !v)}>
        🔧 {calls.length} tool{calls.length === 1 ? '' : 's'} <span className="fl-tool-summary-caret">{open ? '▲' : '▼'}</span>
      </button>
      {open && (
        <div className="fl-tool-summary-list">
          {calls.map((c, i) => (
            <div key={i} className="fl-tool-call">
              <button type="button" className="fl-tool-call-row" onClick={() => setOpenIdx((cur) => (cur === i ? null : i))}>
                <span className="fl-tool-call-name">{c.tool}</span>
                {c.args && <span className="fl-tool-call-args">{c.args}</span>}
              </button>
              {openIdx === i && (
                <pre className="fl-tool-call-response">{c.err ? `Error: ${c.err}` : (c.result || '(still running…)')}</pre>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// SceneFrame renders one show_scene snippet, auto-sized to its actual content
// height (see withSceneResizeScript) instead of a fixed height that would
// either clip taller scenes or leave dead space under shorter ones. Each
// instance only reacts to resize reports from its OWN iframe (matched via
// the message event's source window), so multiple scenes in the same thread
// don't interfere with each other.
// activityDir: the current activity folder, so a find_image picture the tutor
// references with a plain relative `<img src="filename.png">` resolves — the
// scene's own srcDoc has no such path otherwise, so the image silently
// rendered as a broken-image glyph (confirmed live: the tutor called
// find_image, got a real picture back, and it still never appeared).
// SCENE_MAX_HEIGHT bounds one scene's footprint in the scrolling chat feed —
// not a content cap (nothing above it is lost: the iframe scrolls internally
// by default, and no ancestor sets overflow:hidden), just how tall a single
// inline turn is allowed to make itself before the rest of the conversation
// gets pushed out of view. Raised from the original 520 now that scenes are
// meant to include real interactivity (games, simulations), which genuinely
// wants more room than a passive diagram did.
const SCENE_MAX_HEIGHT = 720

function SceneFrame({ html, activityDir }: { html: string; activityDir: string }) {
  const ref = useRef<HTMLIFrameElement>(null)
  const [rawHeight, setRawHeight] = useState(160)
  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      if (e.source !== ref.current?.contentWindow) return
      const m = e.data
      if (m && typeof m === 'object' && m.__sq === 1 && m.op === 'scene-resize' && typeof m.height === 'number') {
        setRawHeight(Math.max(m.height, 80))
      }
    }
    window.addEventListener('message', onMsg)
    return () => window.removeEventListener('message', onMsg)
  }, [])
  const resolved = rewriteImgSrcsRelativeTo(html, activityDir)
  // A child should never have to discover a cut-off scene by noticing a faint
  // scrollbar — when content is genuinely taller than the cap, say so visibly.
  const clipped = rawHeight > SCENE_MAX_HEIGHT
  return (
    <div className="fl-scene-card">
      <iframe ref={ref} className="fl-scene-frame" title="Scene" sandbox="allow-scripts" style={{ height: Math.min(rawHeight, SCENE_MAX_HEIGHT) }} srcDoc={withSceneResizeScript(resolved)} />
      {clipped && <div className="fl-scene-more" aria-hidden="true">scroll for more ↓</div>}
    </div>
  )
}

// ActivityItemPreview shows an activity item's actual content — HTML in a
// sandboxed iframe, Markdown rendered in-page — at real (readable) size, in a
// short scrollable box, so a parent can read a test/guide's real content in
// place rather than just a filename. Falls back to a plain icon+name row
// while loading, or for anything else (images/PDF already have their own
// list glyph, and aren't a page to peek into). `large` gives it noticeably
// more room — used when it's the only item in the activity, so there's
// nothing else competing for space and more of the real content shows at once.
function ActivityItemPreview({ path, name, large }: { path: string; name: string; large?: boolean }) {
  const [content, setContent] = useState<{ kind: 'html' | 'md'; text: string } | null>(null)
  useEffect(() => {
    let cancelled = false
    setContent(null)
    fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent(path)}`)
      .then((res) => res.json())
      .then((d: { content?: string }) => {
        if (cancelled) return
        const raw = d.content ?? ''
        if (/^\s*<(!doctype|html)/i.test(raw)) setContent({ kind: 'html', text: rewriteRelativeAssetURLs(raw, path) })
        else if (/\.(md|markdown)$/i.test(path)) setContent({ kind: 'md', text: raw })
        else setContent(null)
      })
      .catch(() => { if (!cancelled) setContent(null) })
    return () => { cancelled = true }
  }, [path])
  if (!content) {
    return (
      <div className="fl-file-item-row">
        <FileGlyph name={name} size={15} />
        <span>{labelFromFilename(name).label}</span>
      </div>
    )
  }
  return (
    <div className={`fl-item-preview${large ? ' is-large' : ''}`}>
      {content.kind === 'html' ? (
        <iframe className="fl-item-preview-frame" title="" sandbox="" srcDoc={content.text} />
      ) : (
        <div className="fl-item-preview-md"><Markdown text={content.text} /></div>
      )}
    </div>
  )
}

// workspaceRelativePath converts a link the agent wrote pointing at a
// workspace file — usually a full filesystem path like
// "/Users/x/.sunlit-learning/workspace/Math/Fractions/foo.md", occasionally
// already-relative like "Math/Fractions/foo.md" — into the workspace-relative
// form the viewer (/api/workspace/file?path=...) expects. Returns null for
// anything that isn't a workspace file link (http(s) links, mailto, anchors,
// absolute web paths), so those keep their normal browser behavior. Subject
// names are arbitrary (not a fixed set of roots like the old shared/parent/
// child split), so anything schemeless and non-absolute is treated as a
// workspace-relative path rather than matching a fixed prefix list.
function workspaceRelativePath(href: string): string | null {
  const marker = '/workspace/'
  const i = href.lastIndexOf(marker)
  if (i !== -1) return href.slice(i + marker.length)
  if (/^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith('#') || href.startsWith('/')) return null
  return href
}

// ChatLink intercepts clicks on links the agent wrote pointing at a
// workspace file so they open in the right-side viewer in-app, instead of
// performing a real browser navigation to a raw filesystem path (which the
// dev/packaged server can't serve, and which was breaking "open this file"
// requests). Anything else (real http(s) links) behaves normally.
function ChatLink({ href, children }: { href?: string; children?: React.ReactNode }) {
  const setDrawerTab = useWorkspaceStore((s) => s.setDrawerTab)
  const setViewerPath = useWorkspaceStore((s) => s.setViewerPath)
  const setViewerImageList = useWorkspaceStore((s) => s.setViewerImageList)
  const setViewerRefreshKey = useWorkspaceStore((s) => s.setViewerRefreshKey)
  const rel = href ? workspaceRelativePath(href) : null
  if (rel) {
    return (
      <a
        href={href}
        onClick={(e) => {
          if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return // let cmd/ctrl/middle-click etc. behave normally
          e.preventDefault()
          setDrawerTab('files')
          setViewerImageList([])
          setViewerPath(rel)
          setViewerRefreshKey((k) => k + 1)
        }}
      >
        {children}
      </a>
    )
  }
  return <a href={href} target="_blank" rel="noreferrer">{children}</a>
}

// MermaidDiagram renders a ```mermaid fenced block as an actual diagram —
// ported from AgentWorks' MarkdownRenderer.tsx (self-contained there, no
// store coupling, so this is a near-verbatim copy).
let mermaidCounter = 0
function MermaidDiagram({ content }: { content: string }) {
  const [svg, setSvg] = useState('')
  const [error, setError] = useState('')
  const idRef = useRef(`mermaid-${mermaidCounter++}`)

  const renderDiagram = useCallback(async () => {
    try {
      const mermaid = await loadMermaid()
      mermaid.initialize({ startOnLoad: false, theme: readTheme() === 'dark' ? 'dark' : 'default', securityLevel: 'loose' })
      const { svg: renderedSvg } = await mermaid.render(idRef.current, content)
      setSvg(renderedSvg)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to render mermaid diagram')
      setSvg('')
    }
  }, [content])

  useEffect(() => { renderDiagram() }, [renderDiagram])

  if (error) {
    return (
      <div className="fl-mermaid-error">
        <div>Diagram error</div>
        <pre>{error}</pre>
      </div>
    )
  }
  if (!svg) return <div className="fl-mermaid-loading">Rendering diagram…</div>
  return <div className="fl-mermaid" dangerouslySetInnerHTML={{ __html: svg }} />
}

// stabilizeStreamingMarkdown hides markdown syntax that hasn't finished arriving.
//
// A stream delivers "**bo" before "**bold**", and react-markdown correctly
// renders an unmatched "**" as literal asterisks — so mid-stream the reply
// flickers raw markdown characters (**, `, _, #) that vanish once the closing
// token lands. The text is never wrong, it just looks broken while it types.
//
// So for the STREAMING bubble only, drop a trailing token that is still
// unbalanced. Applied to the live preview, never to the stored message — the
// final render always gets the untouched text.
function stabilizeStreamingMarkdown(text: string): string {
  let out = text
  // An odd count means the run that's still open is the last one; cut from there.
  for (const token of ['**', '`', '*', '_']) {
    const parts = out.split(token)
    if (parts.length > 1 && (parts.length - 1) % 2 === 1) {
      out = parts.slice(0, -1).join(token)
    }
  }
  // A heading or list marker alone on the final line has no content yet.
  return out.replace(/\n[#>\-*+]+[ \t]*$/, '')
}

// Markdown has no native color syntax at all, but the model writes raw HTML
// spans very fluently (it's an extremely common real-world pattern in the
// GitHub-flavored markdown this kind of model is trained on) — far more
// natural than inventing a bespoke {color}text{/color} syntax it would only
// ever use if the prompt keeps reminding it to. rehype-raw parses that raw
// HTML into real nodes; rehype-sanitize (extended to allow `style` on
// `span`, which the default schema doesn't) strips anything actually
// dangerous (script tags, event handlers, iframes, etc.); this schema and
// the plugin below narrow what survives specifically to color.
const colorSpanSchema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    span: [...(defaultSchema.attributes?.span ?? []), 'style'],
  },
}

// A style attribute passing rehype-sanitize's schema is still an ARBITRARY
// CSS declaration list (sanitize only allowlists which ATTRIBUTES exist, not
// their values) — position/overlay/pointer-events tricks are a real UI-
// spoofing concern even without script execution. This keeps only a `color`
// declaration and discards everything else in the same style attribute, so
// a span can change text color and nothing more.
function restrictSpanStyleToColor() {
  return (tree: HastElement) => {
    visit(tree, 'element', (node: HastElement) => {
      if (node.tagName !== 'span' || typeof node.properties?.style !== 'string') return
      const match = /color\s*:\s*([#a-zA-Z0-9(),.\s%]+)/i.exec(node.properties.style)
      if (match) {
        node.properties.style = `color: ${match[1].trim()}`
      } else {
        delete node.properties.style
      }
    })
  }
}

// Markdown renders the agent's reply with react-markdown + GFM — the same
// battle-tested renderer the main AgentWorks frontend uses (handles tables,
// nested lists, lazy-continuation of terminal-wrapped list items, mermaid
// diagrams, and syntax-highlighted code, etc.). Also allows a narrow,
// sanitized `<span style="color:...">` for colored text — see
// colorSpanSchema/restrictSpanStyleToColor above for why and how it's scoped.
function Markdown({ text }: { text: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeRaw, [rehypeSanitize, colorSpanSchema], restrictSpanStyleToColor]}
      components={{
        a: ChatLink,
        // The `code` renderer below returns its own fully-formed element for
        // fenced blocks (a MermaidDiagram, a Suspense-wrapped
        // SyntaxHighlightedCode, or a styled <pre>) — react-markdown's
        // default `pre` would otherwise wrap that a second time, so hand
        // back children as-is here.
        pre({ children }) {
          return <>{children}</>
        },
        code(props) {
          const { className, children, ...rest } = props as { className?: string; children?: React.ReactNode; node?: unknown }
          const match = /language-(\w+)/.exec(className || '')
          const isInline = !match && !String(children).includes('\n')
          if (isInline) {
            return <code className="fl-inline-code" {...rest}>{children}</code>
          }
          const codeString = String(children).replace(/\n$/, '')
          if (!match) {
            return <pre className="fl-code-block-plain">{codeString}</pre>
          }
          const language = match[1]
          if (language === 'mermaid') return <MermaidDiagram content={codeString} />
          if (['text', 'txt', 'plain', 'plaintext', 'terminal'].includes(language.toLowerCase())) {
            return <pre className="fl-code-block-plain">{codeString}</pre>
          }
          return (
            <Suspense fallback={<pre className="fl-code-block-plain">{codeString}</pre>}>
              <SyntaxHighlightedCode code={codeString} language={language} isDark={readTheme() === 'dark'} />
            </Suspense>
          )
        },
      }}
    >
      {text}
    </ReactMarkdown>
  )
}

// QUICK_SKILLS are one-click shortcuts in the composer menu; each sends a message
// that triggers the matching agent skill.
const QUICK_SKILLS = [
  { label: 'Create study material', message: 'Create study material for my child — follow your create-study-material skill and make it a designed, static (view-only) HTML page.' },
  { label: 'Create a practice test', message: 'Create a practice test for my child — follow your create-test skill: an interactive HTML page that records my child’s typed answers, plus a separate answer key for me.' },
  { label: 'Update progress report', message: 'Build an updated progress report — follow your create-progress-report skill, make it a designed HTML page, and give me a short coach-style read of the evidence here in chat too.' },
  { label: 'Update academic map', message: 'Update the academic map — follow your create-academic-map skill (designed HTML at reports/academic-map.html).' },
  { label: 'Back up workspace', message: 'Back up my workspace now — follow your backup skill.' },
]

// PARENT_WAIT_HINTS / CHILD_WAIT_HINTS cycle in the "thinking" indicator while
// waiting for a reply with no live tool-status yet — real, usable tips on how
// to use the chat, shown instead of a bare "thinking…" so the wait is at
// least a little useful. Live tool status (e.g. "Opening the file…") always
// takes priority over these when it's available.
const PARENT_WAIT_HINTS = [
  'Tip: ask "How is my child doing so far?" anytime for an evidence-based read of their progress.',
  'Tip: once a test or guide is ready, use the "Give to child" button to hand it over.',
  'Tip: tell Quill how you want tutoring handled — e.g. "give one hint before the answer" — and it remembers.',
  'Tip: ask for several things at once — "make a guide, a quick test, and an advanced one" — bundled as one activity.',
  'Tip: Quill can look up board-specific tips and exam strategies — just ask.',
  'Tip: you can ask Quill to explain a topic to you, not just make material for your child.',
  'Tip: link WhatsApp in Connectors to chat with Quill from your phone, and get check-ins there.',
  'Tip: set up the Browser connector and Quill can peek at your school portal for new assignments.',
  'Tip: turn on Pulse (top bar) and Quill checks in on its own — reviewing progress and the school portal.',
  'Tip: just mention things in passing — "her exam is next Friday", "she gets anxious with timers" — Quill remembers and applies them later.',
]
const CHILD_WAIT_HINTS = [
  'Tip: stuck? Just say "give me a hint!"',
  'Tip: you can ask Quill to explain it a different way.',
  'Tip: tell Quill your answer — it will tell you if you got it right.',
  'Tip: ask for an example if a question feels tricky.',
  'Tip: you can ask Quill anything about what you\'re learning, not just the current question.',
  'Tip: ask a parent to set up WhatsApp so you can practice with Quill on the phone too!',
  'Tip: ask a parent to turn on Pulse so Quill keeps track of how you\'re doing.',
  'Tip: ask a parent to connect the school portal so Quill can help with your assignments.',
]

// CHILD_QUICK_ACTIONS: fixed one-tap shortcuts for the handful of requests that
// come up constantly but aren't worth typing out — same Sparkles-icon popover
// pattern as QUICK_SKILLS in Parent Mode. Unlike suggest_actions (removed from
// Child Mode entirely), these are deliberately NOT model-generated: a static
// list of common asks, always the same, always available. Tapping one just
// sends its message exactly as if she'd typed it — the tutor's own judgment
// (already covered by its existing prompt) decides what to actually do, same
// as if she'd typed the words herself. Add more here freely; nothing else
// needs to change.
const CHILD_QUICK_ACTIONS: { label: string; message: string }[] = [
  { label: 'Update answers for print', message: "Please update all my answered questions on this page so it's ready to print." },
  { label: 'No more hints', message: "Don't give me any more hints — just tell me if I'm right or wrong." },
  { label: 'Harder question', message: 'Can you give me a harder question?' },
  { label: 'Easier question', message: 'Can you give me an easier question?' },
  { label: 'Give me a hint', message: 'Can you give me a hint?' },
  { label: 'Keep quizzing me, harder each time', message: 'Keep asking me progressively harder questions on this until I fully understand it.' },
  { label: 'One section at a time', message: "Let's go one section at a time — keep quizzing me on this section until I really understand it before moving to the next." },
]

// formatBytes renders a byte count the way a person reads it. Sizes here are
// for keeping an eye on how the workspace grows, so one decimal past KB is
// plenty of precision.
function formatBytes(bytes?: number): string {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let i = 0
  while (value >= 1024 && i < units.length - 1) { value /= 1024; i++ }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[i]}`
}

// FileTree renders the workspace as an expandable tree (AgentWorks-style). Files
// are clickable to open in the viewer; .meta.json is hidden as noise. Each entry
// carries its size on disk — a folder's is the recursive total — so it's visible
// at a glance which part of the workspace is actually growing.
function FileTree({ nodes, onOpen, depth = 0, expanded, onToggle }: {
  nodes: TreeNode[]
  onOpen: (path: string) => void
  depth?: number
  expanded: Record<string, boolean>
  onToggle: (path: string, open: boolean) => void
}) {
  const visible = nodes.filter((n) => !n.name.startsWith('.') && !n.name.endsWith('.meta.json'))
  if (visible.length === 0) return null
  return (
    <ul className="fl-tree">
      {visible.map((n) => (
        <li key={n.path}>
          {n.type === 'dir' ? (
            <details
              open={expanded[n.path] ?? depth < 1}
              onToggle={(e) => onToggle(n.path, e.currentTarget.open)}
            >
              <summary className="fl-tree-dir">
                <Folder className="fl-tree-icon is-closed" size={15} />
                <FolderOpen className="fl-tree-icon is-open" size={15} />
                <span>{n.name}</span>
                <span className="fl-tree-size">{formatBytes(n.size)}</span>
              </summary>
              {n.children && <FileTree nodes={n.children} onOpen={onOpen} depth={depth + 1} expanded={expanded} onToggle={onToggle} />}
            </details>
          ) : (
            <button className="fl-tree-file" type="button" onClick={() => onOpen(n.path)}>
              <FileGlyph name={n.name} size={14} />
              <span>{n.name}</span>
              <span className="fl-tree-size">{formatBytes(n.size)}</span>
            </button>
          )}
        </li>
      ))}
    </ul>
  )
}
const IMAGE_PATH_RE = /\.(png|jpe?g|gif|webp|svg|bmp)$/i

// isPrintable — the viewer shows a print button for documents worth printing:
// HTML pages (tests, study material, reports) and Markdown (which some tests /
// package items are). Images/PDFs use the browser's own controls; other files
// aren't previewed.
function isPrintable(path: string): boolean {
  return /\.(html?|md|markdown)$/i.test(path)
}

// formatJSONText pretty-prints a .json/.jsonl file's raw text for the viewer;
// falls back to the raw text unchanged if it doesn't parse (e.g. JSONL).
function formatJSONText(content: string): string {
  try {
    return JSON.stringify(JSON.parse(content), null, 2)
  } catch {
    return content
  }
}

// printFile prints a viewer file. HTML opens in a new tab that auto-prints
// (?print=1 on the raw endpoint) — robust: it doesn't depend on the generated
// HTML embedding a print handler (a skill can forget to) and isn't blocked by
// the viewer iframe's sandbox. Markdown/text is rendered in-page as React, so it
// falls back to printViewerContent (CSS-isolated window.print).
function printFile(path: string) {
  if (/\.(html?)$/i.test(path)) {
    window.open(`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(path)}&print=1`, '_blank', 'noopener,noreferrer')
  } else {
    printViewerContent()
  }
}

// printViewerContent prints the open viewer's in-page rendered content (Markdown
// / plain text) by flagging the document and letting an @media print rule
// isolate .fl-viewer-md / .fl-viewer-pre before printing.
function printViewerContent() {
  const root = document.documentElement
  root.classList.add('fl-printing')
  const cleanup = () => { root.classList.remove('fl-printing'); window.removeEventListener('afterprint', cleanup) }
  window.addEventListener('afterprint', cleanup)
  window.print()
}

// FileGlyph renders a file-type icon coloured by extension, so the workspace
// shows a PDF/Word/PowerPoint/Excel/image/archive at a glance rather than one
// generic sheet-of-paper for everything.
function fileGlyphFor(name: string): { Icon: typeof FileText; kind: string } {
  const ext = (name.split('.').pop() || '').toLowerCase()
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'heic'].includes(ext)) return { Icon: ImageIcon, kind: 'image' }
  if (ext === 'pdf') return { Icon: FileType, kind: 'pdf' }
  if (['doc', 'docx', 'rtf', 'odt'].includes(ext)) return { Icon: FileText, kind: 'doc' }
  if (['ppt', 'pptx', 'odp'].includes(ext)) return { Icon: Presentation, kind: 'ppt' }
  if (['xls', 'xlsx', 'csv', 'ods', 'tsv'].includes(ext)) return { Icon: FileSpreadsheet, kind: 'sheet' }
  if (['zip', 'tar', 'gz', 'tgz', 'rar', '7z'].includes(ext)) return { Icon: FileArchive, kind: 'zip' }
  if (['html', 'htm'].includes(ext)) return { Icon: FileCode, kind: 'html' }
  if (['mp4', 'mov', 'avi', 'mkv', 'webm'].includes(ext)) return { Icon: Film, kind: 'video' }
  if (['mp3', 'wav', 'm4a', 'aac', 'ogg', 'flac'].includes(ext)) return { Icon: Music, kind: 'audio' }
  return { Icon: FileText, kind: 'file' }
}
function FileGlyph({ name, size = 16 }: { name: string; size?: number }) {
  const { Icon, kind } = fileGlyphFor(name)
  return <Icon size={size} className={`fl-glyph fl-glyph-${kind}`} />
}

// FileMetaPanel renders the sidecar metadata (<path>.meta.json) the process-file
// skill writes for a filed document — what Quill understood the file to be. Only
// the parent-meaningful fields are shown, in plain language (no raw JSON, no
// paths), consistent with the rest of the parent UI.
function FileMetaPanel({ meta }: { meta: Record<string, unknown> }) {
  const str = (k: string): string => (typeof meta[k] === 'string' ? (meta[k] as string).trim() : '')
  const concepts = Array.isArray(meta.key_concepts)
    ? (meta.key_concepts as unknown[]).filter((c): c is string => typeof c === 'string' && c.trim() !== '')
    : []
  const summary = str('summary')
  const subject = str('subject')
  const topic = str('topic')
  const type = str('type')
  const chips = [subject, topic, type].filter(Boolean)
  if (!summary && chips.length === 0 && concepts.length === 0) return null
  return (
    <div className="fl-meta-panel">
      <p className="fl-meta-title"><Info size={13} /> What Quill knows about this file</p>
      {chips.length > 0 && (
        <div className="fl-meta-chips">
          {chips.map((c, i) => <span key={i} className="fl-meta-chip">{c}</span>)}
        </div>
      )}
      {summary && <p className="fl-meta-summary">{summary}</p>}
      {concepts.length > 0 && (
        <div className="fl-meta-concepts">
          {concepts.map((c, i) => <span key={i} className="fl-meta-concept">{c}</span>)}
        </div>
      )}
    </div>
  )
}

// NonPreviewableFile is shown for files the browser can't display inline (Word,
// PowerPoint, spreadsheets, archives, …). We deliberately don't try to convert
// them — just show what Quill knows about the file (its metadata, if any) and a
// Download button to open it on the device. Keeps preview simple: images, PDFs,
// and HTML/text render inline; everything else is metadata + download.
function NonPreviewableFile({ path, meta }: { path: string; meta: Record<string, unknown> | null }) {
  const name = path.split('/').pop() || path
  return (
    <div className="fl-nopreview">
      <div className="fl-nopreview-head">
        <FileGlyph name={name} size={34} />
        <span className="fl-nopreview-name">{name}</span>
      </div>
      {meta ? (
        <FileMetaPanel meta={meta} />
      ) : (
        <p className="fl-note">This kind of file can’t be shown here — download it to open on your device.</p>
      )}
      <a
        className="fl-download-btn"
        href={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(path)}&download=1`}
        download={name}
      >
        Download
      </a>
    </div>
  )
}

// labelFromFilename turns a bare filename like
// "2026-07-21-fractions-revision-worksheet.md" into a date + human label.
// Filenames are sometimes auto-generated noise (WhatsApp Image ..., s02.png),
// so the label prefers the date-stripped name, falling back to
// "Photo"/"File" for image uploads whose name carries no information at all.
function labelFromFilename(filename: string): { date?: string; label: string } {
  const nameNoExt = filename.replace(/\.[a-z0-9]+$/i, '')
  const dateMatch = nameNoExt.match(/^(\d{4}-\d{2}-\d{2})[-_](.+)$/)
  const date = dateMatch ? dateMatch[1] : undefined
  let rawLabel = (dateMatch ? dateMatch[2] : nameNoExt).replace(/[-_]+/g, ' ').trim()
  if (!rawLabel || /^(whatsapp image|img\d*|s\d+|image\d*|photo\d*)\b/i.test(rawLabel)) {
    rawLabel = IMAGE_PATH_RE.test(filename) ? 'Photo' : 'File'
  }
  return { date, label: rawLabel }
}

// parseMaterialPath reads a materials/<subject>/<topic>/<file> path (the only
// remaining path shape the UI needs to reverse-derive subject/topic from —
// generated content now lives in Activity objects, which already carry their
// own subject/topic/items instead of encoding them in a path).
function parseMaterialPath(p: string): { subject?: string; topic?: string; date?: string; label: string } {
  const parts = p.split('/')
  const rest = parts.slice(1) // drop "materials"
  const filename = rest[rest.length - 1] || p
  const subject = rest.length >= 1 ? rest[0] : undefined
  const topic = rest.length >= 3 ? rest[1] : undefined
  return { subject, topic, ...labelFromFilename(filename) }
}

export default function LearningApp() {
  const [theme] = useState<Theme>(readTheme)

  const screen = useSetupStore((s) => s.screen)
  const setScreen = useSetupStore((s) => s.setScreen)
  const engines = useSetupStore((s) => s.engines)
  const setEngines = useSetupStore((s) => s.setEngines)
  const enginesState = useSetupStore((s) => s.enginesState)
  const setEnginesState = useSetupStore((s) => s.setEnginesState)
  const engine = useSetupStore((s) => s.engine)
  const setEngine = useSetupStore((s) => s.setEngine)
  const testState = useSetupStore((s) => s.testState)
  const setTestState = useSetupStore((s) => s.setTestState)
  const testMessage = useSetupStore((s) => s.testMessage)
  const setTestMessage = useSetupStore((s) => s.setTestMessage)

  useEffect(() => {
    let cancelled = false
    setEnginesState('loading')
    fetch(`${FAMILY_API}/api/engines`)
      .then((res) => { if (!res.ok) throw new Error(String(res.status)); return res.json() })
      .then((data: ApiEngine[]) => {
        if (cancelled) return
        const sorted = [...data].sort((a, b) => pres(a.id, a.name).order - pres(b.id, b.name).order)
        setEngines(sorted)
        // Only supply a DEFAULT for a genuinely fresh install — never override an
        // already-known choice. This used to fire unconditionally, racing the
        // /api/setup effect below (which loads the family's real saved engine):
        // /api/setup is a fast state-file read, while this call does live runtime
        // detection across four CLIs and reliably resolves second, so it was
        // silently clobbering the real selection back to whichever engine happens
        // to rank first in ENGINE_PRESENTATION's hardcoded order — confirmed live,
        // every actual conversation kept running on the real saved engine (its own
        // session is pinned to it independent of this picker) while Settings showed
        // a completely different one as "active". Read the CURRENT store value at
        // call time, not a value captured when this effect was created, since
        // /api/setup may resolve either before or after this one.
        if (!useSetupStore.getState().engine) {
          const firstReady = sorted.find((item) => item.usable) ?? sorted[0]
          if (firstReady) setEngine(firstReady.id)
        }
        setEnginesState('ready')
      })
      .catch(() => { if (!cancelled) setEnginesState('error') })
    return () => { cancelled = true }
  }, [setEngine, setEngines, setEnginesState])
  const childName = useFamilyStore((s) => s.childName)
  const setChildName = useFamilyStore((s) => s.setChildName)
  const grade = useFamilyStore((s) => s.grade)
  const setGrade = useFamilyStore((s) => s.setGrade)
  const board = useFamilyStore((s) => s.board)
  const setBoard = useFamilyStore((s) => s.setBoard)
  const focusInput = useParentChatStore((s) => s.focusInput)
  const setFocusInput = useParentChatStore((s) => s.setFocusInput)
  const parentMessages = useParentChatStore((s) => s.parentMessages)
  const setParentMessages = useParentChatStore((s) => s.setParentMessages)
  const sending = useParentChatStore((s) => s.sending)
  const setSending = useParentChatStore((s) => s.setSending)
  const liveStatus = useParentChatStore((s) => s.liveStatus)
  const setLiveStatus = useParentChatStore((s) => s.setLiveStatus)
  const streamingReply = useParentChatStore((s) => s.streamingReply)
  const setStreamingReply = useParentChatStore((s) => s.setStreamingReply)
  const suggestions = useParentChatStore((s) => s.suggestions)
  const setSuggestions = useParentChatStore((s) => s.setSuggestions)
  // Rendering the WHOLE parent↔Quill history (one long-running thread, can
  // grow to hundreds of messages over months) made every keystroke in the
  // composer reconcile every bubble in the DOM — visibly laggy typing, and a
  // heavy initial paint. Show only the most recent PARENT_HISTORY_PAGE_SIZE by
  // default; "Load earlier messages" reveals more in the same-size chunks.
  // Growing this only ever reveals OLDER messages — new ones arriving still
  // show immediately since the window is always "last N", not "first N".
  const [parentVisibleCount, setParentVisibleCount] = useState(PARENT_HISTORY_PAGE_SIZE)
  const visibleParentMessages = parentMessages.length > parentVisibleCount ? parentMessages.slice(-parentVisibleCount) : parentMessages
  // Absolute offset of the visible window's first item within the FULL
  // history — used as the React key base so a bubble's identity stays stable
  // across "load more" clicks (which shift every relative index) instead of
  // remounting everything already on screen.
  const hiddenParentCount = parentMessages.length - visibleParentMessages.length
  const parentRenderGroups = groupConsecutivePhotos(visibleParentMessages, hiddenParentCount)
  // Before actually switching into Child Mode, ask the parent whether to
  // continue Myra's existing conversation or start a brand-new one — handing
  // off an activity often means "just carry on the same chat", not a fresh
  // start, so this is the parent's call rather than a silent guess.
  const [pendingChildEntry, setPendingChildEntry] = useState<{ dir: string; greetingText: string } | null>(null)
  // Bumped by every performHandoff call, captured by its own response. If a
  // second "Give to child" fires (a different activity, clicked before the
  // first request finished) before this one's response lands, its generation
  // no longer matches — the response is discarded instead of kicking off a
  // chat for an activity the parent already navigated away from. Without
  // this, whichever request happened to resolve LAST won regardless of which
  // was actually clicked last, since fetch responses aren't guaranteed to
  // arrive in request order.
  const handoffGenerationRef = useRef(0)
  // The workspace's TRUE size on disk, from /api/workspace/tree — including
  // what the listing hides (see workspaceTreeResponse), so the number in the
  // Files tab is the one worth watching for growth.
  const [treeTotalSize, setTreeTotalSize] = useState(0)
  // The current turn's tool calls, live — no result yet (still running), shown
  // as a collapsible "N tools" chip next to the thinking indicator. Reset at
  // turn start; replaced by a persisted debug_summary message (WITH results,
  // from the final response) once the turn completes — see sendChildMessage.
  const [childLiveToolCalls, setChildLiveToolCalls] = useState<DebugToolCall[]>([])
  // Live status/tool calls for a turn on THIS activity that this tab did NOT
  // start itself (e.g. WhatsApp's @child routing running runChildTurn) — the
  // ambient subscription effect below is what actually populates these; see
  // its own comment for why a second, separate stream/state pair is needed
  // rather than reusing childLiveStatus/childLiveToolCalls.
  const [childRemoteStatus, setChildRemoteStatus] = useState('')
  const [childRemoteToolCalls, setChildRemoteToolCalls] = useState<DebugToolCall[]>([])
  const [liveToolCalls, setLiveToolCalls] = useState<DebugToolCall[]>([])
  // Live status/tool calls for a parent-conversation turn THIS tab did NOT
  // start (e.g. a real WhatsApp message running w.runTurn) — see the ambient
  // subscription effect below, mirroring Child Mode's own childRemoteStatus/
  // childRemoteToolCalls.
  const [remoteStatus, setRemoteStatus] = useState('')
  const [remoteToolCalls, setRemoteToolCalls] = useState<DebugToolCall[]>([])
  const menuOpen = useParentChatStore((s) => s.menuOpen)
  const setMenuOpen = useParentChatStore((s) => s.setMenuOpen)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [savingEngine, setSavingEngine] = useState(false)
  // Voice settings — the tier catalog is computed server-side against THIS
  // machine's hardware (see /api/voice/status), so the UI never has to guess
  // what an Intel vs Apple Silicon Mac can actually run.
  const [voiceStatus, setVoiceStatus] = useState<VoiceStatus | null>(null)
  const [goalPopoverOpen, setGoalPopoverOpen] = useState(false)
  // Secrets (credentials the parent saves for Quill's tools, e.g. a school
  // portal login) — settings-form only, never through chat, so a value typed
  // here never touches the model or the persisted conversation transcript.
  const [secretNames, setSecretNames] = useState<string[]>([])
  const [secretNameDraft, setSecretNameDraft] = useState('')
  const [secretValueDraft, setSecretValueDraft] = useState('')
  const [savingSecret, setSavingSecret] = useState(false)
  const [deletingSecret, setDeletingSecret] = useState<string | null>(null)
  const waOpen = useWhatsAppStore((s) => s.waOpen)
  const setWaOpen = useWhatsAppStore((s) => s.setWaOpen)
  const [connectorSection, setConnectorSection] = useState<'whatsapp' | 'browser'>('whatsapp')
  // Multiple phones can be linked (one per parent) — accounts is the list of
  // already-paired numbers; pairing reflects whichever NEW phone's QR is
  // currently being shown (there's always room to add one more).
  const [waStatus, setWaStatus] = useState<{ accounts: { jid: string; connected: boolean }[]; pairing: { qr_available: boolean; qr_expires_at?: string }; voice_transcription?: { enabled: boolean; installed: boolean; installing: boolean; model_size_mb: number; available: boolean; error?: string } } | null>(null)
  const [voiceToggling, setVoiceToggling] = useState(false)
  const [waQrNonce, setWaQrNonce] = useState(0)
  const [unpairingJid, setUnpairingJid] = useState<string | null>(null)
  const [browserStatus, setBrowserStatus] = useState<{ cli_installed: boolean } | null>(null)
  const [browserCopied, setBrowserCopied] = useState(false)
  const [pulseConfig, setPulseConfig] = useState<{ enabled: boolean; cadence_hours: number; last_run_at?: string; watch_sites?: string[]; preferred_hour: number; preferred_hour_set: boolean } | null>(null)
  const [savingPulse, setSavingPulse] = useState(false)
  // "This Week" tab — offset 0 is the current week, -1/+1 step a week at a
  // time. weekData is the combined schedule+activity-log+deadlines response
  // (see week.go); scheduleDraft mirrors it into an editable row list for the
  // mini-editor, only diverging from weekData.schedule while the parent has
  // unsaved edits open.
  const [weekOffset, setWeekOffset] = useState(0)
  const [weekData, setWeekData] = useState<WeekResponse | null>(null)
  const [scheduleEditorOpen, setScheduleEditorOpen] = useState(false)
  const [scheduleDraft, setScheduleDraft] = useState<ScheduleEntry[]>([])
  const [savingSchedule, setSavingSchedule] = useState(false)
  const [watchSitesDraft, setWatchSitesDraft] = useState('')
  const [pulseSaved, setPulseSaved] = useState(false)
  const [pulsePopoverOpen, setPulsePopoverOpen] = useState(false)
  const [pendingConvUpdate, setPendingConvUpdate] = useState<StoredMsg[] | null>(null)
  // Messages the parent typed while a turn was still processing — sent one at
  // a time as the current turn finishes (see the drain effect). Shown as
  // "queued" bubbles so they know it's coming.
  const [queue, setQueue] = useState<string[]>([])
  const [childQueue, setChildQueue] = useState<string[]>([])
  const [pulseRunning, setPulseRunning] = useState(false)
  const [pulseRunError, setPulseRunError] = useState<string | null>(null)
  const wsFiles = useWorkspaceStore((s) => s.wsFiles)
  const setWsFiles = useWorkspaceStore((s) => s.setWsFiles)
  const allFiles = useWorkspaceStore((s) => s.allFiles)
  const setAllFiles = useWorkspaceStore((s) => s.setAllFiles)
  // The parent has ONE ongoing conversation with Quill — web, WhatsApp, and
  // Pulse all share this single "parent" thread (matching the backend's
  // parentConversationID). No multi-conversation list, so the id is fixed.
  const conversationId = 'parent'
  const resumedRef = useRef(false)
  const childResumedRef = useRef(false)
  const childMessages = useChildChatStore((s) => s.childMessages)
  const childRenderGroups = groupConsecutivePhotos(childMessages)
  const setChildMessages = useChildChatStore((s) => s.setChildMessages)
  const childSending = useChildChatStore((s) => s.childSending)
  const setChildSending = useChildChatStore((s) => s.setChildSending)
  const childInput = useChildChatStore((s) => s.childInput)
  const setChildInput = useChildChatStore((s) => s.setChildInput)
  const childLiveStatus = useChildChatStore((s) => s.childLiveStatus)
  const setChildLiveStatus = useChildChatStore((s) => s.setChildLiveStatus)
  const childStreamingReply = useChildChatStore((s) => s.childStreamingReply)
  const setChildStreamingReply = useChildChatStore((s) => s.setChildStreamingReply)
  const parentLabel = useFamilyStore((s) => s.parentLabel)
  const setParentLabel = useFamilyStore((s) => s.setParentLabel)

  const wsRefreshKey = useWorkspaceStore((s) => s.wsRefreshKey)
  const setWsRefreshKey = useWorkspaceStore((s) => s.setWsRefreshKey)
  const treeNodes = useWorkspaceStore((s) => s.treeNodes)
  const setTreeNodes = useWorkspaceStore((s) => s.setTreeNodes)
  // Reflect the workspace file system in the drawer (materials the agent can
  // read). Refetches when entering the chat and after each upload/tool event.
  // The child's own conversation resume lives in a separate effect below,
  // keyed off /api/child/activity instead of scanning the tree — there is no
  // longer a single flat child/conversations/ folder to walk.
  useEffect(() => {
    if (screen !== 'parent' && screen !== 'tutor') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/workspace/tree`)
      .then((res) => res.json())
      .then((data: TreeNode[] | { nodes?: TreeNode[]; total_size?: number }) => {
        if (cancelled) return
        // Accepts both the current {nodes,total_size} object and the older bare
        // array, so a packaged frontend built before the size fields still works
        // against a newer server (and vice versa).
        const nodes = Array.isArray(data) ? data : (data?.nodes ?? [])
        setTreeTotalSize(Array.isArray(data) ? 0 : (data?.total_size ?? 0))
        const files: { path: string; name: string }[] = []
        const walk = (ns: TreeNode[]) => ns?.forEach((n) => {
          if (n.type === 'file') files.push({ path: n.path, name: n.name })
          if (n.children) walk(n.children)
        })
        walk(nodes)
        setTreeNodes(nodes)
        const mats: WsFile[] = files
          .filter((f) => f.path.includes('/materials/') && !f.name.endsWith('.meta.json'))
          .map((f) => {
            const parts = f.path.split('/')
            const mi = parts.indexOf('materials')
            return { path: f.path, name: f.name, scope: parts[0] || '', subject: parts[mi + 1] || '', topic: parts[mi + 2] || '' }
          })
        setWsFiles(mats)
        setAllFiles(files.map((f) => f.path))
        // Resume the single parent conversation (once) so the parent continues
        // where they left off — including anything that arrived via WhatsApp or
        // Pulse, since it's all the same thread now.
        if (!resumedRef.current && parentMessages.length === 0) {
          resumedRef.current = true
          fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent('conversations/parent.json')}`)
            .then((r) => r.json())
            .then((dd) => {
              if (!dd?.content) return
              const c = JSON.parse(dd.content) as { messages?: StoredMsg[] }
              const loaded = (c.messages || []).map(toParentMsg)
              setParentMessages(loaded)
            })
            .catch(() => {})
        }
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [parentMessages.length, screen, setAllFiles, setParentMessages, setTreeNodes, setWsFiles, wsRefreshKey])
  const drawerOpen = true // right side always open
  const threadEndRef = useRef<HTMLDivElement>(null)
  const childThreadEndRef = useRef<HTMLDivElement>(null)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const childIframeRef = useRef<HTMLIFrameElement>(null)

  // Child-mode split: how much room her worksheet gets versus the chat. Two ways
  // to set it, both writing the same persisted preference: dragging the divider
  // between the panes (fine-grained, and its hit area is deliberately much wider
  // than the hairline it draws), or the widen/restore button in the worksheet
  // toolbar for the coarse "normal ⇄ as big as it goes" jump.
  const [childSideWidthPref, setChildSideWidth] = useState(readChildSideWidth)
  // Track the window width so the stored preference can be re-clamped when the
  // window shrinks. Without this, widening on a large display and then resizing
  // smaller would leave the chat below its minimum (or push it off-screen), since
  // the preference is an absolute pixel value.
  const [windowWidth, setWindowWidth] = useState(() => window.innerWidth)
  useEffect(() => {
    const onResize = () => setWindowWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  // The preference is what the child chose; this is what the layout can honour.
  const childSideWidth = Math.min(Math.max(childSideWidthPref, CHILD_SIDE_MIN), childSideMax(windowWidth))
  // Set-and-save in one place, clamped to what the layout allows, so the drag
  // handle, its arrow keys and the toolbar button can't drift apart.
  const commitChildSideWidth = (px: number) => {
    const next = Math.min(Math.max(px, CHILD_SIDE_MIN), childSideMax(windowWidth))
    setChildSideWidth(next)
    persistChildSideWidth(next)
  }
  const childBodyRef = useRef<HTMLDivElement>(null)
  const [childResizing, setChildResizing] = useState(false)
  // Reading size for the worksheet, cycled by the toolbar's "Aa" button and
  // remembered — a child who needs bigger text needs it every session, not
  // once.
  const [childZoom, setChildZoom] = useState(readChildZoom)
  const cycleChildZoom = () => {
    const i = CHILD_ZOOM_STEPS.indexOf(childZoom)
    const next = CHILD_ZOOM_STEPS[(i + 1) % CHILD_ZOOM_STEPS.length]
    setChildZoom(next)
    persistChildZoom(next)
  }
  const [childChatZoom, setChildChatZoom] = useState(readChildChatZoom)
  const cycleChildChatZoom = () => {
    const i = CHILD_ZOOM_STEPS.indexOf(childChatZoom)
    const next = CHILD_ZOOM_STEPS[(i + 1) % CHILD_ZOOM_STEPS.length]
    setChildChatZoom(next)
    persistChildChatZoom(next)
  }
  // Pointer-events (not mouse) so the same handler covers trackpad, mouse and
  // touch. Width is measured from the body's RIGHT edge rather than by
  // accumulating deltas, so the panel edge tracks the finger exactly and can't
  // drift after a clamp at either end.
  const startChildResize = (e: React.PointerEvent<HTMLDivElement>) => {
    const body = childBodyRef.current
    if (!body) return
    e.preventDefault()
    ;(e.currentTarget as HTMLDivElement).focus({ preventScroll: true })
    const right = body.getBoundingClientRect().right
    const max = childSideMax(windowWidth)
    let last = childSideWidth
    setChildResizing(true)
    const move = (ev: PointerEvent) => {
      last = Math.min(Math.max(right - ev.clientX, CHILD_SIDE_MIN), max)
      setChildSideWidth(last)
    }
    const end = () => {
      setChildResizing(false)
      // Persist once at the end, not on every move — this writes localStorage.
      persistChildSideWidth(last)
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', end)
      window.removeEventListener('pointercancel', end)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', end)
    window.addEventListener('pointercancel', end)
  }
  // Parent Mode's own resizer — same pointer-tracking approach as the
  // child's, clamped to parentSideMax's 50%-of-window ceiling instead.
  const [parentSideWidthPref, setParentSideWidth] = useState(readParentSideWidth)
  const parentSideWidth = Math.min(Math.max(parentSideWidthPref, PARENT_SIDE_MIN), parentSideMax(windowWidth))
  const commitParentSideWidth = (px: number) => {
    const next = Math.min(Math.max(px, PARENT_SIDE_MIN), parentSideMax(windowWidth))
    setParentSideWidth(next)
    persistParentSideWidth(next)
  }
  const parentBodyRef = useRef<HTMLDivElement>(null)
  const [parentResizing, setParentResizing] = useState(false)
  const startParentResize = (e: React.PointerEvent<HTMLDivElement>) => {
    const body = parentBodyRef.current
    if (!body) return
    e.preventDefault()
    ;(e.currentTarget as HTMLDivElement).focus({ preventScroll: true })
    const right = body.getBoundingClientRect().right
    const max = parentSideMax(windowWidth)
    let last = parentSideWidth
    setParentResizing(true)
    const move = (ev: PointerEvent) => {
      last = Math.min(Math.max(right - ev.clientX, PARENT_SIDE_MIN), max)
      setParentSideWidth(last)
    }
    const end = () => {
      setParentResizing(false)
      persistParentSideWidth(last)
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', end)
      window.removeEventListener('pointercancel', end)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', end)
    window.addEventListener('pointercancel', end)
  }
  // "Wide" once past the midpoint between default and max, so the button's icon
  // always shows which way the next tap will move it.
  const childSideWide = childSideWidth > (CHILD_SIDE_DEFAULT + childSideMax(windowWidth)) / 2
  const drawerTab = useWorkspaceStore((s) => s.drawerTab)
  const setDrawerTab = useWorkspaceStore((s) => s.setDrawerTab)
  // The ONE activity the child is currently bound to (/api/child/activity) —
  // the child workspace shows only this, not every activity ever created.
  const childActivity = useChildChatStore((s) => s.childActivity)
  const setChildActivity = useChildChatStore((s) => s.setChildActivity)
  const childViewerPath = useChildChatStore((s) => s.childViewerPath)
  const setChildViewerPath = useChildChatStore((s) => s.setChildViewerPath)
  // Bumped whenever open_file fires, even for the SAME path — Quill re-opens
  // the child's own active/ copy after editing it to add a progress note, and
  // a same-string setChildViewerPath wouldn't otherwise trigger a refetch.
  const [childViewerRefreshKey, setChildViewerRefreshKey] = useState(0)
  // Optional element id the tutor asked us to scroll to inside the opened page
  // (open_file's `focus`). Empty = let the viewer pick the first unanswered question.
  const [childViewerFocus, setChildViewerFocus] = useState('')
  const [startBurst, setStartBurst] = useState(false)
  const [childQuickMenuOpen, setChildQuickMenuOpen] = useState(false)
  // Last known scroll offset per file, reported out of the sandboxed iframe (see
  // withViewerPositionScript). A ref, not state: it updates on every scroll frame
  // and must never trigger a re-render, which would reload the iframe and destroy
  // the very position being tracked.
  const childViewerPathRef = useRef<string | null>(null)
  const childViewerScrollRef = useRef<Record<string, number>>({})
  useEffect(() => {
    const onMsg = (ev: MessageEvent) => {
      const m = ev.data as { __sq?: number; op?: string; y?: number } | null
      if (!m || m.__sq !== 1 || m.op !== 'viewer-scroll' || typeof m.y !== 'number') return
      const path = childViewerPathRef.current
      if (path) childViewerScrollRef.current[path] = m.y
    }
    window.addEventListener('message', onMsg)
    return () => window.removeEventListener('message', onMsg)
  }, [])
  const childViewerContent = useChildChatStore((s) => s.childViewerContent)
  // Reading childViewerScrollRef.current DURING render (instead of only here,
  // memoized) would re-bake the latest scroll position into srcDoc on every
  // unrelated re-render of this component (e.g. each composer keystroke) —
  // React would then see a changed srcDoc string and reload the iframe,
  // replaying the position-restore highlight/scroll for no reason. Memoizing
  // on the intentional inputs only means the live-updating ref is read once
  // per genuine content/focus/path/zoom change, not once per render.
  const childViewerSrcDoc = useMemo(
    () => withViewerPositionScript(
      childViewerContent?.content ?? '',
      childViewerFocus,
      childViewerScrollRef.current[childViewerPath ?? ''] ?? 0,
      childZoom,
    ),

    [childViewerContent, childViewerFocus, childZoom, childViewerPath],
  )
  const setChildViewerContent = useChildChatStore((s) => s.setChildViewerContent)
  const childTreeRefreshKey = useChildChatStore((s) => s.childTreeRefreshKey)
  const setChildTreeRefreshKey = useChildChatStore((s) => s.setChildTreeRefreshKey)
  const filesSubjectFilter = useWorkspaceStore((s) => s.filesSubjectFilter)
  const setFilesSubjectFilter = useWorkspaceStore((s) => s.setFilesSubjectFilter)
  const filesGroupBy = useWorkspaceStore((s) => s.filesGroupBy)
  const setFilesGroupBy = useWorkspaceStore((s) => s.setFilesGroupBy)
  const activities = useWorkspaceStore((s) => s.activities)
  const setActivities = useWorkspaceStore((s) => s.setActivities)
  const viewerPath = useWorkspaceStore((s) => s.viewerPath)
  const setViewerPath = useWorkspaceStore((s) => s.setViewerPath)
  const viewerRefreshKey = useWorkspaceStore((s) => s.viewerRefreshKey)
  const setViewerRefreshKey = useWorkspaceStore((s) => s.setViewerRefreshKey)
  const viewerImageList = useWorkspaceStore((s) => s.viewerImageList)
  const setViewerImageList = useWorkspaceStore((s) => s.setViewerImageList)
  // The dir of an activity opened via open_activity (the whole activity
  // overview). Can be set ALONGSIDE viewerPath (not just instead of it):
  // clicking an item inside the activity view sets viewerPath without
  // clearing this, so viewerPath's own "back" button falls through to the
  // activity view again instead of the raw file list — viewerPath simply
  // takes render priority over viewerActivityDir whenever both are set.
  const [viewerActivityDir, setViewerActivityDir] = useState<string | null>(null)
  const viewerContent = useWorkspaceStore((s) => s.viewerContent)
  const setViewerContent = useWorkspaceStore((s) => s.setViewerContent)
  const [viewerMeta, setViewerMeta] = useState<Record<string, unknown> | null>(null)
  const [metaOpen, setMetaOpen] = useState(false)
  // Which activity's guide_note (the parent's own pacing/instructions for
  // that activity) is currently revealed via its (i) button — collapsed by default.
  const [expandedActivity, setExpandedActivity] = useState<string | null>(null)
  // Which folders in the "all files" tree the user has explicitly opened or
  // closed, keyed by path — survives the FileTree unmounting when a file is
  // opened for viewing (drawerTab's ternary swaps to the viewer branch, then
  // back), so opening a file and returning no longer collapses everything
  // back to the default top-level-only view. Absent from this map = use the
  // component's own default (top level open, everything nested closed).
  const [treeExpanded, setTreeExpanded] = useState<Record<string, boolean>>({})
  const mapHtml = useWorkspaceStore((s) => s.mapHtml)
  const setMapHtml = useWorkspaceStore((s) => s.setMapHtml)
  const mapRefreshKey = useWorkspaceStore((s) => s.mapRefreshKey)
  const setMapRefreshKey = useWorkspaceStore((s) => s.setMapRefreshKey)
  const progressHtml = useWorkspaceStore((s) => s.progressHtml)
  const setProgressHtml = useWorkspaceStore((s) => s.setProgressHtml)
  const booting = useSetupStore((s) => s.booting)
  const setBooting = useSetupStore((s) => s.setBooting)
  const bootError = useSetupStore((s) => s.bootError)
  const setBootError = useSetupStore((s) => s.setBootError)
  const pin = useSetupStore((s) => s.pin)
  const setPin = useSetupStore((s) => s.setPin)
  const pinConfirm = useSetupStore((s) => s.pinConfirm)
  const setPinConfirm = useSetupStore((s) => s.setPinConfirm)
  const pinError = useSetupStore((s) => s.pinError)
  const setPinError = useSetupStore((s) => s.setPinError)
  const saving = useSetupStore((s) => s.saving)
  const setSaving = useSetupStore((s) => s.setSaving)
  // Child→Parent is gated by the parent PIN so a child can't reach answer keys.
  const pinGate = usePinGateStore((s) => s.pinGate)
  const setPinGate = usePinGateStore((s) => s.setPinGate)
  const gateValue = usePinGateStore((s) => s.gateValue)
  const setGateValue = usePinGateStore((s) => s.setGateValue)
  const gateError = usePinGateStore((s) => s.gateError)
  const setGateError = usePinGateStore((s) => s.setGateError)

  // Load the real, agent-generated reports/academic-map.html for the Subjects
  // tab — refetches whenever the tab is opened or a turn just completed (the
  // agent may have rebuilt the map during that turn).
  useEffect(() => {
    if (drawerTab !== 'map') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent('reports/academic-map.html')}`)
      .then((r) => r.json())
      .then((d) => { if (!cancelled) setMapHtml(d.content ?? '') })
      .catch(() => { if (!cancelled) setMapHtml('') })
    return () => { cancelled = true }
  }, [drawerTab, mapRefreshKey, setMapHtml])

  // Load the real, agent-generated reports/progress.html for the Progress tab
  // — a single living document, rendered directly (not a link the parent has
  // to click through to).
  useEffect(() => {
    if (drawerTab !== 'progress') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent('reports/progress.html')}`)
      .then((r) => r.json())
      .then((d) => { if (!cancelled) setProgressHtml(d.content ?? '') })
      .catch(() => { if (!cancelled) setProgressHtml('') })
    return () => { cancelled = true }
  }, [drawerTab, mapRefreshKey, setProgressHtml])

  // "This Week" tab — combined schedule/activity-log/deadlines view (see
  // week.go). Re-fetches whenever the tab is open, the week being viewed
  // changes, or a turn just completed (mapRefreshKey — same signal Progress
  // uses, since a turn can add an activity-log entry or update the schedule).
  useEffect(() => {
    if (drawerTab !== 'week') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/week?offset=${weekOffset}`)
      .then((r) => r.json())
      .then((d: WeekResponse) => { if (!cancelled) setWeekData(d) })
      .catch(() => { if (!cancelled) setWeekData(null) })
    return () => { cancelled = true }
  }, [drawerTab, weekOffset, mapRefreshKey])

  // Every activity, structured — refetched whenever the Files/Uploaded tab is
  // open or a turn just completed (Quill may have created or added to one).
  // Gated on the drawer tab as a whole, deliberately loosely: open_activity
  // can jump straight to an activity's detail view from anywhere, and with a
  // narrower gate an activity Quill had just created and opened could show
  // "no longer available" simply because this fetch never ran.
  useEffect(() => {
    if (drawerTab !== 'files' && drawerTab !== 'uploaded') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/activities`)
      .then((r) => r.json())
      .then((d: Activity[]) => { if (!cancelled) setActivities(d ?? []) })
      .catch(() => { if (!cancelled) setActivities([]) })
    return () => { cancelled = true }
  }, [drawerTab, mapRefreshKey, setActivities])

  // Poll real WhatsApp pairing status while the connector modal's WhatsApp
  // section is open — refreshes the QR (it's short-lived) until paired.
  useEffect(() => {
    if (!waOpen || connectorSection !== 'whatsapp') return
    let cancelled = false
    const poll = () => {
      fetch(`${FAMILY_API}/api/whatsapp/status`)
        .then((r) => r.json())
        .then((d: { accounts: { jid: string; connected: boolean }[]; pairing: { qr_available: boolean; qr_expires_at?: string }; voice_transcription?: { enabled: boolean; installed: boolean; installing: boolean; model_size_mb: number; available: boolean; error?: string } }) => {
          if (cancelled) return
          setWaStatus(d)
          setWaQrNonce((n) => n + 1) // there's always a pairing slot open for one more phone
        })
        .catch(() => {})
    }
    poll()
    const id = window.setInterval(poll, 3000)
    return () => { cancelled = true; window.clearInterval(id) }
  }, [waOpen, connectorSection])

  // Browser connector — just a one-time CLI-install check; whether a CDP
  // Chrome is actually reachable is decided by agent-browser itself per call.
  useEffect(() => {
    if (!waOpen || connectorSection !== 'browser') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/browser/status`)
      .then((r) => r.json())
      .then((d: { cli_installed: boolean }) => { if (!cancelled) setBrowserStatus(d) })
      .catch(() => { if (!cancelled) setBrowserStatus({ cli_installed: false }) })
    return () => { cancelled = true }
  }, [waOpen, connectorSection])

  // Pulse config — loaded on entering the parent screen (so the header pill
  // reflects real status right away) and refreshed whenever Settings or the
  // pill's own popover opens.
  useEffect(() => {
    if (screen !== 'parent' && !settingsOpen && !pulsePopoverOpen) return
    let cancelled = false
    fetch(`${FAMILY_API}/api/pulse/config`)
      .then((r) => r.json())
      .then((d: { enabled: boolean; cadence_hours: number; last_run_at?: string; watch_sites?: string[]; preferred_hour: number; preferred_hour_set: boolean }) => {
        if (cancelled) return
        setPulseConfig(d)
        setWatchSitesDraft((d.watch_sites || []).join('\n'))
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [screen, settingsOpen, pulsePopoverOpen])

  // Fast Mode — whether every turn (parent, child, WhatsApp, Pulse) uses the
  // provider's cheap/fast low tier instead of the normal tuned model (see
  // model_tier.go's selectedModelID). Same load-on-Settings-open pattern as
  // Pulse config above.
  const [fastMode, setFastMode] = useState(false)
  const [savingFastMode, setSavingFastMode] = useState(false)

  // Child Mode reminder sound — off by default, opt-in from Settings. Quill
  // can take anywhere from a few seconds to several minutes to reply (real
  // turns logged tonight ran 30s-5min); a child who's wandered off while
  // waiting has no other signal that a reply actually arrived. Persisted to
  // localStorage (not familyState) since it's a per-device UI preference, not
  // a family policy — same reasoning the old read-aloud toggle used.
  const [childReminderSound, setChildReminderSound] = useState(() => readReminderSoundPref())
  const toggleChildReminderSound = (on: boolean) => {
    setChildReminderSound(on)
    persistReminderSoundPref(on)
  }
  useEffect(() => {
    if (screen !== 'parent' && screen !== 'tutor' && !settingsOpen) return
    let cancelled = false
    fetch(`${FAMILY_API}/api/fast-mode`)
      .then((r) => r.json())
      .then((d: { enabled: boolean }) => { if (!cancelled) setFastMode(!!d.enabled) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [screen, settingsOpen])

  const toggleFastMode = (enabled: boolean) => {
    setFastMode(enabled)
    setSavingFastMode(true)
    fetch(`${FAMILY_API}/api/fast-mode`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    })
      .catch(() => {})
      .finally(() => setSavingFastMode(false))
  }

  // Voice tier catalog — loaded whenever Settings opens. Cheap (a sysctl read
  // plus two LookPath calls), so it's refetched each time rather than cached:
  // installing a model elsewhere should be reflected on the next open.
  const refreshVoiceStatus = useCallback(() => {
    fetch(`${FAMILY_API}/api/voice/status`)
      .then((r) => r.json())
      .then((d: VoiceStatus) => setVoiceStatus(d))
      .catch(() => {})
  }, [])
  useEffect(() => {
    if (!settingsOpen) return
    refreshVoiceStatus()
    // Poll while a model is actually downloading (live progress) OR still
    // warming up in the background (see voice_worker.go's proactive
    // startup warm-up) — otherwise "Warming up…" would sit stale until the
    // parent happened to close and reopen Settings. Idle Settings with
    // nothing in flight shouldn't hit the server every couple seconds though.
    const allTiers = voiceStatus?.stt_tiers ?? []
    const anyInstalling = allTiers.some((t) => t.installing)
    const anyWarming = allTiers.some((t) => t.installed && t.warm === false)
    if (!anyInstalling && !anyWarming) return
    const id = window.setInterval(refreshVoiceStatus, 1500)
    return () => window.clearInterval(id)
  }, [settingsOpen, refreshVoiceStatus, voiceStatus])

  // Secret names (never values) — loaded whenever Settings is opened.
  useEffect(() => {
    if (!settingsOpen) return
    let cancelled = false
    fetch(`${FAMILY_API}/api/secrets`)
      .then((r) => r.json())
      .then((d: { names?: string[] }) => { if (!cancelled) setSecretNames(d.names ?? []) })
      .catch(() => { if (!cancelled) setSecretNames([]) })
    return () => { cancelled = true }
  }, [settingsOpen])

  const saveSecret = () => {
    const name = secretNameDraft.trim()
    const value = secretValueDraft.trim()
    if (!name || !value) return
    setSavingSecret(true)
    fetch(`${FAMILY_API}/api/secrets`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, value }),
    })
      .then((r) => r.json())
      .then((d: { names?: string[] }) => {
        setSecretNames(d.names ?? [])
        setSecretNameDraft('')
        setSecretValueDraft('')
      })
      .finally(() => setSavingSecret(false))
  }

  const deleteSecret = (name: string) => {
    setDeletingSecret(name)
    fetch(`${FAMILY_API}/api/secrets`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    })
      .then((r) => r.json())
      .then((d: { names?: string[] }) => setSecretNames(d.names ?? []))
      .finally(() => setDeletingSecret(null))
  }

  // Pick up async updates to the open conversation — Pulse (or a WhatsApp
  // reply, if this same conversation is the real WhatsApp thread) can append
  // a new message to this exact conversation file from the background, with
  // nothing else telling this open tab to know. Poll lightly and, if the
  // file has grown since we last rendered it, surface a small "new update"
  // banner rather than silently rewriting the screen under the parent —
  // they choose when to pull it in. Skip while a send is in flight.
  useEffect(() => {
    if (screen !== 'parent' || !conversationId) return
    const id = window.setInterval(() => {
      if (sending) return
      fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent('conversations/parent.json')}`)
        .then((r) => r.json())
        .then((d) => {
          if (!d?.content) return
          const c = JSON.parse(d.content) as { messages?: StoredMsg[] }
          const fresh = c.messages || []
          if (fresh.length > parentMessages.length) {
            setPendingConvUpdate(fresh)
          }
        })
        .catch(() => {})
    }, 20000)
    return () => window.clearInterval(id)
  }, [screen, conversationId, sending, parentMessages.length])

  // Clear any pending "new update" banner whenever the parent switches
  // conversations or sends their own message — it only ever refers to the
  // specific conversation/point in time it was detected for.
  useEffect(() => { setPendingConvUpdate(null) }, [conversationId])

  // sendingRef mirrors `sending` for the ambient effect below without being
  // in its dependency array — see childSendingRef's own comment for why.
  const sendingRef = useRef(sending)
  useEffect(() => { sendingRef.current = sending }, [sending])

  // Ambient live-status subscription for the parent conversation — mirrors
  // Child Mode's own ambient effect (see its comment for the full
  // rationale): sendParentText already opens its OWN /api/parent/status
  // stream around this tab's own fetch, but a turn started elsewhere (a real
  // WhatsApp message running w.runTurn, or Pulse) publishes to the exact
  // same per-conversation-id topic with nobody here subscribed to catch it.
  // This stays open for as long as the parent screen is showing, not just
  // around this tab's own send.
  useEffect(() => {
    if (screen !== 'parent' || !conversationId) return
    const source = new EventSource(`${FAMILY_API}/api/parent/status?conversation_id=${encodeURIComponent(conversationId)}`)
    let idleTimer: number | undefined
    const armIdleReset = () => {
      if (idleTimer) window.clearTimeout(idleTimer)
      idleTimer = window.setTimeout(() => {
        setRemoteStatus('')
        setRemoteToolCalls([])
      }, 45000)
    }
    source.onmessage = (ev) => {
      if (sendingRef.current) return
      try {
        const parsed = JSON.parse(ev.data) as { type?: string; text?: string; tool?: string; args?: string }
        if (parsed.type === 'status') {
          setRemoteStatus(parsed.text ?? '')
          armIdleReset()
        } else if (parsed.type === 'tool_call' && parsed.tool) {
          setRemoteToolCalls((cur) => [...cur, { tool: parsed.tool as string, args: parsed.args }])
          armIdleReset()
        }
      } catch { /* ignore malformed event */ }
    }
    return () => {
      source.close()
      if (idleTimer) window.clearTimeout(idleTimer)
    }
  }, [screen, conversationId])

  // The browser's own send supersedes any stale remote indicator.
  useEffect(() => {
    if (sending) {
      setRemoteStatus('')
      setRemoteToolCalls([])
    }
  }, [sending])

  // Child Mode's own version of the polling above — watches the CURRENT
  // activity's own conversation.json instead of conversations/parent.json,
  // gated on being in the tutor screen with a bound activity rather than
  // 'parent'. Unlike the parent side, this applies new messages directly
  // rather than surfacing a "tap to refresh" banner first — a child waiting
  // on a WhatsApp-routed reply shouldn't have to notice and tap a banner to
  // see it; the auto-scroll effect above already brings it into view.
  useEffect(() => {
    const dir = childActivity?.dir
    if (screen !== 'tutor' || !dir) return
    const id = window.setInterval(() => {
      if (childSending) return
      fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent(`${dir}/conversation.json`)}`)
        .then((r) => r.json())
        .then((d) => {
          if (!d?.content) return
          const c = JSON.parse(d.content) as { messages?: StoredMsg[] }
          const fresh = c.messages || []
          if (fresh.length > childMessages.length) {
            setChildMessages(fresh.map(toParentMsg))
            // Whatever remote turn was running (see the ambient live-status
            // effect below) has now clearly finished and persisted — drop its
            // indicator rather than let it linger until the idle timeout.
            setChildRemoteStatus('')
            setChildRemoteToolCalls([])
            if (childReminderSound && fresh[fresh.length - 1]?.role === 'assistant') playReminderChime()
          }
        })
        .catch(() => {})
    }, 20000)
    return () => window.clearInterval(id)
  }, [screen, childActivity?.dir, childSending, childMessages.length])

  // childSendingRef mirrors childSending for the ambient effect below without
  // being in its dependency array — re-subscribing the SSE connection every
  // time a send starts/stops would risk missing an event in the reconnect gap.
  const childSendingRef = useRef(childSending)
  useEffect(() => { childSendingRef.current = childSending }, [childSending])

  // Ambient live-status subscription for Child Mode: sendChildText/
  // sendChildKickoff already open their OWN /api/child/status stream around
  // this tab's own fetch, so live tool calls show up when SHE sends a
  // message. But the exact same SSE topic (see status_stream.go's
  // withLiveStatus/statusHub — a plain per-conversation-id pub/sub with no
  // concept of "who's listening") is published to by ANY turn on this
  // activity, including one WhatsApp's @child routing runs via runChildTurn
  // while nobody here is looking. Without a subscription open at THAT
  // moment, those events are silently dropped (no listener), which is
  // exactly why tool calls never showed up for a WhatsApp-triggered turn
  // before this effect existed. This one stays open for as long as the tutor
  // screen is showing this activity, not just around this tab's own send.
  useEffect(() => {
    const dir = childActivity?.dir
    if (screen !== 'tutor' || !dir) return
    const source = new EventSource(`${FAMILY_API}/api/child/status?conversation_id=${encodeURIComponent(dir)}`)
    let idleTimer: number | undefined
    const armIdleReset = () => {
      if (idleTimer) window.clearTimeout(idleTimer)
      // No event on the wire for a while — assume the remote turn finished
      // and we simply missed (or there wasn't) a clean final signal, so this
      // indicator never gets stuck showing forever.
      idleTimer = window.setTimeout(() => {
        setChildRemoteStatus('')
        setChildRemoteToolCalls([])
      }, 45000)
    }
    source.onmessage = (ev) => {
      // Events from THIS tab's own send are already shown via
      // childLiveStatus/childLiveToolCalls (see sendChildText) — skip here so
      // the same tool call never renders twice.
      if (childSendingRef.current) return
      try {
        const parsed = JSON.parse(ev.data) as { type?: string; text?: string; tool?: string; args?: string }
        if (parsed.type === 'status') {
          setChildRemoteStatus(parsed.text ?? '')
          armIdleReset()
        } else if (parsed.type === 'tool_call' && parsed.tool) {
          setChildRemoteToolCalls((cur) => [...cur, { tool: parsed.tool as string, args: parsed.args }])
          armIdleReset()
        }
      } catch { /* ignore malformed event */ }
    }
    // Deliberately no onerror handler that closes the connection — the
    // browser's own EventSource auto-reconnects on a transient drop, and this
    // subscription is meant to stay up for as long as the screen does.
    return () => {
      source.close()
      if (idleTimer) window.clearTimeout(idleTimer)
    }
  }, [screen, childActivity?.dir])

  // The browser's own send supersedes any stale remote indicator — clear it
  // the moment a real send starts here, same reasoning as clearing it once
  // fresh messages are polled in above.
  useEffect(() => {
    if (childSending) {
      setChildRemoteStatus('')
      setChildRemoteToolCalls([])
    }
  }, [childSending])

  // Drain the send queue: once the current turn finishes, send the next queued
  // message. One at a time, in order — so the transcript stays well-formed and
  // each reply builds on the previous. sendParentText itself flips `sending`
  // back on, which re-guards this until that turn also completes.
  useEffect(() => {
    if (sending || queue.length === 0) return
    const [next, ...rest] = queue
    setQueue(rest)
    sendParentText(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sending, queue])

  // Same drain, for the child's own queue.
  useEffect(() => {
    if (childSending || childQueue.length === 0) return
    const [next, ...rest] = childQueue
    setChildQueue(rest)
    sendChildText(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [childSending, childQueue])

  const savePulseConfig = (patch: { enabled?: boolean; cadence_hours?: number; watch_sites?: string[]; preferred_hour?: number; preferred_hour_set?: boolean }) => {
    setSavingPulse(true)
    fetch(`${FAMILY_API}/api/pulse/config`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    })
      .then((r) => r.json())
      .then((d) => setPulseConfig(d))
      .catch(() => {})
      .finally(() => setSavingPulse(false))
  }

  // Opens the schedule mini-editor, seeding the draft from whatever the week
  // view currently knows the schedule to be — de-duplicated across days since
  // weekData.days each carry their own matching entries (one recurring entry
  // appears on every matching weekday in the response).
  const openScheduleEditor = () => {
    const seen = new Set<string>()
    const entries: ScheduleEntry[] = []
    for (const day of weekData?.days ?? []) {
      for (const e of day.schedule ?? []) {
        const key = `${e.day}|${e.start}|${e.end}|${e.label}`
        if (seen.has(key)) continue
        seen.add(key)
        entries.push(e)
      }
    }
    setScheduleDraft(entries)
    setScheduleEditorOpen(true)
  }

  // Wholesale replace — the mini-editor always sends its whole edited list
  // (conversational capture goes through set_child_schedule instead, which
  // ADDS rather than replaces — see parent_tools.go).
  const saveSchedule = () => {
    setSavingSchedule(true)
    fetch(`${FAMILY_API}/api/child-schedule`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ entries: scheduleDraft }),
    })
      .then(() => fetch(`${FAMILY_API}/api/week?offset=${weekOffset}`))
      .then((r) => r.json())
      .then((d: WeekResponse) => { setWeekData(d); setScheduleEditorOpen(false) })
      .catch(() => {})
      .finally(() => setSavingSchedule(false))
  }

  // Runs Pulse right now (regardless of the recurring toggle) — used to test
  // it without waiting for the ticker. Fires the request, then polls config
  // and watches last_run_at change to know when the real turn (which can
  // take a few minutes) has finished.
  const runPulseNow = () => {
    const before = pulseConfig?.last_run_at
    setPulseRunError(null)
    setPulseRunning(true)
    fetch(`${FAMILY_API}/api/pulse/run`, { method: 'POST' })
      .then((r) => r.json().then((d) => ({ ok: r.ok, d })))
      .then(({ ok, d }) => {
        if (!ok) { setPulseRunError(d.error || 'Could not start.'); setPulseRunning(false); return }
        const poll = (attempt: number) => {
          if (attempt > 300) { setPulseRunning(false); setPulseRunError('Taking longer than expected — check back shortly.'); return }
          fetch(`${FAMILY_API}/api/pulse/config`)
            .then((r) => r.json())
            .then((cfg) => {
              setPulseConfig(cfg)
              if (cfg.last_run_at && cfg.last_run_at !== before) {
                setPulseRunning(false)
              } else {
                window.setTimeout(() => poll(attempt + 1), 4000)
              }
            })
            .catch(() => window.setTimeout(() => poll(attempt + 1), 4000))
        }
        window.setTimeout(() => poll(0), 4000)
      })
      .catch(() => { setPulseRunError('Could not reach SparkQuill.'); setPulseRunning(false) })
  }

  // The ONE activity the child is currently bound to — replaces the old
  // scoped-tree scan + package-manifest derivation entirely. Also resumes the
  // child's own conversation (now the activity's own conversation.json)
  // exactly once, the same "don't silently cold-start on refresh" fix the
  // parent thread has above.
  useEffect(() => {
    if (screen !== 'parent' && screen !== 'tutor') return
    let cancelled = false
    fetch(`${FAMILY_API}/api/child/activity`)
      .then((r) => r.json())
      .then((act: Activity | null) => {
        if (cancelled) return
        setChildActivity(act)
        if (!act || childResumedRef.current || useChildChatStore.getState().childMessages.length > 0) return
        childResumedRef.current = true
        fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent(`${act.dir}/conversation.json`)}`)
          .then((r2) => r2.json())
          .then((dd) => {
            if (!dd?.content) return
            const c = JSON.parse(dd.content) as { messages?: StoredMsg[] }
            const loaded = (c.messages || []).map(toParentMsg)
            setChildMessages(loaded)
          })
          .catch(() => {})
      })
      .catch(() => { if (!cancelled) setChildActivity(null) })
    return () => { cancelled = true }
  }, [childTreeRefreshKey, screen, setChildActivity, setChildMessages])

  // The moment a distinct activity is bound (a fresh handoff, or resuming on
  // reload), show its first item — the same "don't wait on the model to
  // remember to call open_file" guarantee the old handoff-response filePath
  // gave, without threading a file path through the handoff call itself.
  // Always force it (not just when nothing is open yet): childViewerPath can
  // still hold a PREVIOUS activity's last-viewed file at this point, and
  // without overriding it the child would keep seeing that old file's
  // content instead of the newly handed-off activity's own.
  const autoOpenedActivityRef = useRef<string | null>(null)
  useEffect(() => {
    if (screen !== 'tutor' || !childActivity) return
    if (autoOpenedActivityRef.current === childActivity.dir) return
    autoOpenedActivityRef.current = childActivity.dir
    const first = childActivity.items[0]
    if (first) { setChildViewerPath(first.path); setChildViewerRefreshKey((k) => k + 1) }
  }, [childActivity, screen, setChildViewerPath])

  // Load the selected file for the child's own inline viewer.
  //
  // Re-opening the SAME file (Quill re-calls open_file after recording an
  // answer) reloads the iframe's document and resets its scroll to the top.
  // That used to be handled by capturing contentWindow.scrollY before the
  // refresh and restoring it after — which never worked: the iframe is
  // sandboxed WITHOUT allow-same-origin, so the read throws, gets caught, and
  // returns 0. The page reliably jumped to the top.
  // withViewerPositionScript now handles this from INSIDE the frame, restoring
  // the offset she was actually at (reported out of the frame as she scrolls).
  useEffect(() => { childViewerPathRef.current = childViewerPath }, [childViewerPath])
  useEffect(() => {
    if (!childViewerPath) { setChildViewerContent(null); return }
    let cancelled = false
    setChildViewerContent(null)
    fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent(childViewerPath)}`)
      .then((r) => r.json())
      .then((d) => { if (!cancelled) setChildViewerContent({ isText: !!d.is_text, content: d.is_text ? rewriteRelativeAssetURLs(d.content ?? '', childViewerPath) : (d.content ?? '') }) })
      .catch(() => { if (!cancelled) setChildViewerContent({ isText: false, content: '' }) })
    return () => { cancelled = true }
  }, [childViewerPath, childViewerRefreshKey, setChildViewerContent])

  // Load the selected file for the drawer's Files viewer.
  useEffect(() => {
    if (!viewerPath) { setViewerContent(null); return }
    let cancelled = false
    setViewerContent(null)
    fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent(viewerPath)}`)
      .then((r) => r.json())
      .then((d) => { if (!cancelled) setViewerContent({ isText: !!d.is_text, content: d.is_text ? rewriteRelativeAssetURLs(d.content ?? '', viewerPath) : (d.content ?? '') }) })
      .catch(() => { if (!cancelled) setViewerContent({ isText: false, content: '' }) })
    return () => { cancelled = true }
  }, [setViewerContent, viewerPath, viewerRefreshKey])

  // Probe for the file's metadata sidecar (<path>.meta.json — written by the
  // process-file skill when Quill files an upload: subject, topic, type, a short
  // summary, key concepts). When present, the viewer shows an info button that
  // reveals it, so the parent can see what Quill understood a document to be.
  useEffect(() => {
    setViewerMeta(null)
    setMetaOpen(false)
    if (!viewerPath || viewerPath.endsWith('.meta.json')) return
    let cancelled = false
    fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent(viewerPath + '.meta.json')}`)
      .then((r) => r.json())
      .then((d) => {
        if (cancelled || !d || !d.is_text || !d.content) return
        try { setViewerMeta(JSON.parse(d.content) as Record<string, unknown>) } catch { /* not valid meta; ignore */ }
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [viewerPath, viewerRefreshKey])

  // Left/Right arrow keys step through the image list the viewer was opened
  // from (e.g. the Uploaded Material thumbnail grid for the current subject).
  useEffect(() => {
    if (!viewerPath || !IMAGE_PATH_RE.test(viewerPath) || viewerImageList.length < 2) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
      const idx = viewerImageList.indexOf(viewerPath)
      if (idx === -1) return
      const dir = e.key === 'ArrowLeft' ? -1 : 1
      const next = (idx + dir + viewerImageList.length) % viewerImageList.length
      setViewerPath(viewerImageList[next])
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [setViewerPath, viewerImageList, viewerPath])

  // Keep the conversation scrolled to the latest message / thinking indicator.
  // Also re-scroll on `screen` itself: switching from child mode back to
  // parent mode remounts this thread at its default (top) scroll position —
  // without screen in the deps, that remount doesn't trigger a re-scroll
  // since parentMessages/sending haven't changed, leaving the parent stuck
  // scrolled to the top until they scroll down manually. streamingReply is
  // ALSO a dep: the live-streamed reply bubble grows character-by-character
  // while parentMessages/sending stay unchanged for the whole turn, so
  // without it the view never follows the growing text — it only ever
  // jumped at the start and end of a turn, not while streaming.
  useEffect(() => {
    if (screen !== 'parent') return
    // While actively streaming, a fresh 'smooth' animation starts on every
    // delta chunk (many times a second) and each one cuts off the previous
    // one before it finishes — that competing-animation restart is what
    // makes the scroll stutter/lag behind instead of following the text.
    // 'auto' (instant) during streaming avoids that; 'smooth' is still nicer
    // for the normal, much-less-frequent case (a whole new message, a screen
    // switch). A turn that made MANY tool calls (each its own live debug
    // bubble, see the SSE 'tool_call' handling below) can append a large
    // batch of new DOM content in one go right as the turn finishes —
    // scrolling in the SAME frame can measure
    // a scrollHeight from before the browser has laid all of it out, landing
    // short of the real bottom. rAF-deferring one frame lets layout settle
    // first so the target position is measured against the final height.
    const id = requestAnimationFrame(() => {
      threadEndRef.current?.scrollIntoView({ behavior: streamingReply ? 'auto' : 'smooth', block: 'end' })
    })
    return () => cancelAnimationFrame(id)
  }, [parentMessages, sending, screen, streamingReply, queue, remoteStatus, remoteToolCalls])

  // Same, for the child's own thread — this had no auto-scroll at all before,
  // so new replies (and the "thinking" indicator) could land below the fold
  // with no automatic scroll to reveal them.
  useEffect(() => {
    if (screen !== 'tutor') return
    // See the parent effect above for why this is rAF-deferred — a turn with
    // many tool calls can append a large batch of content in one go.
    const id = requestAnimationFrame(() => {
      childThreadEndRef.current?.scrollIntoView({ behavior: childStreamingReply ? 'auto' : 'smooth', block: 'end' })
    })
    return () => cancelAnimationFrame(id)
  }, [childMessages, childSending, screen, childStreamingReply, childQueue, childRemoteStatus, childRemoteToolCalls])

  // Cycle a usable "how to use the chat" tip in the thinking indicator instead
  // of a bare "thinking…" — resets and restarts each time a new turn begins,
  // and only matters while there's no real live tool status to show instead.
  const [parentHintIndex, setParentHintIndex] = useState(0)
  useEffect(() => {
    if (!sending) { setParentHintIndex(0); return }
    const id = window.setInterval(() => setParentHintIndex((i) => (i + 1) % PARENT_WAIT_HINTS.length), 7000)
    return () => window.clearInterval(id)
  }, [sending])
  const [childHintIndex, setChildHintIndex] = useState(0)
  useEffect(() => {
    if (!childSending) { setChildHintIndex(0); return }
    const id = window.setInterval(() => setChildHintIndex((i) => (i + 1) % CHILD_WAIT_HINTS.length), 7000)
    return () => window.clearInterval(id)
  }, [childSending])

  // Bridge for interactive HTML: the sandboxed viewer iframe posts SQ.save/load
  // messages; the app persists them to a workspace file (child/attempts) so the
  // child's answers survive reloads and Quill can read them later. 'choose' is
  // the newer op a show_scene snippet's button posts to offer a real choice —
  // treated exactly like the child typing/tapping that text, so Quill actually
  // sees and responds to whichever one she picks (see scene_tool.go).
  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      const m = e.data
      if (!m || typeof m !== 'object' || (m as { __sq?: unknown }).__sq !== 1) return
      const msg = m as { op?: string; key?: string; id?: string; data?: unknown; text?: string }
      if (msg.op === 'save' && typeof msg.key === 'string') {
        fetch(`${FAMILY_API}/api/workspace/state`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ key: msg.key, data: msg.data }),
        }).catch(() => {})
      } else if (msg.op === 'load' && typeof msg.key === 'string') {
        fetch(`${FAMILY_API}/api/workspace/state?key=${encodeURIComponent(msg.key)}`)
          .then((r) => r.json())
          .then((d) => iframeRef.current?.contentWindow?.postMessage({ __sq: 1, op: 'loaded', id: msg.id, data: d?.data ?? null }, '*'))
          .catch(() => iframeRef.current?.contentWindow?.postMessage({ __sq: 1, op: 'loaded', id: msg.id, data: null }, '*'))
      } else if (msg.op === 'choose' && typeof msg.text === 'string') {
        sendChildTextRef.current(msg.text)
      }
    }
    window.addEventListener('message', onMsg)
    return () => window.removeEventListener('message', onMsg)
  }, [])

  // On launch, ask family-server where onboarding stands. If setup is complete
  // we land straight in the chat; otherwise resume at the right step.
  useEffect(() => {
    let cancelled = false
    const load = (attempt: number) => {
      fetch(`${FAMILY_API}/api/setup`)
        .then((res) => { if (!res.ok) throw new Error(String(res.status)); return res.json() })
        .then((data: { next_step?: string; engine?: string; child?: { name?: string; grade?: string; board?: string } | null; parent_label?: string }) => {
          if (cancelled) return
          if (data.engine) setEngine(data.engine)
          if (data.child) {
            if (data.child.name) setChildName(data.child.name)
            if (data.child.grade) setGrade(data.child.grade)
            if (data.child.board) setBoard(data.child.board)
          }
          if (data.parent_label) setParentLabel(data.parent_label)
          const step = data.next_step
          if (step === 'done') setScreen(readHandoffSide() === 'tutor' ? 'tutor' : 'parent')
          else if (step === 'pin') setScreen('pin')
          else if (step === 'child') setScreen('child')
          else setScreen('engine')
          setBooting(false)
        })
        .catch(() => {
          if (cancelled) return
          // Never fall back to onboarding on a transient failure — retry, then
          // show an explicit error so completed setup is not lost visually.
          if (attempt < 4) window.setTimeout(() => load(attempt + 1), 500)
          else { setBootError(true); setBooting(false) }
        })
    }
    load(0)
    return () => { cancelled = true }
  }, [setBoard, setBootError, setBooting, setChildName, setEngine, setGrade, setParentLabel, setScreen])

  const selectedEngine = engines.find((item) => item.id === engine)
  const initial = childName.trim().slice(0, 1).toUpperCase() || 'M'

  const runTest = () => {
    if (!selectedEngine) return
    setTestState('testing')
    setTestMessage('')
    fetch(`${FAMILY_API}/api/engines/validate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider: selectedEngine.id, model_id: '' }),
    })
      .then((res) => res.json())
      .then((data: { valid: boolean; message?: string }) => {
        setTestState(data.valid ? 'valid' : 'invalid')
        setTestMessage(data.message ?? (data.valid ? 'Connection works.' : 'Test failed.'))
      })
      .catch(() => {
        setTestState('invalid')
        setTestMessage('Could not run the test.')
      })
  }

  const move = (next: Screen) => {
    setScreen(next)
  }

  // Verify the parent PIN before returning to Parent Mode from the child screen.
  const submitPinGate = () => {
    setGateError('')
    fetch(`${FAMILY_API}/api/parent/pin/verify`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pin: gateValue }),
    })
      .then((res) => res.json())
      .then((data: { ok?: boolean }) => {
        if (data.ok) { setPinGate(false); setGateValue(''); persistHandoffSide('parent'); move('parent') }
        else setGateError('That PIN isn’t right.')
      })
      .catch(() => setGateError('Could not check the PIN.'))
  }

  const persistEngineAndContinue = () => {
    if (!selectedEngine) return
    setSaving(true)
    fetch(`${FAMILY_API}/api/engine/selection`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ engine: selectedEngine.id }),
    }).finally(() => { setSaving(false); move('child') })
  }

  const createChildAndContinue = () => {
    if (!childName.trim()) return
    setSaving(true)
    fetch(`${FAMILY_API}/api/child`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: childName, grade, board }),
    }).finally(() => { setSaving(false); move('pin') })
  }

  const savePinAndContinue = () => {
    setPinError('')
    if (!/^\d{4,8}$/.test(pin)) { setPinError('Use 4–8 digits.'); return }
    if (pin !== pinConfirm) { setPinError('The two PINs don’t match.'); return }
    setSaving(true)
    fetch(`${FAMILY_API}/api/parent/pin`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pin }),
    })
      .then((res) => res.json())
      .then((data: { error?: string }) => { if (data.error) { setPinError(data.error); return } persistHandoffSide('parent'); move('parent') })
      .catch(() => setPinError('Could not save the PIN.'))
      .finally(() => setSaving(false))
  }

  const sendParentText = (raw: string) => {
    const text = raw.trim()
    if (!text) return
    // A turn is already running — STEER is the primary path: try to inject
    // it into the live turn right now (see steer.go). If that lands, show it
    // as a normal message immediately (it's genuinely part of the
    // conversation now, not a separate future turn) — only fall back to the
    // "queued" bubble (sent as its own turn once this one finishes) when
    // steering explicitly isn't possible (no turn actually in flight
    // server-side, a non-tmux provider, or the request itself failed).
    if (sending) {
      setFocusInput('')
      fetch(`${FAMILY_API}/api/parent/steer`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conversation_id: conversationId, message: text }),
      })
        .then((res) => res.json())
        .then((data: { steered?: boolean }) => {
          if (data.steered) setParentMessages((cur) => [...cur, { role: 'user', text }])
          else setQueue((q) => [...q, text])
        })
        .catch(() => setQueue((q) => [...q, text])) // couldn't even reach the server — fall back to queued
      return
    }
    const next: ParentMsg[] = [...parentMessages, { role: 'user', text }]
    setParentMessages(next)
    setFocusInput('')
    setSuggestions([])
    // Drop any pending "new update" banner — the parent's own send supersedes
    // it, and applying the stale pre-send snapshot would wipe out the message
    // they just typed (a real bug this caused). Their send + reply, and the
    // next poll, bring things current anyway.
    setPendingConvUpdate(null)
    setSending(true)
    setLiveStatus('')
    setStreamingReply('')
    setLiveToolCalls([])
    // Live status labels AND real streamed reply content share one SSE
    // connection (see status_stream.go's sseEvent) — "status" replaces the
    // cosmetic "Quill is: …" line, "delta" appends to the live reply preview
    // shown while sending (see the streamingReply render block below). Both
    // are best-effort UX: a stream error is silently ignored, and the final
    // persisted reply (from the blocking fetch below) is always the source
    // of truth regardless of what streamed in.
    const statusSource = new EventSource(`${FAMILY_API}/api/parent/status?conversation_id=${encodeURIComponent(conversationId)}`)
    statusSource.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(ev.data) as { type?: string; text?: string; tool?: string; args?: string }
        if (parsed.type === 'delta') setStreamingReply((cur) => cur + (parsed.text ?? ''))
        else if (parsed.type === 'status') setLiveStatus(parsed.text ?? '')
        // Live tool-call visibility as each call happens (not batched at the
        // end) — see tool_call_debug.go. No result yet; the final response
        // replaces this with the same calls, results filled in.
        else if (parsed.type === 'tool_call' && parsed.tool) {
          setLiveToolCalls((cur) => [...cur, { tool: parsed.tool as string, args: parsed.args }])
        }
      } catch { /* ignore malformed event */ }
    }
    statusSource.onerror = () => statusSource.close()
    // Keep source on each message so Pulse/etc. tags survive the round-trip and
    // don't get flattened to plain replies when this turn re-persists history.
    const history = next.filter((m) => m.role === 'user' || m.role === 'assistant').map((m) => ({ role: m.role, text: m.text ?? '', source: m.source }))
    // Only pass viewer_path while the right-side panel is actually showing a
    // file (same condition the viewer JSX itself uses) — otherwise nothing is
    // really "on screen" to reference.
    const currentViewerPath = (drawerTab === 'files' || drawerTab === 'allfiles' || drawerTab === 'uploaded') ? viewerPath : ''
    fetch(`${FAMILY_API}/api/parent/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages: history, conversation_id: conversationId, viewer_path: currentViewerPath || undefined }),
    })
      .then((res) => res.json())
      .then((data: { reply?: string; error?: string; suggestions?: { label: string; message: string }[]; tool_events?: { tool: string; name?: string; grade?: string; board?: string; path?: string; parent_label?: string }[]; debug_tool_calls?: DebugToolCall[] }) => {
        const events = data.tool_events ?? []
        const toolMsgs: ParentMsg[] = events.filter((e) => e.tool === 'set_child_profile').map((e) => ({ role: 'tool', tool: e.tool, name: e.name, grade: e.grade, board: e.board }))
        const cp = events.find((e) => e.tool === 'set_child_profile')
        if (cp) { if (cp.name) setChildName(cp.name); if (cp.grade) setGrade(cp.grade); if (cp.board) setBoard(cp.board) }
        const pl = events.find((e) => e.tool === 'set_parent_label' && e.parent_label)
        if (pl?.parent_label) setParentLabel(pl.parent_label)
        const of = [...events].reverse().find((e) => e.tool === 'open_file' && e.path)
        if (of?.path) { setDrawerTab('files'); setViewerImageList([]); setViewerActivityDir(null); setViewerPath(of.path); setViewerRefreshKey((k) => k + 1) }
        const op = events.find((e) => e.tool === 'open_activity' && e.path)
        // Auto-expand so its actual content previews are visible right away —
        // otherwise an activity with multiple items shows only its title/note
        // until the parent notices and clicks the (easy-to-miss) chevron.
        if (op?.path) { setDrawerTab('files'); setViewerPath(null); setViewerActivityDir(op.path); setExpandedActivity(op.path) }
        setSuggestions(data.suggestions ?? [])
        if (data.debug_tool_calls?.length) toolMsgs.push({ role: 'tool', tool: 'debug_summary', toolCalls: data.debug_tool_calls })
        setParentMessages((cur) => [...cur, ...toolMsgs, { role: 'assistant', text: data.error ? `Sorry — ${data.error}` : (data.reply || '(no response)') }])
      })
      .catch(() => setParentMessages((cur) => [...cur, { role: 'assistant', text: 'Sorry — I couldn’t reach the learning engine.' }]))
      .finally(() => { setSending(false); setLiveStatus(''); setStreamingReply(''); setLiveToolCalls([]); statusSource.close(); setMapRefreshKey((k) => k + 1) })
  }

  const sendParentMessage = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    sendParentText(focusInput)
  }

  // Real WhatsApp connection (whatsmeow QR pairing) — see whatsapp_bot.go.
  // Once paired, incoming messages in the linked account's own "Message
  // Yourself" chat are handled directly by the backend event handler; there
  // is no frontend send path for real WhatsApp messages.
  const unpairWhatsApp = (jid: string) => {
    if (!window.confirm(`Unlink this WhatsApp number (+${jid})? You can always re-pair by scanning a new QR code.`)) return
    setUnpairingJid(jid)
    fetch(`${FAMILY_API}/api/whatsapp/unpair`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ jid }),
    })
      .then((r) => r.json())
      .then(() => {
        setWaStatus((cur) => (cur ? { ...cur, accounts: cur.accounts.filter((a) => a.jid !== jid) } : cur))
        setWaQrNonce((n) => n + 1)
      })
      .finally(() => setUnpairingJid(null))
  }

  // Toggles on-device WhatsApp voice-note transcription (Parakeet, Apple
  // Silicon only). Enabling kicks off the shared MLX voice install if it
  // isn't already there — the same install that also powers the "most
  // natural" read-aloud voice; the status poll above picks up "installing" →
  // "installed" as it progresses. Disabling does NOT delete anything: doing
  // so would silently break read-aloud too, since they share one
  // environment. Deleting it is only ever a deliberate action via a tier's
  // own "Remove" button in Settings → Voice.
  const toggleVoiceTranscription = (enabled: boolean) => {
    setVoiceToggling(true)
    fetch(`${FAMILY_API}/api/whatsapp/voice`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    })
      .then((r) => r.json())
      .then((d: { enabled: boolean; installed: boolean; installing: boolean; model_size_mb: number; available: boolean; error?: string }) => {
        setWaStatus((cur) => (cur ? { ...cur, voice_transcription: d } : cur))
      })
      .finally(() => setVoiceToggling(false))
  }

  // Child Mode tutor — talks to /api/child/message (sandboxed child agent).
  // The conversation id is the CURRENT activity's own dir (the backend now
  // derives its session/live-status key from currentActivityDir() itself, not
  // a client-generated id) — so there is exactly one child conversation per
  // activity, matching activity.json's own conversation.json.
  // modelExtra is appended to what the MODEL sees for this one message, but
  // never shown to the child or persisted in their transcript — for the
  // handoff kickoff, this is how the parent's actual guide_note instructions
  // reach Quill directly on the first turn, rather than relying on it
  // separately deciding to go read activity.json on its own initiative.
  const sendChildText = (raw: string, base?: ParentMsg[], modelExtra?: string) => {
    const text = raw.trim()
    if (!text) return
    const convId = childActivity?.dir ?? ''
    // A turn is already running — STEER is the primary path (mirrors
    // sendParentText above): try to inject it into the live turn right now.
    // Only fall back to the "queued" bubble when steering genuinely isn't
    // possible.
    if (childSending) {
      setChildInput('')
      fetch(`${FAMILY_API}/api/child/steer`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conversation_id: convId, message: text }),
      })
        .then((res) => res.json())
        .then((data: { steered?: boolean }) => {
          if (data.steered) setChildMessages((cur) => [...cur, { role: 'user', text }])
          else setChildQueue((q) => [...q, text])
        })
        .catch(() => setChildQueue((q) => [...q, text]))
      return
    }
    const next: ParentMsg[] = [...(base ?? childMessages), { role: 'user', text }]
    setChildMessages(next)
    setChildInput('')
    setChildSending(true)
    setChildLiveStatus('')
    setChildStreamingReply('')
    setChildLiveToolCalls([])
    const statusSource = new EventSource(`${FAMILY_API}/api/child/status?conversation_id=${encodeURIComponent(convId)}`)
    statusSource.onmessage = (ev) => {
      // Same JSON envelope as the parent stream ({type:"status"|"delta"|"tool_call",text,tool,args}).
      try {
        const parsed = JSON.parse(ev.data) as { type?: string; text?: string; tool?: string; args?: string }
        if (parsed.type === 'delta') setChildStreamingReply((cur) => cur + (parsed.text ?? ''))
        else if (parsed.type === 'status') setChildLiveStatus(parsed.text ?? '')
        else if (parsed.type === 'tool_call' && parsed.tool) {
          setChildLiveToolCalls((cur) => [...cur, { tool: parsed.tool as string, args: parsed.args }])
        }
      } catch { /* ignore malformed event */ }
    }
    statusSource.onerror = () => statusSource.close()
    const history = next.filter((m) => m.role === 'user' || m.role === 'assistant').map((m) => ({ role: m.role, text: m.text ?? '' }))
    if (modelExtra && history.length > 0) {
      history[history.length - 1] = { ...history[history.length - 1], text: history[history.length - 1].text + '\n\n' + modelExtra }
    }
    fetch(`${FAMILY_API}/api/child/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages: history, conversation_id: convId }),
    })
      .then((res) => res.json())
      .then((data: { reply?: string; error?: string; tool_events?: { tool: string; path?: string; focus?: string; stars?: number; total?: number; reason?: string }[]; scene?: string; debug_tool_calls?: DebugToolCall[] }) => {
        const events = data.tool_events ?? []
        const of = [...events].reverse().find((e) => e.tool === 'open_file' && e.path)
        if (of?.path) { setChildViewerFocus(of.focus ?? ''); setChildViewerPath(of.path); setChildViewerRefreshKey((k) => k + 1) }
        const cel = events.find((e) => e.tool === 'celebrate')
        // Snap the still-visible streaming bubble to the FINAL reply text
        // before swapping it for the real message. The live stream tails a
        // raw tmux pane (hard-wrapped, markdown stripped); the final reply is
        // the reconciled, properly-formatted version (see ReconcileFinalAnswer
        // in multi-llm-provider-go) — without this, the words she was
        // reading visibly change the instant the turn finishes, which reads
        // as the tutor's "thinking" being deleted and replaced. Setting this
        // first means the streaming bubble already shows the exact final
        // text by the time it's swapped for the real message, so nothing
        // visibly changes — only the bubble's own styling settles.
        if (data.reply) setChildStreamingReply(data.reply)
        setChildMessages((cur) => {
          const next: ParentMsg[] = [...cur]
          if (data.debug_tool_calls?.length) next.push({ role: 'tool', tool: 'debug_summary', toolCalls: data.debug_tool_calls })
          next.push({ role: 'assistant', text: data.error ? `Hmm, something went wrong — ${data.error}` : (data.reply || '(no response)') })
          if (cel) next.push({ role: 'tool', tool: 'celebrate', stars: cel.stars ?? 1, reason: cel.reason ?? '' })
          if (data.scene) next.push({ role: 'tool', tool: 'scene', html: data.scene })
          return next
        })
        if (childReminderSound) playReminderChime()
      })
      .catch(() => setChildMessages((cur) => [...cur, { role: 'assistant', text: 'I couldn’t reach the tutor just now — try again in a moment.' }]))
      .finally(() => { setChildSending(false); setChildLiveStatus(''); setChildStreamingReply(''); setChildLiveToolCalls([]); statusSource.close(); setChildTreeRefreshKey((k) => k + 1) })
  }

  // sendChildKickoff silently starts a turn after a handoff WITHOUT showing a
  // fake "child said this" bubble — the greeting text (naming the real
  // activity) is still sent to the model, since it needs some message to
  // respond to, but only Quill's own real reply is added to the visible
  // thread. base is the message list to keep showing beforehand (empty for a
  // fresh session, the resumed history when continuing).
  const sendChildKickoff = (greeting: string, base: ParentMsg[], modelExtra?: string) => {
    const text = greeting.trim()
    if (!text || childSending) return
    const convId = childActivity?.dir ?? ''
    const hidden: ParentMsg[] = [...base, { role: 'user', text }]
    setChildInput('')
    setChildSending(true)
    setChildLiveStatus('')
    setChildStreamingReply('')
    setChildLiveToolCalls([])
    const statusSource = new EventSource(`${FAMILY_API}/api/child/status?conversation_id=${encodeURIComponent(convId)}`)
    statusSource.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(ev.data) as { type?: string; text?: string; tool?: string; args?: string }
        if (parsed.type === 'delta') setChildStreamingReply((cur) => cur + (parsed.text ?? ''))
        else if (parsed.type === 'status') setChildLiveStatus(parsed.text ?? '')
        else if (parsed.type === 'tool_call' && parsed.tool) {
          setChildLiveToolCalls((cur) => [...cur, { tool: parsed.tool as string, args: parsed.args }])
        }
      } catch { /* ignore malformed event */ }
    }
    statusSource.onerror = () => statusSource.close()
    const history = hidden.filter((m) => m.role === 'user' || m.role === 'assistant').map((m) => ({ role: m.role, text: m.text ?? '' }))
    if (modelExtra && history.length > 0) {
      history[history.length - 1] = { ...history[history.length - 1], text: history[history.length - 1].text + '\n\n' + modelExtra }
    }
    fetch(`${FAMILY_API}/api/child/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages: history, conversation_id: convId }),
    })
      .then((res) => res.json())
      .then((data: { reply?: string; error?: string; tool_events?: { tool: string; path?: string; focus?: string; stars?: number; total?: number; reason?: string }[]; scene?: string; debug_tool_calls?: DebugToolCall[] }) => {
        const events = data.tool_events ?? []
        const of = [...events].reverse().find((e) => e.tool === 'open_file' && e.path)
        if (of?.path) { setChildViewerFocus(of.focus ?? ''); setChildViewerPath(of.path); setChildViewerRefreshKey((k) => k + 1) }
        const cel = events.find((e) => e.tool === 'celebrate')
        // Snap the still-visible streaming bubble to the FINAL reply text
        // before swapping it for the real message — see sendChildMessage's
        // identical comment above for why.
        if (data.reply) setChildStreamingReply(data.reply)
        // Append to base (not hidden) — the synthetic kickoff message never
        // joins the visible thread, only Quill's real reply (and any debug
        // tool-call bubbles / scene) do.
        setChildMessages((cur) => {
          const next: ParentMsg[] = [...cur]
          if (data.debug_tool_calls?.length) next.push({ role: 'tool', tool: 'debug_summary', toolCalls: data.debug_tool_calls })
          next.push({ role: 'assistant', text: data.error ? `Hmm, something went wrong — ${data.error}` : (data.reply || '(no response)') })
          if (cel) next.push({ role: 'tool', tool: 'celebrate', stars: cel.stars ?? 1, reason: cel.reason ?? '' })
          if (data.scene) next.push({ role: 'tool', tool: 'scene', html: data.scene })
          return next
        })
        if (childReminderSound) playReminderChime()
      })
      .catch(() => setChildMessages((cur) => [...cur, { role: 'assistant', text: 'I couldn’t reach the tutor just now — try again in a moment.' }]))
      .finally(() => { setChildSending(false); setChildLiveStatus(''); setChildStreamingReply(''); setChildLiveToolCalls([]); statusSource.close(); setChildTreeRefreshKey((k) => k + 1) })
  }

  // The SQ postMessage bridge (below) is registered once on mount, so it
  // would otherwise always call the FIRST render's sendChildText — a stale
  // closure over that render's childActivity/childMessages/etc. Routing
  // through a ref kept current every render avoids that without having to
  // tear down and re-add the window listener on every render instead.
  const sendChildTextRef = useRef(sendChildText)
  useEffect(() => { sendChildTextRef.current = sendChildText })

  // Enter Child Mode after a handoff response. new_session decides whether the
  // child continues their existing conversation (still the same activity) or
  // starts a clean one (a different activity — per-handoff resume only makes
  // sense while it's genuinely the same one). The activity's own first item
  // (if any) is opened automatically by the auto-open effect above once
  // childActivity reflects this handoff — no need to thread a file path
  // through the handoff call itself.
  const enterChildModeAfterHandoff = (newSession: boolean, greeting: string, guideNote?: string, activityTitle?: string) => {
    persistHandoffSide('tutor')
    setScreen('tutor')
    setChildTreeRefreshKey((k) => k + 1)
    // Hand the parent's real instructions to Quill directly on this first turn
    // — never shown to the child as a separate message, just extra context for
    // the model. On a brand-new session, also have Quill fold a short, plain
    // statement of the actual plan into the START of its own opening reply
    // (not a generic "let's begin!") — this is the one place the guide_note's
    // real content becomes visible to the child, since sendChildKickoff never
    // renders a synthetic message of its own.
    const modelExtra = guideNote
      ? `(For you, Quill — not from ${childName || 'the child'}: the parent's own instructions for${activityTitle ? ` "${activityTitle}"` : ' this'}: ${guideNote} Follow this pacing/order exactly.${newSession ? ` Open your very first reply with one short, plain sentence stating the actual plan in your own words (e.g. "Here's our plan: ...") before anything else — this is the only place ${childName || 'the child'} sees what this session is about, so state it concretely, not generically.` : ''})`
      : undefined
    if (newSession) {
      setChildMessages([])
      sendChildKickoff(greeting, [], modelExtra)
    } else {
      sendChildKickoff(greeting, childMessages, modelExtra)
    }
  }

  // handoffGreeting is what the child's chat "says" to kick off a handoff — it
  // reads like the child speaking to Quill, so it uses parentLabel ("mom",
  // "dad", a name) when known, falling back to "parent" until Quill has asked.
  const handoffGreeting = (what: string) => `My ${parentLabel || 'parent'} just ${what}. Can you help me get started?`

  // Does the real API call: bind the child to this activity, switch into
  // child mode, and kick off Quill — it opens the activity and guides the
  // child. No filename/path is shown; Quill composes everything the child
  // reads. resume asks the backend to keep Myra's existing conversation going
  // instead of its own same-activity heuristic.
  const performHandoff = (dir: string, greetingText: string, resume: boolean) => {
    const myGeneration = ++handoffGenerationRef.current
    fetch(`${FAMILY_API}/api/parent/handoff`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir, resume }),
    })
      .then((res) => res.json())
      .then((data: { new_session?: boolean; dir?: string; title?: string; guide_note?: string }) => {
        if (!data.dir) return
        // A newer handoff has started since this one was fired (a different
        // activity, clicked before this request finished) — its own response
        // will apply instead, so bail out here rather than starting a chat
        // for an activity the parent already navigated away from.
        if (myGeneration !== handoffGenerationRef.current) return
        enterChildModeAfterHandoff(!!data.new_session, handoffGreeting(greetingText), data.guide_note, data.title)
      })
      .catch(() => {})
  }

  // Same handoff, but re-triggered from the Files browser (create_learning_activity
  // already did the equivalent when the activity was made) — e.g. to hand off
  // an activity made earlier in the conversation. Only ASK continue-vs-fresh
  // when this is genuinely the SAME activity Myra is already partway through
  // (childActivity.dir, loaded for the assignment pill) — a different activity
  // is unambiguously a fresh handoff, no need to ask. First-ever handoff has no
  // childActivity yet, so it's fresh too.
  // title names the REAL activity in the greeting ("set up 'X' for me..."),
  // not a generic "something new" phrase that reads the same for every activity.
  const startActivityHandoff = (dir: string, title: string) => {
    const greetingText = `set up "${title}" for me to work on`
    if (childActivity?.dir === dir) {
      setPendingChildEntry({ dir, greetingText })
    } else {
      performHandoff(dir, greetingText, false)
    }
  }

  // Runs the actual handoff once the parent has answered continue-vs-fresh.
  const confirmChildEntry = (resume: boolean) => {
    const entry = pendingChildEntry
    if (!entry) return
    setPendingChildEntry(null)
    performHandoff(entry.dir, entry.greetingText, resume)
  }

  const sendChildMessage = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    sendChildText(childInput)
  }

  const fileInputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)

  // Re-measure the composer's height on every value change, not just typing.
  // The textarea's own onChange handler already calls autoGrowTextarea, but
  // voice dictation (MicButton's onText) sets the value directly via
  // setFocusInput/setChildInput — a React state update, not a DOM change
  // event — so a long transcribed message never grew the box: it landed with
  // the value already in place but the height untouched. This effect covers
  // BOTH directions (grows for a long value, shrinks back for an empty one)
  // regardless of how the value got there.
  const focusTextareaRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (focusTextareaRef.current) autoGrowTextarea(focusTextareaRef.current)
  }, [focusInput])
  const childTextareaRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (childTextareaRef.current) autoGrowTextarea(childTextareaRef.current)
  }, [childInput])

  const onPickFiles = () => fileInputRef.current?.click()

  // Shared by the file picker (onFilesSelected) AND pasting an image directly
  // into the composer (onParentComposerPaste) — same upload, same tool-card
  // result, just two different ways of getting File objects.
  const uploadParentFiles = (files: File[]) => {
    if (files.length === 0) return Promise.resolve()
    setUploading(true)
    const jobs = files.map((f) => {
      const fd = new FormData()
      fd.append('file', f)
      fd.append('scope', 'parent')
      return fetch(`${FAMILY_API}/api/upload`, { method: 'POST', body: fd })
        .then((res) => res.json())
        .then((data: { name?: string; error?: string }) => ({ name: data.name || f.name, error: data.error }))
        .catch(() => ({ name: f.name, error: 'upload failed' }))
    })
    return Promise.all(jobs)
      .then((results) => {
        const cards: ParentMsg[] = results.map((r) => ({ role: 'tool', tool: r.error ? 'upload_error' : 'upload', name: r.name }))
        setParentMessages((cur) => [...cur, ...cards])
      })
      .finally(() => setUploading(false))
  }

  const onFilesSelected = (event: ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files
    if (!files || files.length === 0) return
    uploadParentFiles(Array.from(files)).finally(() => {
      if (fileInputRef.current) fileInputRef.current.value = ''
    })
  }

  // A screenshot or copied image pasted straight into the composer (Cmd/Ctrl+V)
  // uploads immediately, same as picking it via the attach button — only
  // intercepts the paste when the clipboard actually holds image data, so a
  // normal text paste is completely unaffected.
  const onParentComposerPaste = (event: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = event.clipboardData?.items
    if (!items) return
    const imageFiles: File[] = []
    for (const item of items) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const f = item.getAsFile()
        if (f) imageFiles.push(f)
      }
    }
    if (imageFiles.length === 0) return
    event.preventDefault()
    uploadParentFiles(imageFiles)
  }

  const childFileInputRef = useRef<HTMLInputElement>(null)
  const [childUploading, setChildUploading] = useState(false)

  const onPickChildFiles = () => childFileInputRef.current?.click()

  // Shared by the file picker (onChildFilesSelected) AND pasting an image
  // directly into the composer (onChildComposerPaste). A photo of the
  // child's own work — lands directly in their current activity folder
  // (their own sandbox) so Quill can see it immediately with no parent
  // approval step. Auto-triggers a turn afterward (as if the child said so)
  // since a kid won't reliably know to say "look at this" right after
  // picking/pasting a photo.
  const uploadChildFiles = (files: File[]) => {
    if (files.length === 0) return Promise.resolve()
    setChildUploading(true)
    const jobs = files.map((f) => {
      const fd = new FormData()
      fd.append('file', f)
      fd.append('scope', 'child')
      return fetch(`${FAMILY_API}/api/upload`, { method: 'POST', body: fd })
        .then((res) => res.json())
        .then((data: { name?: string; error?: string }) => ({ name: data.name || f.name, error: data.error }))
        .catch(() => ({ name: f.name, error: 'upload failed' }))
    })
    return Promise.all(jobs)
      .then((results) => {
        const cards: ParentMsg[] = results.map((r) => ({ role: 'tool', tool: r.error ? 'upload_error' : 'upload', name: r.name }))
        const ok = results.some((r) => !r.error)
        const next = [...childMessages, ...cards]
        setChildMessages(next)
        if (ok) sendChildText('I just uploaded a photo of my work — can you take a look?', next)
      })
      .finally(() => setChildUploading(false))
  }

  const onChildFilesSelected = (event: ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files
    if (!files || files.length === 0) return
    uploadChildFiles(Array.from(files)).finally(() => {
      if (childFileInputRef.current) childFileInputRef.current.value = ''
    })
  }

  const onChildComposerPaste = (event: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = event.clipboardData?.items
    if (!items) return
    const imageFiles: File[] = []
    for (const item of items) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const f = item.getAsFile()
        if (f) imageFiles.push(f)
      }
    }
    if (imageFiles.length === 0) return
    event.preventDefault()
    uploadChildFiles(imageFiles)
  }

  if (booting) {
    return (
      <main className="learning-app" data-theme={theme}>
        <div className="fl-boot"><img src="/sparkquill-loader.svg" alt="" width={76} height={76} /><p>Starting SparkQuill…</p></div>
      </main>
    )
  }

  if (bootError) {
    return (
      <main className="learning-app" data-theme={theme}>
        <div className="fl-boot">
          <img src="/sparkquill-mark.svg" alt="" width={64} height={64} />
          <p>Couldn’t reach SparkQuill on this computer.</p>
          <button className="primary-button" type="button" onClick={() => window.location.reload()}>Try again</button>
        </div>
      </main>
    )
  }

  if (screen === 'parent') {
    return (
      <main className="learning-app" data-theme={theme}>
        <div
          ref={parentBodyRef}
          className={`fl-shell${parentResizing ? ' is-resizing' : ''}`}
          data-rail="closed"
          data-drawer={drawerOpen ? 'open' : 'closed'}
          style={{ ['--parent-side-w' as string]: `${Math.round(parentSideWidth)}px` }}
        >
          <section className="fl-center">
            <div className="fl-toolbar">
              <div className="fl-toolbar-left">
                <img className="fl-header-logo" src="/sparkquill-mark.svg" alt="" width={30} height={30} />
                <div className="fl-toolbar-title">
                  <strong className="fl-brand-word">Spark<span>Quill</span></strong>
                  <span>{childName || 'Your child'}{grade ? ` · Grade ${grade}` : ''}{board ? ` · ${board}` : ''}</span>
                </div>
              </div>
              <div className="fl-toolbar-right">
                <div className="fl-pulse-wrap">
                  <button
                    className="fl-pulse-pill"
                    type="button"
                    aria-label="Pulse"
                    title="Pulse"
                    onClick={() => setPulsePopoverOpen((v) => !v)}
                  >
                    <PulseIcon size={14} />
                    <span>Pulse</span>
                    <span className={`fl-dot ${pulseConfig?.enabled ? 'is-ready' : ''}`} />
                  </button>
                  {pulsePopoverOpen && (
                    <>
                    <div className="fl-pulse-backdrop" onClick={() => setPulsePopoverOpen(false)} />
                    <div className="fl-pulse-popover" role="dialog">
                      <div className="fl-pulse-popover-head">
                        <PulseIcon size={15} />
                        <span>Pulse</span>
                        <span className={`fl-pulse-badge ${pulseConfig?.enabled ? 'is-on' : 'is-off'}`}>
                          {pulseConfig?.enabled ? 'On' : 'Off'}
                        </span>
                        <button type="button" className="fl-pulse-popover-close" onClick={() => setPulsePopoverOpen(false)} aria-label="Close">×</button>
                      </div>
                      <div className="fl-pulse-body">
                        <div className="fl-pulse-col">
                          <p className="fl-pulse-popover-desc">Quill checks in on its own now and then — reviewing recent activity, keeping the progress report and academic map current.</p>
                          <button
                            type="button"
                            className="fl-pulse-toggle"
                            disabled={savingPulse || !pulseConfig}
                            onClick={() => savePulseConfig({ enabled: !pulseConfig?.enabled })}
                          >
                            <span className={`fl-pulse-toggle-track ${pulseConfig?.enabled ? 'is-on' : ''}`}>
                              <span className="fl-pulse-toggle-thumb" />
                            </span>
                            {pulseConfig?.enabled ? 'Turn off' : 'Turn on'}
                          </button>
                          <label className="fl-pulse-config-row">
                            <span>Check every</span>
                            <select
                              value={pulseConfig?.cadence_hours ?? 24}
                              disabled={savingPulse || !pulseConfig}
                              onChange={(e) => savePulseConfig({ cadence_hours: Number(e.target.value) })}
                            >
                              <option value={6}>6 hours</option>
                              <option value={12}>12 hours</option>
                              <option value={24}>24 hours (daily)</option>
                              <option value={72}>3 days</option>
                              <option value={168}>weekly</option>
                            </select>
                          </label>
                          <label className="fl-pulse-config-row">
                            <input
                              type="checkbox"
                              checked={pulseConfig?.preferred_hour_set ?? false}
                              disabled={savingPulse || !pulseConfig}
                              onChange={(e) => savePulseConfig({ preferred_hour_set: e.target.checked })}
                            />
                            <span>Around a specific time</span>
                            <select
                              value={pulseConfig?.preferred_hour ?? 8}
                              disabled={savingPulse || !pulseConfig || !pulseConfig?.preferred_hour_set}
                              onChange={(e) => savePulseConfig({ preferred_hour: Number(e.target.value), preferred_hour_set: true })}
                            >
                              {Array.from({ length: 24 }, (_, h) => (
                                <option key={h} value={h}>
                                  {h === 0 ? '12:00 AM' : h < 12 ? `${h}:00 AM` : h === 12 ? '12:00 PM' : `${h - 12}:00 PM`}
                                </option>
                              ))}
                            </select>
                          </label>
                          <div className="fl-pulse-popover-meta">
                            <span>Last check-in</span>
                            <span>{pulseConfig?.last_run_at ? new Date(pulseConfig.last_run_at).toLocaleString() : 'Not yet'}</span>
                          </div>
                          {pulseConfig?.enabled && (
                            <div className="fl-pulse-popover-meta">
                              <span>Next check-in</span>
                              <span>
                                {pulseConfig.last_run_at
                                  ? (() => {
                                      // Mirrors the backend's own due-check (pulse.go
                                      // startPulseTicker): the cadence window alone can
                                      // land at any hour, so PreferredHour only ever
                                      // pushes it LATER, same day, never earlier — a
                                      // cadence that elapses at 2pm with a preferred
                                      // hour of 8am still fires right away.
                                      const next = new Date(new Date(pulseConfig.last_run_at).getTime() + pulseConfig.cadence_hours * 3600_000)
                                      if (pulseConfig.preferred_hour_set && next.getHours() < pulseConfig.preferred_hour) {
                                        next.setHours(pulseConfig.preferred_hour, 0, 0, 0)
                                      }
                                      return next.toLocaleString()
                                    })()
                                  : `within ${pulseConfig.cadence_hours}h`}
                              </span>
                            </div>
                          )}
                          <button
                            type="button"
                            className="fl-pulse-run-now"
                            disabled={pulseRunning || !pulseConfig}
                            onClick={runPulseNow}
                          >
                            {pulseRunning ? 'Running… (a few minutes)' : 'Run now (test it)'}
                          </button>
                          {pulseRunError && <p className="fl-pulse-run-error">{pulseRunError}</p>}
                        </div>

                        <div className="fl-pulse-col">
                          <p className="fl-pulse-config-hint">Websites to check (optional) — any pages Quill should look at: a school portal, a class site, anything. One per line. Uses your signed-in browser (Connectors → Browser).</p>
                          <textarea
                            className="fl-pulse-config-input"
                            rows={3}
                            placeholder={"https://portal.myraschool.edu/assignments\nhttps://classroom.google.com/..."}
                            value={watchSitesDraft}
                            onChange={(e) => { setWatchSitesDraft(e.target.value); setPulseSaved(false) }}
                          />
                        </div>
                      </div>
                      <button
                        type="button"
                        className={`fl-pulse-save ${pulseSaved ? 'is-saved' : ''}`}
                        disabled={savingPulse}
                        onClick={() => {
                          const sites = watchSitesDraft.split(/[\n,]/).map((s) => s.trim()).filter(Boolean)
                          savePulseConfig({ watch_sites: sites })
                          setPulseSaved(true)
                        }}
                      >
                        {savingPulse ? 'Saving…' : pulseSaved ? 'Saved ✓' : 'Save'}
                      </button>
                    </div>
                    </>
                  )}
                </div>
                <button className="fl-icon-btn" type="button" aria-label="Settings" title="Settings" onClick={() => setSettingsOpen(true)}>
                  <SettingsIcon size={18} />
                </button>
                <button className="fl-whatsapp-btn" type="button" aria-label="WhatsApp" title="SparkQuill on WhatsApp" onClick={() => setWaOpen(true)}>
                  <svg viewBox="0 0 24 24" width="19" height="19" fill="currentColor" aria-hidden="true"><path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51l-.57-.01c-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.71.306 1.263.489 1.694.626.712.226 1.36.194 1.872.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/></svg>
                </button>
              </div>
            </div>

            {pendingConvUpdate && (
              <button
                type="button"
                className="fl-new-update-banner"
                onClick={() => {
                  // Re-fetch the CURRENT file on tap rather than applying the
                  // snapshot captured when the banner appeared — the snapshot
                  // can be stale (e.g. the parent sent a message after it was
                  // taken), and applying it would drop that message.
                  setPendingConvUpdate(null)
                  fetch(`${FAMILY_API}/api/workspace/file?path=${encodeURIComponent('conversations/parent.json')}`)
                    .then((r) => r.json())
                    .then((d) => {
                      if (!d?.content) return
                      const c = JSON.parse(d.content) as { messages?: StoredMsg[] }
                      setParentMessages((c.messages || []).map(toParentMsg))
                      setRemoteStatus('')
                      setRemoteToolCalls([])
                    })
                    .catch(() => {})
                }}
              >
                <RefreshCw size={14} /> New update — tap to refresh
              </button>
            )}
            <div className="fl-thread" aria-label="Parent learning conversation">
              <div className="fl-msg is-agent">
                <span className="fl-msg-avatar is-sun"><Sun size={18} /></span>
                <div className="fl-msg-col">
                  <div className="fl-bubble">Hi! I’m Quill, {childName || 'your child'}’s learning guide. Tell me what {childName || 'your child'} is working on, or ask me to explain progress, make study material, or create a test.</div>
                </div>
              </div>

              {hiddenParentCount > 0 && (
                <button
                  type="button"
                  className="fl-load-earlier"
                  onClick={() => setParentVisibleCount((c) => c + PARENT_HISTORY_PAGE_SIZE)}
                >
                  Load {Math.min(hiddenParentCount, PARENT_HISTORY_PAGE_SIZE)} earlier message{Math.min(hiddenParentCount, PARENT_HISTORY_PAGE_SIZE) === 1 ? '' : 's'}
                </button>
              )}
              {parentRenderGroups.map((g, gi) => {
                if (g.kind === 'photos') {
                  return (
                    <div key={`photos-${gi}`} className="fl-msg is-agent">
                      <span className="fl-msg-avatar is-sun"><Paperclip size={16} /></span>
                      <div className="fl-msg-col">
                        <div className="fl-photo-row">
                          {g.paths.map((p, pi) => {
                            const rawUrl = `${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(p)}`
                            return (
                              <a key={pi} href={rawUrl} target="_blank" rel="noopener noreferrer" className="fl-photocard">
                                <img src={rawUrl} alt="Photo received on WhatsApp" loading="lazy" />
                              </a>
                            )
                          })}
                        </div>
                      </div>
                    </div>
                  )
                }
                const { msg: m, index: i } = g
                if (m.role === 'tool') {
                  if (m.tool === 'debug_summary') {
                    return <ToolCallSummary key={i} calls={m.toolCalls ?? []} />
                  }
                  if (m.tool === 'video' && m.path) {
                    const rawUrl = `${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(m.path)}`
                    return (
                      <div key={i} className="fl-msg is-agent">
                        <span className="fl-msg-avatar is-sun"><Paperclip size={16} /></span>
                        <div className="fl-msg-col">
                          <video className="fl-videocard" src={rawUrl} controls preload="metadata" />
                        </div>
                      </div>
                    )
                  }
                  if (m.tool === 'voice_failed' && m.path) {
                    const rawUrl = `${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(m.path)}`
                    return (
                      <div key={i} className="fl-msg is-agent">
                        <span className="fl-msg-avatar is-sun"><Mic size={16} /></span>
                        <div className="fl-msg-col">
                          <div className="fl-toolcard is-error"><Mic size={15} /> <span>Voice note received — couldn’t transcribe it</span></div>
                          <audio className="fl-audiocard" src={rawUrl} controls preload="metadata" />
                        </div>
                      </div>
                    )
                  }
                  if (m.tool === 'upload' || m.tool === 'upload_error') {
                    const bad = m.tool === 'upload_error'
                    return (
                      <div key={i} className="fl-msg is-agent">
                        <span className="fl-msg-avatar is-sun"><Paperclip size={16} /></span>
                        <div className="fl-msg-col">
                          <div className={`fl-toolcard ${bad ? 'is-error' : 'is-upload'}`}><Paperclip size={15} /> <span>{bad ? <>Couldn’t add <strong>{m.name}</strong></> : <>Added material — <strong>{m.name}</strong></>}</span></div>
                        </div>
                      </div>
                    )
                  }
                  return (
                    <div key={i} className="fl-msg is-agent">
                      <span className="fl-msg-avatar is-sun"><Check size={18} strokeWidth={3} /></span>
                      <div className="fl-msg-col">
                        <div className="fl-toolcard"><Check size={15} strokeWidth={3} /> <span>Saved <strong>{m.name || 'child profile'}</strong>{m.grade ? ` · Grade ${m.grade}` : ''}{m.board ? ` · ${m.board}` : ''}</span></div>
                      </div>
                    </div>
                  )
                }
                if (m.source === 'pulse' && m.role === 'user') {
                  // The Pulse trigger — shown as a clear, centered "check-in
                  // ran" divider rather than a fake parent bubble (it isn't
                  // something the parent typed), so the whole automated turn
                  // is visible: this divider, then Quill's reply below it.
                  return (
                    <div key={i} className="fl-pulse-divider">
                      <PulseIcon size={13} /> <span>{m.text}</span>
                    </div>
                  )
                }
                return (
                  <div key={i} className={`fl-msg ${m.role === 'user' ? 'is-parent' : 'is-agent'}`}>
                    {m.role === 'user' ? (
                      <>
                        <div className="fl-msg-col"><div className="fl-bubble">{m.text}</div></div>
                        <span className="fl-msg-avatar is-parent">{initial}</span>
                      </>
                    ) : (
                      <>
                        <span className={`fl-msg-avatar ${m.source === 'pulse' ? 'is-pulse' : 'is-sun'}`}>{m.source === 'pulse' ? <PulseIcon size={17} /> : <Sun size={18} />}</span>
                        <div className="fl-msg-col">
                          <div className={`fl-bubble ${m.source === 'pulse' ? 'is-pulse' : ''}`}>
                            <Markdown text={m.text ?? ''} />
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                )
              })}

              {!sending && (remoteStatus || remoteToolCalls.length > 0) && (
                <div className="fl-msg is-agent">
                  <span className="fl-msg-avatar is-sun"><Sun size={18} /></span>
                  <div className="fl-msg-col">
                    <div className="fl-thinking">
                      <img src="/sparkquill-loader.svg" alt="" width={38} height={38} />
                      <span>{remoteStatus ? `Quill is: ${remoteStatus}… (from WhatsApp)` : 'Working on a message sent from WhatsApp…'}</span>
                    </div>
                    <ToolCallSummary calls={remoteToolCalls} />
                  </div>
                </div>
              )}
              {sending && (
                <div className="fl-msg is-agent">
                  <span className="fl-msg-avatar is-sun"><Sun size={18} /></span>
                  <div className="fl-msg-col">
                    {streamingReply && (
                      <div className="fl-bubble is-streaming"><Markdown text={stabilizeStreamingMarkdown(streamingReply)} /></div>
                    )}
                    <div className="fl-thinking">
                      {!streamingReply && <img src="/sparkquill-loader.svg" alt="" width={38} height={38} />}
                      <span>{liveStatus ? `Quill is: ${liveStatus}…` : PARENT_WAIT_HINTS[parentHintIndex]}</span>
                    </div>
                    <ToolCallSummary calls={liveToolCalls} />
                  </div>
                </div>
              )}

              {queue.map((q, i) => (
                <div key={`q-${i}`} className="fl-msg is-parent">
                  <div className="fl-msg-col"><div className="fl-bubble is-queued">{q} <span className="fl-queued-tag">queued</span></div></div>
                  <span className="fl-msg-avatar is-parent">{initial}</span>
                </div>
              ))}

              {parentMessages.length === 0 && !sending && (
                <div className="parent-quick-actions" aria-label="Suggested parent requests">
                  <button type="button" onClick={() => setFocusInput(`How is ${childName || 'my child'} doing so far?`)}>Understand progress</button>
                  <button type="button" onClick={() => setFocusInput('Make a short revision worksheet for my child')}>Create study material</button>
                  <button type="button" onClick={() => setFocusInput('Create a short practice test for my child')}>Create a test</button>
                </div>
              )}
              {suggestions.length > 0 && !sending && (
                <div className="fl-suggestions" aria-label="Recommended next steps">
                  {suggestions.map((s, i) => (
                    <button key={i} type="button" className="fl-suggestion" onClick={() => sendParentText(s.message)}>{s.label}</button>
                  ))}
                </div>
              )}
              <div ref={threadEndRef} />
            </div>

            <form className="fl-composer" onSubmit={sendParentMessage}>
              <input ref={fileInputRef} type="file" multiple accept="image/*,application/pdf" onChange={onFilesSelected} style={{ display: 'none' }} />
              <button className="composer-icon" type="button" aria-label="Attach a photo or PDF" onClick={onPickFiles} disabled={uploading}><Paperclip size={19} /></button>
              <button
                type="button"
                className={`composer-icon ${fastMode ? 'is-active' : ''}`}
                aria-label="Fast mode"
                aria-pressed={fastMode}
                title={fastMode ? 'Fast mode is on — quicker, lighter replies. Tap to turn off.' : 'Turn on fast mode for quicker (lighter) replies'}
                onClick={() => toggleFastMode(!fastMode)}
                disabled={savingFastMode}
              >
                <Zap size={19} />
              </button>
              <MicButton
                onText={(text) => {
                  setFocusInput((cur) => (cur ? `${cur} ${text}` : text))
                  // So Enter immediately sends — without this the composer
                  // stays unfocused after dictation and Enter does nothing.
                  focusTextareaRef.current?.focus()
                }}
                disabled={uploading}
                shortcutEnabled={screen === 'parent'}
              />
              <textarea
                ref={focusTextareaRef}
                aria-label="Message the learning guide"
                placeholder={sending ? 'Quill is replying — your next message will be queued…' : `Ask anything about ${childName || 'your child'}’s learning…`}
                value={focusInput}
                rows={1}
                onChange={(event) => { setFocusInput(event.target.value); autoGrowTextarea(event.target) }}
                onPaste={onParentComposerPaste}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    sendParentText(focusInput)
                  }
                }}
              />
              <div className="fl-composer-menu">
                {menuOpen && <div className="fl-menu-backdrop" onClick={() => setMenuOpen(false)} />}
                <button type="button" className="composer-icon" aria-label="Quick actions" aria-expanded={menuOpen} onClick={() => setMenuOpen((v) => !v)}><Sparkles size={19} /></button>
                {menuOpen && (
                  <div className="fl-menu" role="menu">
                    {QUICK_SKILLS.map((s) => (
                      <button key={s.label} type="button" role="menuitem" onClick={() => { setMenuOpen(false); sendParentText(s.message) }}>{s.label}</button>
                    ))}
                  </div>
                )}
              </div>
              <button className="composer-send" type="submit" aria-label="Send message" disabled={!focusInput.trim()}><Send size={18} /></button>
            </form>
            <p className="fl-disclaimer">SparkQuill can make mistakes. Please review important content before sharing it with {childName || 'your child'}.</p>
          </section>
          <div
            className="fl-parent-resizer"
            role="separator"
            aria-orientation="vertical"
            aria-label="Drag to resize the workspace panel"
            aria-valuenow={Math.round(parentSideWidth)}
            aria-valuemin={PARENT_SIDE_MIN}
            aria-valuemax={Math.round(parentSideMax(windowWidth))}
            tabIndex={0}
            onPointerDown={startParentResize}
            onKeyDown={(e) => {
              const step = e.shiftKey ? 80 : 24
              if (e.key === 'ArrowLeft') { e.preventDefault(); commitParentSideWidth(parentSideWidth + step) }
              else if (e.key === 'ArrowRight') { e.preventDefault(); commitParentSideWidth(parentSideWidth - step) }
              else if (e.key === 'Home') { e.preventDefault(); commitParentSideWidth(PARENT_SIDE_DEFAULT) }
            }}
          >
            <span className="fl-parent-resizer-grip" aria-hidden="true" />
          </div>

          <aside className="fl-drawer" aria-label="Learning workspace">
            {!((drawerTab === 'files' || drawerTab === 'allfiles' || drawerTab === 'uploaded') && viewerPath) && (
              <div className="fl-drawer-tabs" role="tablist" aria-label="Workspace views">
                <button role="tab" aria-selected={drawerTab === 'map'} className={drawerTab === 'map' ? 'is-active' : ''} type="button" onClick={() => setDrawerTab('map')}>Academics</button>
                <button role="tab" aria-selected={drawerTab === 'progress'} className={drawerTab === 'progress' ? 'is-active' : ''} type="button" onClick={() => setDrawerTab('progress')}>Progress</button>
                <button role="tab" aria-selected={drawerTab === 'week'} className={drawerTab === 'week' ? 'is-active' : ''} type="button" onClick={() => setDrawerTab('week')}>This Week</button>
                <button role="tab" aria-selected={drawerTab === 'files'} className={drawerTab === 'files' ? 'is-active' : ''} type="button" onClick={() => setDrawerTab('files')}>Workspace</button>
                <button role="tab" aria-selected={drawerTab === 'uploaded'} className={drawerTab === 'uploaded' ? 'is-active' : ''} type="button" onClick={() => setDrawerTab('uploaded')}>Uploaded</button>
                {/* Browsing every raw file is a power-user escape hatch, not a
                    peer of the four content views — an icon keeps it one tap
                    away without competing with them for attention. */}
                <button
                  type="button"
                  className={`fl-icon-btn fl-allfiles-btn${drawerTab === 'allfiles' ? ' is-active' : ''}`}
                  aria-label="Browse all files"
                  aria-pressed={drawerTab === 'allfiles'}
                  title="Browse all files"
                  onClick={() => setDrawerTab(drawerTab === 'allfiles' ? 'files' : 'allfiles')}
                >
                  <FolderOpen size={15} />
                </button>
                <button
                  type="button"
                  className="fl-icon-btn fl-refresh-btn"
                  aria-label="Refresh workspace"
                  title="Refresh"
                  onClick={() => { setWsRefreshKey((k) => k + 1); setMapRefreshKey((k) => k + 1) }}
                >
                  <RefreshCw size={15} />
                </button>
              </div>
            )}

            <div className="fl-drawer-scroll">
              {drawerTab === 'assets' && (
                <>
                  {(() => {
                    if (wsFiles.length === 0) {
                      return <p className="fl-note">No materials yet. Use the attach button to add photos or PDFs — they’ll appear here, organized by subject and topic, for Quill to read.</p>
                    }
                    const groups: Record<string, WsFile[]> = {}
                    wsFiles.forEach((f) => { const k = f.subject || 'General'; (groups[k] = groups[k] || []).push(f) })
                    return Object.entries(groups).map(([subj, files]) => (
                      <section key={subj} className="fl-asset-group">
                        <p className="fl-drawer-label">{subj}</p>
                        {files.map((f) => (
                          <div key={f.path} className="fl-asset">
                            <span className="fl-asset-icon"><FileGlyph name={f.name} size={17} /></span>
                            <span className="fl-asset-body"><strong>{f.name}</strong><small>{f.topic || 'material'}</small></span>
                          </div>
                        ))}
                      </section>
                    ))
                  })()}
                  <p className="fl-callout"><span className="fl-dot is-ready" /> Materials live in the family workspace on this computer. Quill reads them to explain progress and create study material.</p>
                </>
              )}

              {drawerTab === 'map' && (
                mapHtml === null ? (
                  <p className="fl-note">Loading the academic map…</p>
                ) : mapHtml.includes('living view grows as') ? (
                  // Still the startup placeholder seedWorkspace() writes before the
                  // agent has ever run create-academic-map — an honest empty state
                  // rather than a blank iframe.
                  <p className="fl-note">The academic map hasn't been built yet — ask Quill to "update the academic map" once there's some material to show.</p>
                ) : (
                  <iframe className="fl-map-frame" title="Academic map" sandbox="allow-scripts" srcDoc={mapHtml} />
                )
              )}

              {drawerTab === 'progress' && (
                <>
                  {progressHtml === null ? (
                    <p className="fl-note">Loading the progress report…</p>
                  ) : progressHtml.includes('living report grows as') ? (
                    <p className="fl-note">The progress report hasn't been built yet — ask Quill to "update the progress report" once there's some real activity to show.</p>
                  ) : (
                    <iframe className="fl-map-frame" title="Progress report" sandbox="allow-scripts" srcDoc={progressHtml} />
                  )}
                </>
              )}

              {drawerTab === 'week' && (
                <div className="fl-week">
                  <div className="fl-week-nav">
                    <button type="button" className="fl-week-nav-btn" onClick={() => setWeekOffset((o) => o - 1)} aria-label="Previous week">← Previous</button>
                    <span className="fl-week-range">
                      {weekData ? `${new Date(weekData.week_start).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${new Date(weekData.week_end).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}` : 'Loading…'}
                      {weekOffset === 0 && <span className="fl-week-badge">This week</span>}
                    </span>
                    <button type="button" className="fl-week-nav-btn" onClick={() => setWeekOffset((o) => o + 1)} aria-label="Next week">Next →</button>
                  </div>

                  {weekData && weekData.upcoming_deadlines && weekData.upcoming_deadlines.length > 0 && (
                    <div className="fl-week-deadlines">
                      <p className="fl-drawer-label">Coming up</p>
                      {weekData.upcoming_deadlines.map((d, i) => (
                        <div key={i} className="fl-week-deadline-row">
                          <span className={`fl-week-deadline-kind is-${d.kind || 'assignment'}`}>{d.kind === 'test' ? 'Test' : 'Due'}</span>
                          <span className="fl-week-deadline-title">{d.title}</span>
                          {d.subject && <span className="fl-week-deadline-subject">{d.subject}</span>}
                          <span className="fl-week-deadline-date">{d.due_date ? new Date(d.due_date).toLocaleDateString(undefined, { weekday: 'short' }) : ''}</span>
                        </div>
                      ))}
                    </div>
                  )}

                  {weekData ? (
                    <div className="fl-week-grid">
                      {weekData.days.map((day) => {
                        const isToday = day.date === new Date().toLocaleDateString('en-CA')
                        return (
                          <div key={day.date} className={`fl-week-day${isToday ? ' is-today' : ''}`}>
                            <div className="fl-week-day-head">
                              <strong>{day.weekday.slice(0, 3)}</strong>
                              <span>{new Date(day.date).getDate()}</span>
                            </div>
                            {(day.schedule ?? []).map((s, i) => (
                              <div key={i} className="fl-week-block is-busy" title={`${s.start}–${s.end}`}>{s.label}</div>
                            ))}
                            {(day.activities ?? []).map((a, i) => (
                              <div key={i} className="fl-week-block is-activity" title={a.title}>{a.title}</div>
                            ))}
                            {(day.deadlines ?? []).map((d, i) => (
                              <div key={i} className={`fl-week-block is-deadline is-${d.kind || 'assignment'}`} title={d.title}>{d.kind === 'test' ? '📝 ' : '📌 '}{d.title}</div>
                            ))}
                            {(day.schedule ?? []).length === 0 && (day.activities ?? []).length === 0 && (day.deadlines ?? []).length === 0 && (
                              <p className="fl-week-day-free">Free</p>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  ) : (
                    <p className="fl-note">Loading this week…</p>
                  )}

                  {!scheduleEditorOpen ? (
                    <button type="button" className="fl-week-edit-schedule" onClick={openScheduleEditor}>Edit recurring schedule</button>
                  ) : (
                    <div className="fl-week-editor">
                      <p className="fl-drawer-label">Recurring weekly schedule</p>
                      {scheduleDraft.map((e, i) => (
                        <div key={i} className="fl-week-editor-row">
                          <select value={e.day} onChange={(ev) => setScheduleDraft((rows) => rows.map((r, j) => j === i ? { ...r, day: ev.target.value } : r))}>
                            {['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((d) => <option key={d} value={d}>{d}</option>)}
                          </select>
                          <input type="time" value={e.start} onChange={(ev) => setScheduleDraft((rows) => rows.map((r, j) => j === i ? { ...r, start: ev.target.value } : r))} />
                          <input type="time" value={e.end} onChange={(ev) => setScheduleDraft((rows) => rows.map((r, j) => j === i ? { ...r, end: ev.target.value } : r))} />
                          <input type="text" placeholder="Label, e.g. School" value={e.label} onChange={(ev) => setScheduleDraft((rows) => rows.map((r, j) => j === i ? { ...r, label: ev.target.value } : r))} />
                          <button type="button" className="fl-icon-btn" aria-label="Remove" onClick={() => setScheduleDraft((rows) => rows.filter((_, j) => j !== i))}>×</button>
                        </div>
                      ))}
                      <div className="fl-week-editor-actions">
                        <button type="button" onClick={() => setScheduleDraft((rows) => [...rows, { day: 'Monday', start: '08:00', end: '14:30', label: '' }])}>+ Add a commitment</button>
                        <span className="fl-week-editor-spacer" />
                        <button type="button" onClick={() => setScheduleEditorOpen(false)} disabled={savingSchedule}>Cancel</button>
                        <button type="button" className="fl-week-editor-save" onClick={saveSchedule} disabled={savingSchedule}>{savingSchedule ? 'Saving…' : 'Save'}</button>
                      </div>
                    </div>
                  )}
                </div>
              )}

              {(drawerTab === 'files' || drawerTab === 'allfiles' || drawerTab === 'uploaded') && viewerPath ? (
                <div className="fl-viewer">
                  <div className="fl-viewer-bar">
                    {/* Clearing only viewerPath (never viewerActivityDir here) means
                        "back" naturally falls through to the activity-detail
                        branch below when this file was opened by clicking an
                        item INSIDE that activity — returning to the activity,
                        not the raw file list, exactly as browsing normally
                        expects. A file opened from the general list (where
                        viewerActivityDir is already null) still falls through to
                        that list, unchanged. */}
                    <button className="fl-viewer-back" type="button" onClick={() => setViewerPath(null)}><ArrowLeft size={15} /> Files</button>
                    <span className="fl-viewer-name">{viewerPath.split('/').pop()}</span>
                    {viewerActivityDir && (() => {
                      const act = activities.find((a) => a.dir === viewerActivityDir)
                      return act ? (
                        <button className="fl-give-to-child" type="button" disabled={sending} onClick={() => startActivityHandoff(act.dir, act.title)}>
                          Give to {childName || 'child'}
                        </button>
                      ) : null
                    })()}
                    <button
                      className="fl-icon-btn"
                      type="button"
                      aria-label="Refresh"
                      title="Reload this file"
                      onClick={() => setViewerRefreshKey((k) => k + 1)}
                    >
                      <RefreshCw size={14} />
                    </button>
                    {viewerMeta && (
                      <button
                        className={`fl-icon-btn${metaOpen ? ' is-active' : ''}`}
                        type="button"
                        aria-label="About this file"
                        aria-pressed={metaOpen}
                        title="What Quill knows about this file"
                        onClick={() => setMetaOpen((v) => !v)}
                      >
                        <Info size={14} />
                      </button>
                    )}
                    {isPrintable(viewerPath) && (
                      <button
                        className="fl-icon-btn"
                        type="button"
                        aria-label="Print"
                        title="Print this page"
                        onClick={() => printFile(viewerPath)}
                      >
                        <Printer size={14} />
                      </button>
                    )}
                  </div>
                  {metaOpen && viewerMeta && <FileMetaPanel meta={viewerMeta} />}
                  {/* key={viewerPath} forces a fresh mount (replaying the
                      fl-viewer-body CSS reveal animation below) every time a
                      different file opens, even if the previous file was the
                      same content TYPE (e.g. one .md to another). */}
                  <div key={viewerPath} className="fl-viewer-body">
                  {/\.(png|jpe?g|gif|webp|svg|bmp)$/i.test(viewerPath) ? (
                    <img className="fl-viewer-img" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(viewerPath)}`} alt={viewerPath.split('/').pop() || ''} />
                  ) : /\.pdf$/i.test(viewerPath) ? (
                    // PDFs render in the browser's native viewer (with its own
                    // zoom/page controls) — the raw endpoint serves them inline
                    // with an application/pdf content type, so a plain iframe
                    // pointed straight at it is all it takes. No sandbox here: it
                    // would disable the built-in PDF viewer, and the bytes are our
                    // own workspace file, not untrusted HTML.
                    <iframe className="fl-viewer-frame" title="PDF preview" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(viewerPath)}`} />
                  ) : /\.(mp4|webm|mov|m4v)$/i.test(viewerPath) ? (
                    <video className="fl-viewer-media" controls src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(viewerPath)}`} />
                  ) : /\.(mp3|wav|m4a|aac|ogg|oga|flac|opus)$/i.test(viewerPath) ? (
                    <audio className="fl-viewer-media" controls src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(viewerPath)}`} />
                  ) : !viewerContent ? (
                    <p className="fl-note">Loading…</p>
                  ) : !viewerContent.isText ? (
                    <NonPreviewableFile path={viewerPath} meta={viewerMeta} />
                  ) : (viewerPath.endsWith('.html') || viewerPath.endsWith('.htm')) ? (
                    <iframe ref={iframeRef} className="fl-viewer-frame" title="File preview" sandbox="allow-scripts" srcDoc={viewerContent.content} />
                  ) : (viewerPath.endsWith('.md') || viewerPath.endsWith('.markdown')) ? (
                    <div className="fl-viewer-md"><Markdown text={viewerContent.content} /></div>
                  ) : (viewerPath.endsWith('.json') || viewerPath.endsWith('.jsonl')) ? (
                    <pre className="fl-viewer-pre">{formatJSONText(viewerContent.content)}</pre>
                  ) : (
                    <pre className="fl-viewer-pre">{viewerContent.content}</pre>
                  )}
                  </div>
                </div>
              ) : (drawerTab === 'files' || drawerTab === 'allfiles' || drawerTab === 'uploaded') && viewerActivityDir ? (() => {
                const act = activities.find((a) => a.dir === viewerActivityDir)
                if (!act) return <p className="fl-note">That activity is no longer available.</p>
                const expanded = expandedActivity === act.dir
                return (
                  <div className="fl-viewer">
                    <div className="fl-viewer-bar">
                      <button className="fl-viewer-back" type="button" onClick={() => setViewerActivityDir(null)}><ArrowLeft size={15} /> Files</button>
                      <span className="fl-viewer-name">{act.title}</span>
                    </div>
                    <div key={viewerActivityDir} className="fl-viewer-body fl-package-detail">
                      <div className="fl-package-detail-head">
                        <div>
                          <h2>{act.title}</h2>
                          <p className="fl-note">
                            {act.items.length > 0 ? `${act.items.length} part${act.items.length === 1 ? '' : 's'}` : 'Adaptive practice'}
                            {dateTimeLabel(act.created_at) ? ` · ${dateTimeLabel(act.created_at)}` : ''}
                          </p>
                        </div>
                        <div className="fl-package-detail-actions">
                          {act.items.length > 0 && (
                            <button
                              type="button"
                              className="fl-package-toggle"
                              aria-expanded={expanded}
                              aria-label={expanded ? 'Hide contents' : 'See what’s inside'}
                              title={expanded ? 'Hide contents' : 'See what’s inside'}
                              onClick={() => setExpandedActivity((cur) => (cur === act.dir ? null : act.dir))}
                            >
                              <ChevronDown size={14} className={expanded ? 'is-open' : ''} />
                            </button>
                          )}
                          {(act.items.length === 0 || expanded) && (
                            <button className="fl-give-to-child" type="button" disabled={sending} onClick={() => startActivityHandoff(act.dir, act.title)}>
                              Give to {childName || 'child'}
                            </button>
                          )}
                        </div>
                      </div>
                      {(act.items.length === 0 || expanded) && act.guide_note && <p className="fl-package-note">{act.guide_note}</p>}
                      {(act.items.length === 0 || expanded) && act.goal && <p className="fl-package-goal"><strong>Goal:</strong> {act.goal}</p>}
                      {expanded && act.items.length > 0 && (
                        <div className="fl-package-detail-items">
                          {act.items.map((item) => (
                            <div key={item.path} className="fl-file-item fl-package-item has-preview">
                              <ActivityItemPreview path={item.path} name={item.name} large={act.items.length === 1} />
                              <button
                                type="button"
                                className="fl-item-open-btn"
                                aria-label={`Open ${item.name}`}
                                title="Open"
                                onClick={() => { setViewerImageList([]); setViewerPath(item.path); setViewerRefreshKey((k) => k + 1) }}
                              >
                                <ExternalLink size={14} />
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                )
              })() : drawerTab === 'allfiles' ? (
                treeNodes.length === 0 ? <p className="fl-note">No files yet.</p> : (
                  <>
                    <p className="fl-tree-total">
                      <HardDrive size={13} />
                      <span>{formatBytes(treeTotalSize || treeNodes.reduce((sum, n) => sum + (n.size ?? 0), 0))} on disk</span>
                    </p>
                    <FileTree
                      nodes={treeNodes}
                      onOpen={(p) => { setViewerImageList([]); setViewerPath(p) }}
                      expanded={treeExpanded}
                      onToggle={(p, open) => setTreeExpanded((prev) => ({ ...prev, [p]: open }))}
                    />
                  </>
                )
              ) : drawerTab === 'files' ? (
                <>
                  {(() => {
                    // Hierarchy: subject -> topic -> activity -> item. Every piece of
                    // generated content IS an activity now, so this groups the
                    // structured /api/activities objects directly — no path-parsing.
                    // Raw uploads have their own "Uploaded" tab; the academic map/
                    // progress report have their own dedicated tabs, so they're not
                    // duplicated here.
                    const subjectsList = Array.from(new Set(activities.filter((a) => a.subject).map((a) => a.subject!))).sort()
                    const relevant = activities.filter((a) => !filesSubjectFilter || a.subject === filesSubjectFilter)

                    const bySubject = new Map<string, Map<string, Activity[]>>()
                    const unplaced: Activity[] = []
                    relevant.forEach((a) => {
                      if (!a.subject) { if (!filesSubjectFilter) unplaced.push(a); return }
                      if (!bySubject.has(a.subject)) bySubject.set(a.subject, new Map())
                      const topics = bySubject.get(a.subject)!
                      const topicKey = a.topic || '—'
                      if (!topics.has(topicKey)) topics.set(topicKey, [])
                      topics.get(topicKey)!.push(a)
                    })
                    // List every part, numbered, so the parent can open and preview
                    // any one of them directly — not just see a summary count.
                    const renderActivities = (acts: Activity[]) => acts.map((act) => {
                      const expanded = expandedActivity === act.dir
                      const openable = act.items.length > 0
                      const mode = activityMode(act.teaching_mode)
                      const isCurrent = childActivity?.dir === act.dir
                      // Details show for an expanded activity, and always for an
                      // adaptive one — it has no items to expand, so its guide
                      // note IS the activity.
                      const showDetails = expanded || !openable
                      return (
                        <div key={act.dir} className={`fl-act${expanded ? ' is-expanded' : ''}${isCurrent ? ' is-current' : ''}`}>
                          <button
                            type="button"
                            className="fl-act-head"
                            aria-expanded={openable ? expanded : undefined}
                            disabled={!openable}
                            onClick={() => setExpandedActivity((cur) => (cur === act.dir ? null : act.dir))}
                          >
                            <span className="fl-act-title">{act.title}</span>
                            {openable && <ChevronDown size={15} className={`fl-act-chev${expanded ? ' is-open' : ''}`} />}
                          </button>
                          <div className="fl-act-row">
                            {isCurrent
                              ? <span className="fl-act-mode is-live">With {childName || 'your child'} now</span>
                              : mode && <span className={`fl-act-mode ${mode.cls}`}>{mode.label}</span>}
                            <span className="fl-act-sub">
                              {openable ? `${act.items.length} part${act.items.length === 1 ? '' : 's'}` : 'Adaptive'}
                              {dateTimeLabel(act.created_at) ? ` · ${dateTimeLabel(act.created_at)}` : ''}
                            </span>
                            <button
                              className="fl-give-to-child"
                              type="button"
                              disabled={sending}
                              onClick={() => startActivityHandoff(act.dir, act.title)}
                            >
                              Give to {childName || 'child'}
                            </button>
                          </div>
                          {showDetails && act.guide_note && <p className="fl-package-note">{act.guide_note}</p>}
                          {showDetails && act.goal && <p className="fl-package-goal"><strong>Goal:</strong> {act.goal}</p>}
                          {expanded && act.items.map((item) => (
                            <div key={item.path} className="fl-file-item fl-package-item has-preview">
                              <ActivityItemPreview path={item.path} name={item.name} large={act.items.length === 1} />
                              <button
                                type="button"
                                className="fl-item-open-btn"
                                aria-label={`Open ${item.name}`}
                                title="Open"
                                onClick={() => { setViewerImageList([]); setViewerPath(item.path) }}
                              >
                                <ExternalLink size={14} />
                              </button>
                            </div>
                          ))}
                        </div>
                      )
                    })
                    // "By date" groups the SAME relevant/filtered activity set
                    // by calendar day instead of subject/topic — most-recent
                    // day first, activities within a day newest-first too.
                    const byDate = new Map<string, Activity[]>()
                    relevant.forEach((a) => {
                      const key = dateOnlyKey(a.created_at)
                      if (!byDate.has(key)) byDate.set(key, [])
                      byDate.get(key)!.push(a)
                    })
                    const dateGroups = Array.from(byDate.entries()).sort((a, b) => b[0].localeCompare(a[0]))
                    dateGroups.forEach(([, acts]) => acts.sort((a, b) => (b.created_at || '').localeCompare(a.created_at || '')))

                    return (
                      <div className="fl-workspace">
                        {subjectsList.length > 0 && (
                          <div className="fl-subject-bar" role="group" aria-label="Filter by subject">
                            <button
                              type="button"
                              className={filesSubjectFilter === '' ? 'is-active' : ''}
                              onClick={() => setFilesSubjectFilter('')}
                            >
                              All
                            </button>
                            {subjectsList.map((s) => (
                              <button
                                key={s}
                                type="button"
                                className={filesSubjectFilter === s ? 'is-active' : ''}
                                onClick={() => setFilesSubjectFilter(filesSubjectFilter === s ? '' : s)}
                              >
                                {s}
                              </button>
                            ))}
                          </div>
                        )}
                        <div className="fl-ws-groupby" role="group" aria-label="Group activities by">
                          <button type="button" className={filesGroupBy === 'subject' ? 'is-active' : ''} onClick={() => setFilesGroupBy('subject')}>By subject</button>
                          <button type="button" className={filesGroupBy === 'date' ? 'is-active' : ''} onClick={() => setFilesGroupBy('date')}>By date</button>
                        </div>
                        {filesGroupBy === 'date' ? (
                          dateGroups.length === 0 ? (
                            <p className="fl-note">Nothing here yet. Ask Quill to make study material or a test.</p>
                          ) : (
                            dateGroups.map(([key, acts]) => (
                              <section key={key || 'undated'} className="fl-ws-subject">
                                <h3 className="fl-ws-subject-name">
                                  {dateOnlyLabel(acts[0]?.created_at)}
                                  <span>{acts.length}</span>
                                </h3>
                                <div className="fl-ws-topic">{renderActivities(acts)}</div>
                              </section>
                            ))
                          )
                        ) : bySubject.size === 0 && unplaced.length === 0 ? (
                          <p className="fl-note">Nothing here yet. Ask Quill to make study material or a test.</p>
                        ) : (
                          <>
                            {Array.from(bySubject.entries()).map(([subj, topics]) => (
                              <section key={subj} className="fl-ws-subject">
                                <h3 className="fl-ws-subject-name">
                                  {subj}
                                  <span>{Array.from(topics.values()).reduce((n, a) => n + a.length, 0)}</span>
                                </h3>
                                {Array.from(topics.entries()).map(([top, acts]) => (
                                  <div key={top} className="fl-ws-topic">
                                    {top !== '—' && <p className="fl-ws-topic-name">{top}</p>}
                                    {renderActivities(acts)}
                                  </div>
                                ))}
                              </section>
                            ))}
                            {unplaced.length > 0 && (
                              <section className="fl-ws-subject">
                                <h3 className="fl-ws-subject-name">General<span>{unplaced.length}</span></h3>
                                <div className="fl-ws-topic">{renderActivities(unplaced)}</div>
                              </section>
                            )}
                          </>
                        )}
                      </div>
                    )
                  })()}
                </>
              ) : drawerTab === 'uploaded' ? (() => {
                // Raw parent-uploaded material (materials/<subject>/<topic>/...) —
                // its own tab, separate from Quill-generated activities.
                type Entry = { path: string; date?: string; label: string }
                const usable = allFiles.filter((p) => !p.endsWith('.meta.json') && !p.startsWith('skills/') && !p.includes('/conversations/') && !p.endsWith('child-profile.json'))
                const classified = usable
                  .filter((p) => p === 'materials' || p.startsWith('materials/'))
                  .map((p) => ({ p, ...parseMaterialPath(p) }))
                const subjectsList = Array.from(new Set(classified.filter((f) => f.subject).map((f) => f.subject!))).sort()
                const relevant = classified.filter((f) => !filesSubjectFilter || f.subject === filesSubjectFilter)

                const bySubject = new Map<string, Entry[]>()
                const general: Entry[] = []
                relevant.forEach((f) => {
                  const entry: Entry = { path: f.p, date: f.date, label: f.label }
                  if (!f.subject) { general.push(entry); return }
                  if (!bySubject.has(f.subject)) bySubject.set(f.subject, [])
                  bySubject.get(f.subject)!.push(entry)
                })
                const byDateDesc = (a: Entry, b: Entry) => (b.date || '').localeCompare(a.date || '')
                const renderEntries = (entries: Entry[]) => {
                  const sorted = [...entries].sort(byDateDesc)
                  const imagePaths = sorted.filter((e) => IMAGE_PATH_RE.test(e.path)).map((e) => e.path)
                  return (
                    <div className="fl-thumb-grid">
                      {sorted.map((e) => (
                        IMAGE_PATH_RE.test(e.path) ? (
                          <button key={e.path} type="button" className="fl-thumb-item" onClick={() => { setViewerImageList(imagePaths); setViewerPath(e.path) }}>
                            <img className="fl-thumb-img" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(e.path)}`} alt="" loading="lazy" />
                            <span className="fl-thumb-caption">{e.label}{e.date ? ` · ${e.date}` : ''}</span>
                          </button>
                        ) : (
                          <button key={e.path} type="button" className="fl-file-item" onClick={() => { setViewerImageList([]); setViewerPath(e.path) }}>
                            <FileGlyph name={e.path} size={16} />
                            <span>{e.label}{e.date ? ` · ${e.date}` : ''}</span>
                          </button>
                        )
                      ))}
                    </div>
                  )
                }
                return (
                  <>
                    {subjectsList.length > 0 && (
                      <select
                        className="fl-subject-select"
                        aria-label="Filter by subject"
                        value={filesSubjectFilter}
                        onChange={(e) => setFilesSubjectFilter(e.target.value)}
                      >
                        <option value="">All subjects</option>
                        {subjectsList.map((s) => <option key={s} value={s}>{s}</option>)}
                      </select>
                    )}
                    {bySubject.size === 0 && general.length === 0 ? (
                      <p className="fl-note">No uploaded material yet. Use the attach button to add photos or documents — they’ll appear here.</p>
                    ) : (
                      <>
                        {Array.from(bySubject.entries()).map(([subj, entries]) => (
                          <section key={subj} className="fl-asset-group">
                            <p className="fl-drawer-label">{subj}</p>
                            {renderEntries(entries)}
                          </section>
                        ))}
                        {general.length > 0 && (
                          <section className="fl-asset-group">
                            <p className="fl-drawer-label">General</p>
                            {renderEntries(general)}
                          </section>
                        )}
                      </>
                    )}
                  </>
                )
              })() : null}
            </div>
          </aside>

          {waOpen && (
            <div className="fl-wa-backdrop" role="dialog" aria-modal="true" onClick={() => setWaOpen(false)}>
              <div className="fl-connectors" onClick={(e) => e.stopPropagation()}>
                <div className="fl-wa-head">
                  <span className="fl-wa-title">Connectors</span>
                  <button className="fl-wa-close" type="button" onClick={() => setWaOpen(false)} aria-label="Close">×</button>
                </div>
                <div className="fl-connectors-body">
                  <nav className="fl-connectors-nav">
                    <button type="button" className={connectorSection === 'whatsapp' ? 'is-active' : ''} onClick={() => setConnectorSection('whatsapp')}>WhatsApp</button>
                    <button type="button" className={connectorSection === 'browser' ? 'is-active' : ''} onClick={() => setConnectorSection('browser')}>Browser</button>
                  </nav>
                  <div className="fl-connectors-panel">
                    {connectorSection === 'whatsapp' ? (
                      <div className="fl-connector-card">
                        {(waStatus?.accounts?.length ?? 0) > 0 && (
                          <>
                            <p className="fl-connector-status is-connected">
                              ✓ {waStatus!.accounts.length} number{waStatus!.accounts.length > 1 ? 's' : ''} linked
                            </p>
                            <ul className="fl-wa-account-list">
                              {waStatus!.accounts.map((a) => (
                                <li key={a.jid} className="fl-wa-account-row">
                                  <span>+{a.jid}{!a.connected ? ' (reconnecting…)' : ''}</span>
                                  <button
                                    className="fl-ghost-btn"
                                    type="button"
                                    onClick={() => unpairWhatsApp(a.jid)}
                                    disabled={unpairingJid === a.jid}
                                  >
                                    {unpairingJid === a.jid ? 'Unlinking…' : 'Unlink'}
                                  </button>
                                </li>
                              ))}
                            </ul>
                            <div className="fl-wa-howto">
                              <p className="fl-wa-howto-title">How to chat with Quill on WhatsApp</p>
                              <ol className="fl-note" style={{ paddingLeft: '1.2em', margin: '6px 0 0' }}>
                                <li>Open WhatsApp on your phone.</li>
                                <li>At the top, search for your own name — the chat labelled <strong>“(You)”</strong> or <strong>“Message yourself”</strong>.</li>
                                <li>Type anything there, like <em>“How is {childName || 'your child'} doing this week?”</em> — Quill reads it and replies right in that same chat.</li>
                              </ol>
                              <p className="fl-note" style={{ marginTop: '8px' }}>That’s it — it works just like texting. You can also send a photo of {childName || 'your child'}’s worksheet there and Quill will look at it. Quill only ever answers in your own “message yourself” chat — never in your chats with other people.</p>
                            </div>
                            {waStatus?.voice_transcription && (
                              <div className="fl-wa-voice">
                                <div className="fl-wa-voice-row">
                                  <div>
                                    <p className="fl-wa-voice-title">Understand voice notes</p>
                                    <p className="fl-note">
                                      {(() => {
                                        const vt = waStatus.voice_transcription!
                                        const sizeLabel = vt.model_size_mb >= 1000 ? `${(vt.model_size_mb / 1000).toFixed(1)}GB` : `${vt.model_size_mb}MB`
                                        if (!vt.available) return 'Needs a newer Mac (2020 or later) — not available on this computer.'
                                        if (vt.installing) return `Setting this up on your computer (~${sizeLabel}, one-time) — this can take several minutes on a home connection…`
                                        if (vt.enabled && vt.installed) return `On — voice notes are transcribed right on this computer (~${sizeLabel} used). Nothing is sent to the cloud for this. English only.`
                                        return `Let Quill understand voice notes you send on WhatsApp — English only. Transcribed entirely on this computer, a one-time ~${sizeLabel} download, no ongoing cost.`
                                      })()}
                                    </p>
                                    {waStatus.voice_transcription.error && (
                                      <p className="fl-note fl-wa-voice-error">Couldn’t set this up: {waStatus.voice_transcription.error}</p>
                                    )}
                                  </div>
                                  <label className="fl-toggle">
                                    <input
                                      type="checkbox"
                                      checked={waStatus.voice_transcription.enabled}
                                      disabled={voiceToggling || waStatus.voice_transcription.installing || !waStatus.voice_transcription.available}
                                      onChange={(e) => toggleVoiceTranscription(e.target.checked)}
                                    />
                                    <span className="fl-toggle-slider" />
                                  </label>
                                </div>
                              </div>
                            )}
                          </>
                        )}
                        <div className="fl-wa-add-another">
                          <p className="fl-note">
                            {(waStatus?.accounts?.length ?? 0) > 0
                              ? 'Add another parent — scan with a different phone:'
                              : 'Scan this code with WhatsApp on your phone:'} <strong>Settings → Linked Devices → Link a Device.</strong>
                          </p>
                          {waStatus?.pairing?.qr_available ? (
                            <img className="fl-wa-qr" src={`${FAMILY_API}/api/whatsapp/pair?n=${waQrNonce}`} alt="WhatsApp pairing QR code" />
                          ) : (
                            <div className="fl-wa-qr is-loading">Preparing QR…</div>
                          )}
                          <p className="fl-note">The code refreshes automatically every 30 seconds until scanned.</p>
                        </div>
                      </div>
                    ) : (
                      <div className="fl-connector-card">
                        <p className="fl-connector-status" style={browserStatus?.cli_installed ? { color: 'var(--fl-green, #2e7d32)' } : undefined}>
                          {browserStatus === null ? 'Checking…' : browserStatus.cli_installed ? '✓ Ready' : 'Not set up yet'}
                        </p>
                        <p className="fl-note">For things like school portals — assignments, report cards, uploaded books — the safest way for Quill to check them is to use a browser you're already signed into, so it never needs your password.</p>
                        <div className="fl-install-steps">
                          <p className="fl-note"><strong>One-time setup:</strong> copy this, paste it into the Terminal app on your Mac, and press Enter.</p>
                          <div className="fl-code-row">
                            <pre className="fl-code-block"><code>curl -fsSL 'https://raw.githubusercontent.com/manishiitg/coding-agent-loop/main/scripts/install-chrome-cdp-macOS.sh' | bash</code></pre>
                            <button
                              type="button"
                              className="fl-ghost-btn"
                              onClick={() => {
                                navigator.clipboard.writeText("curl -fsSL 'https://raw.githubusercontent.com/manishiitg/coding-agent-loop/main/scripts/install-chrome-cdp-macOS.sh' | bash")
                                setBrowserCopied(true)
                                window.setTimeout(() => setBrowserCopied(false), 2000)
                              }}
                            >
                              {browserCopied ? 'Copied!' : 'Copy'}
                            </button>
                          </div>
                          <p className="fl-note">A new browser window opens on its own once it's done.</p>
                        </div>
                        <p className="fl-note">Then sign into the school portal (or anything else you'd like Quill to check) in that window, and just leave it open. From then on, Quill can look things up there whenever it's useful — it never sees or stores your password.</p>
                        {browserStatus && !browserStatus.cli_installed && (
                          <p className="fl-note">(Also needed once: ask whoever set this computer up to run <code>npm install -g agent-browser@latest</code>.)</p>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}

          {pendingChildEntry && (
            <div className="fl-signoff-backdrop" role="dialog" aria-modal="true" aria-labelledby="fl-continue-title" onClick={() => setPendingChildEntry(null)}>
              <div className="fl-signoff-card" onClick={(e) => e.stopPropagation()}>
                <div className="fl-signoff-icon"><BookOpen size={22} /></div>
                <h2 id="fl-continue-title">Continue {childName || 'her'} chat, or start fresh?</h2>
                <p>You're about to switch to {childName || 'your child'}'s screen. Should Quill pick up in the same ongoing conversation, or begin a brand-new one for this?</p>
                <div className="fl-signoff-actions">
                  <button className="fl-ghost-btn" type="button" onClick={() => confirmChildEntry(false)}>Start fresh</button>
                  <button className="primary-button" type="button" onClick={() => confirmChildEntry(true)}>Continue her chat</button>
                </div>
              </div>
            </div>
          )}

          {settingsOpen && (
            <div className="fl-settings-backdrop" role="dialog" aria-modal="true" onClick={() => setSettingsOpen(false)}>
              <div className="fl-settings" onClick={(e) => e.stopPropagation()}>
                <div className="fl-settings-head">
                  <span className="fl-settings-title">Settings</span>
                  <button className="fl-wa-close" type="button" onClick={() => setSettingsOpen(false)} aria-label="Close">×</button>
                </div>
                <div className="fl-settings-body">
                  <p className="fl-drawer-label">Which AI Quill uses</p>
                  <p className="fl-note">The AI behind both your chat and {childName || 'your child'}’s tutor. They all work — pick whichever account you already pay for.</p>
                  {enginesState === 'loading' ? (
                    <p className="fl-note">Checking what’s available…</p>
                  ) : engines.length === 0 ? (
                    <p className="fl-note">None found on this computer yet.</p>
                  ) : (
                    <div className="fl-settings-engines">
                      {engines.map((item) => {
                        const status = engineStatus(item)
                        const active = engine === item.id
                        return (
                          <button
                            key={item.id}
                            type="button"
                            className={`fl-settings-engine-card ${active ? 'is-active' : ''}`}
                            disabled={!status.ready || savingEngine}
                            onClick={() => {
                              setEngine(item.id)
                              setSavingEngine(true)
                              fetch(`${FAMILY_API}/api/engine/selection`, {
                                method: 'POST',
                                headers: { 'Content-Type': 'application/json' },
                                body: JSON.stringify({ engine: item.id }),
                              }).finally(() => setSavingEngine(false))
                            }}
                          >
                            <span className="fl-settings-engine-col">
                              <span className="fl-settings-engine-name">{pres(item.id, item.name).name}</span>
                              <span className="fl-settings-engine-blurb">{pres(item.id, item.name).blurb}</span>
                            </span>
                            <span className={`fl-settings-engine-status ${status.ready ? 'is-ready' : ''}`}>{status.label}</span>
                            {active && <Check size={16} />}
                          </button>
                        )
                      })}
                    </div>
                  )}

                  <p className="fl-drawer-label" style={{ marginTop: '20px' }}>Fast mode</p>
                  <p className="fl-note">Trades depth for speed — a cheaper, faster model answers every chat (web, WhatsApp, and Pulse check-ins) instead of the usual one. Good for quick questions; turn it off again for anything that needs careful judgment.</p>
                  <label className="fl-pulse-config-row">
                    <input
                      type="checkbox"
                      checked={fastMode}
                      disabled={savingFastMode}
                      onChange={(e) => toggleFastMode(e.target.checked)}
                    />
                    <span>Use fast mode</span>
                  </label>

                  <VoiceSettings status={voiceStatus} childName={childName} onRefresh={refreshVoiceStatus} />

                  <p className="fl-drawer-label" style={{ marginTop: '20px' }}>Secrets</p>
                  <p className="fl-note">Credentials Quill's tools can use — e.g. a school portal login. Saved here, never through chat, so a value you type below never appears in any saved conversation. Quill only ever sees the name, never the value.</p>
                  {secretNames.length > 0 && (
                    <ul className="fl-wa-account-list">
                      {secretNames.map((name) => (
                        <li key={name} className="fl-wa-account-row">
                          <span>{name}</span>
                          <button
                            className="fl-ghost-btn"
                            type="button"
                            onClick={() => deleteSecret(name)}
                            disabled={deletingSecret === name}
                          >
                            {deletingSecret === name ? 'Removing…' : 'Remove'}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                  <div className="form-row">
                    <label>
                      <span>Name</span>
                      <input
                        type="text"
                        placeholder="e.g. school portal password"
                        value={secretNameDraft}
                        onChange={(e) => setSecretNameDraft(e.target.value)}
                      />
                    </label>
                    <label>
                      <span>Value</span>
                      <input
                        type="password"
                        placeholder="the credential itself"
                        value={secretValueDraft}
                        onChange={(e) => setSecretValueDraft(e.target.value)}
                        onKeyDown={(e) => { if (e.key === 'Enter') saveSecret() }}
                      />
                    </label>
                  </div>
                  <button
                    type="button"
                    className="fl-ghost-btn"
                    onClick={saveSecret}
                    disabled={savingSecret || !secretNameDraft.trim() || !secretValueDraft.trim()}
                  >
                    {savingSecret ? 'Saving…' : 'Save secret'}
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </main>
    )
  }

  if (screen === 'tutor') {
    return (
      <main className="learning-app" data-theme={theme}>
        <div className="fl-child">
          <div
            ref={childBodyRef}
            className={`fl-child-body${childResizing ? ' is-resizing' : ''}`}
            style={{ ['--child-side-w' as string]: `${Math.round(childSideWidth)}px` }}
          >
            <section className="fl-child-chat" style={{ ['--chat-scale' as string]: childChatZoom }}>
              <header className="fl-child-top">
                <div className="fl-child-id">
                  <img className="fl-header-logo" src="/sparkquill-mark.svg" alt="" width={30} height={30} />
                  <div className="fl-child-hi"><strong>Hi {childName || 'Maya'}!</strong><small>Let’s keep learning together</small></div>
                </div>
                {childActivity?.title && (() => {
                  const hasInfo = !!(childActivity.goal || childActivity.guide_note)
                  return (
                    <div className="fl-child-assignment-wrap">
                      {goalPopoverOpen && <div className="fl-menu-backdrop" onClick={() => setGoalPopoverOpen(false)} />}
                      <button
                        type="button"
                        className="fl-child-assignment-pill"
                        aria-expanded={hasInfo ? goalPopoverOpen : undefined}
                        aria-label={hasInfo ? `${childActivity.title} — show the goal` : childActivity.title}
                        onClick={() => hasInfo && setGoalPopoverOpen((v) => !v)}
                      >
                        <BookOpen size={14} />
                        <span>{childActivity.title}</span>
                        {hasInfo && <ChevronDown size={13} className={goalPopoverOpen ? 'is-open' : ''} />}
                      </button>
                      {goalPopoverOpen && hasInfo && (
                        <div className="fl-child-goal-popover" role="dialog">
                          {childActivity.goal && <p><strong>Goal</strong>{childActivity.goal}</p>}
                          {childActivity.guide_note && <p><strong>Plan</strong>{childActivity.guide_note}</p>}
                        </div>
                      )}
                    </div>
                  )
                })()}
                <div className="fl-child-top-right">
                  {/* Chat reading size — same cycle as the worksheet's button but
                      its own preference, since the two sides are read differently. */}
                  <button
                    className={`fl-icon-btn fl-zoom-btn${childChatZoom > 1 ? ' is-on' : ''}`}
                    type="button"
                    aria-label={`Chat text size: ${Math.round(childChatZoom * 100)}% — tap for bigger`}
                    title={childChatZoom > 1 ? `Chat text ${Math.round(childChatZoom * 100)}% — tap for bigger` : 'Make the chat text bigger'}
                    onClick={cycleChildChatZoom}
                  >
                    <Type size={14} />
                    {childChatZoom > 1 && <span className="fl-zoom-badge">{Math.round(childChatZoom * 100)}%</span>}
                  </button>
                  <button className="fl-parent-return" type="button" title="Parent Mode" onClick={() => { setGateValue(''); setGateError(''); setPinGate(true) }}><LockKeyhole size={16} /><span>Parent Mode</span></button>
                </div>
              </header>
              <div className="fl-child-thread" aria-label="Tutor conversation">
                {childRenderGroups.map((g, gi) => (
                  g.kind === 'photos' ? (
                    <div key={`photos-${gi}`} className="fl-tmsg is-tutor">
                      <span className="fl-tmsg-avatar"><Paperclip size={16} /></span>
                      <div className="fl-photo-row">
                        {g.paths.map((p, pi) => (
                          <a
                            key={pi}
                            href={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(p)}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="fl-photocard"
                          >
                            <img src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(p)}`} alt="Photo received on WhatsApp" loading="lazy" />
                          </a>
                        ))}
                      </div>
                    </div>
                  ) : (() => {
                    const { msg: m, index: i } = g
                    return m.role === 'tool' && m.tool === 'debug_summary' ? (
                    <ToolCallSummary key={i} calls={m.toolCalls ?? []} />
                  ) : m.role === 'tool' && m.tool === 'video' && m.path ? (
                    <div key={i} className="fl-tmsg is-tutor">
                      <span className="fl-tmsg-avatar"><Paperclip size={16} /></span>
                      <video className="fl-videocard" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(m.path)}`} controls preload="metadata" />
                    </div>
                  ) : m.role === 'tool' && m.tool === 'voice_failed' && m.path ? (
                    <div key={i} className="fl-tmsg is-tutor">
                      <span className="fl-tmsg-avatar"><Mic size={16} /></span>
                      <div className="fl-msg-col">
                        <div className="fl-toolcard is-error"><Mic size={15} /> <span>Voice note received — couldn’t transcribe it</span></div>
                        <audio className="fl-audiocard" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(m.path)}`} controls preload="metadata" />
                      </div>
                    </div>
                  ) : m.role === 'tool' && (m.tool === 'upload' || m.tool === 'upload_error') ? (
                    <div key={i} className="fl-tmsg is-tutor">
                      <span className="fl-tmsg-avatar"><Paperclip size={16} /></span>
                      <div className={`fl-toolcard ${m.tool === 'upload_error' ? 'is-error' : 'is-upload'}`}>
                        <Paperclip size={15} />
                        <span>{m.tool === 'upload_error' ? <>Couldn’t add your photo</> : <>Added your photo</>}</span>
                      </div>
                    </div>
                  ) : m.role === 'tool' && m.tool === 'celebrate' ? (
                    <div key={i} className="fl-celebration" role="status">
                      <span className="fl-celebration-stars">
                        {Array.from({ length: m.stars ?? 1 }, (_, si) => (
                          <Star key={si} className="fl-celebration-star" size={20} fill="currentColor" strokeWidth={1} style={{ animationDelay: `${si * 0.12}s` }} />
                        ))}
                      </span>
                      <span className="fl-celebration-text">{m.reason}</span>
                    </div>
                  ) : m.role === 'tool' && m.tool === 'scene' ? (
                    <div key={i} className="fl-tmsg is-tutor">
                      <span className="fl-tmsg-avatar"><Sun size={20} /></span>
                      <SceneFrame html={m.html ?? ''} activityDir={childActivity?.dir ?? ''} />
                    </div>
                  ) : m.role === 'assistant' ? (
                    <div key={i} className="fl-tmsg is-tutor">
                      <span className="fl-tmsg-avatar"><Sun size={20} /></span>
                      <div className="fl-tbubble">
                        <Markdown text={m.text ?? ''} />
                        <label className="fl-reminder-row">
                          <input
                            type="checkbox"
                            checked={childReminderSound}
                            onChange={(e) => toggleChildReminderSound(e.target.checked)}
                          />
                          <Bell size={13} />
                          <span>Remind me with a sound</span>
                        </label>
                      </div>
                    </div>
                  ) : (
                    <div key={i} className="fl-tmsg is-child">
                      <div className="fl-tbubble"><Markdown text={m.text ?? ''} /></div>
                      <span className="fl-tmsg-avatar is-child">{initial}</span>
                    </div>
                  )
                  })()
                ))}
                {!childSending && (childRemoteStatus || childRemoteToolCalls.length > 0) && (
                  <div className="fl-tmsg is-tutor">
                    <span className="fl-tmsg-avatar"><Sun size={20} /></span>
                    <div className="fl-tbubble-col">
                      <div className="fl-thinking">
                        <img src="/sparkquill-loader.svg" alt="" width={38} height={38} />
                        <span>{childRemoteStatus ? `Quill is: ${childRemoteStatus}… (from WhatsApp)` : 'Working on a message sent from WhatsApp…'}</span>
                      </div>
                      <ToolCallSummary calls={childRemoteToolCalls} />
                    </div>
                  </div>
                )}
                {childSending && (
                  <div className="fl-tmsg is-tutor">
                    <span className="fl-tmsg-avatar"><Sun size={20} /></span>
                    <div className="fl-tbubble-col">
                      {childStreamingReply && (
                        <div className="fl-tbubble is-streaming"><Markdown text={stabilizeStreamingMarkdown(childStreamingReply)} /></div>
                      )}
                      <div className="fl-thinking">
                        {!childStreamingReply && <img src="/sparkquill-loader.svg" alt="" width={38} height={38} />}
                        <span>{childLiveStatus ? `Quill is: ${childLiveStatus}…` : CHILD_WAIT_HINTS[childHintIndex]}</span>
                      </div>
                      <ToolCallSummary calls={childLiveToolCalls} />
                    </div>
                  </div>
                )}
                {childQueue.map((q, i) => (
                  <div key={`cq-${i}`} className="fl-tmsg is-child">
                    <div className="fl-tbubble is-queued">{q} <span className="fl-queued-tag">queued</span></div>
                    <span className="fl-tmsg-avatar is-child">{initial}</span>
                  </div>
                ))}
                <div ref={childThreadEndRef} />
              </div>
              <form className="fl-child-composer" onSubmit={sendChildMessage}>
                <input ref={childFileInputRef} type="file" multiple accept="image/*" onChange={onChildFilesSelected} style={{ display: 'none' }} />
                <button className="composer-icon" type="button" aria-label="Attach a photo of your work" onClick={onPickChildFiles} disabled={childSending || childUploading}><Paperclip size={19} /></button>
                <button
                  type="button"
                  className={`composer-icon ${fastMode ? 'is-active' : ''}`}
                  aria-label="Fast mode"
                  aria-pressed={fastMode}
                  title={fastMode ? 'Fast mode is on — quicker, lighter replies. Tap to turn off.' : 'Turn on fast mode for quicker (lighter) replies'}
                  onClick={() => toggleFastMode(!fastMode)}
                  disabled={savingFastMode}
                >
                  <Zap size={19} />
                </button>
                <MicButton
                  onText={(text) => {
                    setChildInput((cur) => (cur ? `${cur} ${text}` : text))
                    // So Enter immediately sends — without this the composer
                    // stays unfocused after dictation and Enter does nothing.
                    childTextareaRef.current?.focus()
                  }}
                  disabled={childUploading}
                  shortcutEnabled={screen === 'tutor'}
                />
                <textarea
                  ref={childTextareaRef}
                  aria-label="Message your tutor"
                  placeholder={childSending ? 'You can still type — Quill will hear you…' : 'Type your answer or ask for help…'}
                  value={childInput}
                  rows={1}
                  onChange={(e) => { setChildInput(e.target.value); autoGrowTextarea(e.target) }}
                  onPaste={onChildComposerPaste}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault()
                      sendChildText(childInput)
                    }
                  }}
                />
                <div className="fl-composer-menu">
                  {childQuickMenuOpen && <div className="fl-menu-backdrop" onClick={() => setChildQuickMenuOpen(false)} />}
                  <button type="button" className="composer-icon" aria-label="Quick requests" aria-expanded={childQuickMenuOpen} onClick={() => setChildQuickMenuOpen((v) => !v)}><Sparkles size={19} /></button>
                  {childQuickMenuOpen && (
                    <div className="fl-menu" role="menu">
                      {CHILD_QUICK_ACTIONS.map((qa) => (
                        <button key={qa.label} type="button" role="menuitem" onClick={() => { setChildQuickMenuOpen(false); sendChildText(qa.message) }}>{qa.label}</button>
                      ))}
                    </div>
                  )}
                </div>
                <button className="composer-send" type="submit" aria-label="Send message" disabled={!childInput.trim()}><Send size={18} /></button>
              </form>
            </section>
            <div
              className="fl-child-resizer"
              role="separator"
              aria-orientation="vertical"
              aria-label="Drag to resize the worksheet"
              aria-valuenow={Math.round(childSideWidth)}
              aria-valuemin={CHILD_SIDE_MIN}
              aria-valuemax={Math.round(childSideMax(windowWidth))}
              tabIndex={0}
              onPointerDown={startChildResize}
              // Keyboard equivalent — a drag handle that only works with a
              // pointer is unreachable for anyone tabbing, and this is the only
              // fine-grained control (the toolbar button is coarse: default vs max).
              onKeyDown={(e) => {
                const step = e.shiftKey ? 80 : 24
                if (e.key === 'ArrowLeft') { e.preventDefault(); commitChildSideWidth(childSideWidth + step) }
                else if (e.key === 'ArrowRight') { e.preventDefault(); commitChildSideWidth(childSideWidth - step) }
                else if (e.key === 'Home') { e.preventDefault(); commitChildSideWidth(CHILD_SIDE_DEFAULT) }
              }}
            >
              <span className="fl-child-resizer-grip" aria-hidden="true" />
            </div>
            <aside className="fl-child-side">
              <div className="fl-child-side-scroll">
              {childViewerPath ? (
                <div className="fl-viewer">
                  <div className="fl-viewer-bar">
                    <button className="fl-viewer-back" type="button" onClick={() => setChildViewerPath(null)}><ArrowLeft size={15} /> Back</button>
                    <span className="fl-viewer-name">{labelFromFilename(childViewerPath.split('/').pop() || childViewerPath).label}</span>
                    {/* Reading size. Cycles through the steps and wraps back to
                        normal, so one button covers both directions — simpler
                        than a +/- pair on a child's toolbar, and the current
                        step is shown on the button itself. */}
                    <button
                      className={`fl-icon-btn fl-zoom-btn${childZoom > 1 ? ' is-on' : ''}`}
                      type="button"
                      aria-label={`Text size: ${Math.round(childZoom * 100)}% — tap for bigger`}
                      title={childZoom > 1 ? `Text size ${Math.round(childZoom * 100)}% — tap for bigger` : 'Make the text bigger'}
                      onClick={cycleChildZoom}
                    >
                      <Type size={14} />
                      {childZoom > 1 && <span className="fl-zoom-badge">{Math.round(childZoom * 100)}%</span>}
                    </button>
                    {/* Widen/restore the worksheet, beside refresh and print.
                        Complements the drag divider between the panes: the
                        button is a coarse two-state jump (normal ⇄ as wide as
                        possible), the divider is for anything in between. */}
                    <button
                      className={`fl-icon-btn fl-widen-btn${childSideWide ? ' is-on' : ''}`}
                      type="button"
                      aria-label={childSideWide ? 'Shrink the worksheet' : 'Widen the worksheet'}
                      title={childSideWide ? 'Give the chat more room' : 'Give the worksheet more room'}
                      aria-pressed={childSideWide}
                      onClick={() => {
                        commitChildSideWidth(childSideWide ? CHILD_SIDE_DEFAULT : childSideMax(windowWidth))
                      }}
                    >
                      {childSideWide ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
                    </button>
                    <button
                      className="fl-icon-btn"
                      type="button"
                      aria-label="Refresh"
                      title="Reload this page"
                      onClick={() => setChildViewerRefreshKey((k) => k + 1)}
                    >
                      <RefreshCw size={14} />
                    </button>
                    {isPrintable(childViewerPath) && (
                      <button
                        className="fl-icon-btn"
                        type="button"
                        aria-label="Print"
                        title="Print this page"
                        onClick={() => printFile(childViewerPath)}
                      >
                        <Printer size={14} />
                      </button>
                    )}
                  </div>
                  <div key={childViewerPath} className="fl-viewer-body">
                  {IMAGE_PATH_RE.test(childViewerPath) ? (
                    <img className="fl-viewer-img" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(childViewerPath)}`} alt="" />
                  ) : /\.pdf$/i.test(childViewerPath) ? (
                    <iframe className="fl-viewer-frame" title="PDF preview" src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(childViewerPath)}`} />
                  ) : /\.(mp4|webm|mov|m4v)$/i.test(childViewerPath) ? (
                    <video className="fl-viewer-media" controls src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(childViewerPath)}`} />
                  ) : /\.(mp3|wav|m4a|aac|ogg|oga|flac|opus)$/i.test(childViewerPath) ? (
                    <audio className="fl-viewer-media" controls src={`${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(childViewerPath)}`} />
                  ) : !childViewerContent ? (
                    <p className="fl-note">Loading…</p>
                  ) : !childViewerContent.isText ? (
                    <NonPreviewableFile path={childViewerPath} meta={null} />
                  ) : (childViewerPath.endsWith('.html') || childViewerPath.endsWith('.htm')) ? (
                    <iframe
                      ref={childIframeRef}
                      className="fl-viewer-frame"
                      title="Preview"
                      sandbox="allow-scripts"
                      srcDoc={childViewerSrcDoc}
                    />
                  ) : childViewerPath.endsWith('.md') ? (
                    <div className="fl-viewer-md"><Markdown text={childViewerContent.content} /></div>
                  ) : (childViewerPath.endsWith('.json') || childViewerPath.endsWith('.jsonl')) ? (
                    <pre className="fl-viewer-pre">{formatJSONText(childViewerContent.content)}</pre>
                  ) : (
                    <pre className="fl-viewer-pre">{childViewerContent.content}</pre>
                  )}
                  </div>
                </div>
              ) : (
                <>
                  {(() => {
                    // Show ONLY the current activity (/api/child/activity) — the one the
                    // parent most recently handed off — plus the child's own saved work
                    // (its attempts/ folder). Not every activity ever created.
                    const currentItems = childActivity?.items ?? []
                    const attempts = childActivity?.attempts ?? []
                    if (!childActivity && attempts.length === 0) {
                      return <p className="fl-child-note"><Sparkles size={15} /> Ask Quill what to work on next!</p>
                    }
                    return (
                      <>
                        {currentItems.length > 0 ? (
                          <section className="fl-asset-group">
                            <p className="fl-drawer-label">From your parent</p>
                            <div className="fl-child-package">
                              <div className="fl-package-title"><BookOpen size={16} /><span>{childActivity?.title || 'Your activity'}<small>{currentItems.length} part{currentItems.length === 1 ? '' : 's'}{dateTimeLabel(childActivity?.created_at) ? ` · ${dateTimeLabel(childActivity?.created_at)}` : ''}</small></span></div>
                              {currentItems.map((item, i) => (
                                <button key={item.path} type="button" className="fl-file-item fl-package-item" onClick={() => { setChildViewerFocus(''); setChildViewerPath(item.path) }}>
                                  <span className="fl-package-step">{i + 1}</span>
                                  <FileGlyph name={item.name} size={15} />
                                  <span>{labelFromFilename(item.name).label}</span>
                                </button>
                              ))}
                            </div>
                          </section>
                        ) : childActivity ? (
                          // Instruction-only activity (no files): kick off the live activity in chat.
                          <section className="fl-asset-group">
                            <p className="fl-drawer-label">From your parent</p>
                            <button type="button" className="fl-file-item is-package" onClick={() => {
                              if (!window.matchMedia('(prefers-reduced-motion: reduce)').matches) setStartBurst(true)
                              setChildViewerPath(null)
                              sendChildText(`Let's start ${childActivity?.title || 'my activity'}!`)
                            }}>
                              <BookOpen size={16} /><span>{childActivity?.title || 'Your activity'}<small>Adaptive practice{dateTimeLabel(childActivity?.created_at) ? ` · ${dateTimeLabel(childActivity?.created_at)}` : ''}</small></span>
                            </button>
                          </section>
                        ) : null}
                        {attempts.length > 0 && (
                          <section className="fl-asset-group">
                            <p className="fl-drawer-label">Your work</p>
                            {attempts.map((item) => {
                              const { label, date } = labelFromFilename(item.name)
                              return (
                                <button key={item.path} type="button" className="fl-file-item" onClick={() => setChildViewerPath(item.path)}>
                                  <FileText size={16} /><span>{label}{date ? ` · ${date}` : ''}</span>
                                </button>
                              )
                            })}
                          </section>
                        )}
                      </>
                    )
                  })()}
                </>
              )}
              </div>
            </aside>
          </div>
          {pinGate && (
            <div className="fl-signoff-backdrop" role="dialog" aria-modal="true" aria-labelledby="fl-gate-title">
              <div className="fl-signoff-card">
                <span className="fl-signoff-icon"><LockKeyhole size={22} /></span>
                <h2 id="fl-gate-title">Enter parent PIN</h2>
                <p>Parent Mode is protected. Enter your PIN to return.</p>
                <input
                  className="fl-gate-input"
                  type="password"
                  inputMode="numeric"
                  autoFocus
                  value={gateValue}
                  onChange={(e) => setGateValue(e.target.value.replace(/\D/g, '').slice(0, 8))}
                  onKeyDown={(e) => { if (e.key === 'Enter') submitPinGate() }}
                  placeholder="PIN"
                />
                {gateError && <p className="pin-error"><LockKeyhole size={16} /> {gateError}</p>}
                <div className="fl-signoff-actions">
                  <button className="fl-ghost-btn" type="button" onClick={() => { setPinGate(false); setGateValue('') }}>Cancel</button>
                  <button className="primary-button" type="button" onClick={submitPinGate} disabled={!gateValue}>Unlock <ArrowRight size={18} /></button>
                </div>
              </div>
            </div>
          )}
        </div>
      </main>
    )
  }

  return (
    <main className="learning-app" data-theme={theme}>
      <header className="learning-header">
        <div className="learning-brand">
          <img className="brand-mark" src="/sparkquill-mark.svg" alt="" width={30} height={30} />
          <span className="brand-word">Spark<strong>Quill</strong></span>
        </div>
        <div className="setup-progress" aria-label={`Setup step ${screen === 'engine' ? '1' : screen === 'child' ? '2' : '3'} of 3`}>
          <span className="setup-step-name">{screen === 'engine' ? 'Learning helper' : screen === 'child' ? 'Your child' : 'Parent PIN'}</span>
          <span className="setup-step-count">{screen === 'engine' ? '1' : screen === 'child' ? '2' : '3'} of 3</span>
          <span className="setup-progress-track" aria-hidden="true">
            <i className="is-complete" />
            <i className={screen === 'child' || screen === 'pin' ? 'is-complete' : ''} />
            <i className={screen === 'pin' ? 'is-complete' : ''} />
          </span>
        </div>
      </header>

      <section className={`learning-stage is-${screen}`}>
        {screen === 'engine' && (
          <section className="learning-panel setup-panel">
            <span className="eyebrow">01 · Choose your learning helper</span>
            <h1>Pick the AI that will help your child learn.</h1>
            <p className="lead">It runs on this computer and powers every lesson, hint, and practice session.</p>

            {enginesState === 'loading' && (
              <p className="engine-note">Checking which AI teachers are installed on this computer…</p>
            )}
            {enginesState === 'error' && (
              <p className="engine-note is-error">Couldn’t reach the learning service at {FAMILY_API}. Make sure it’s running, then <button type="button" className="linklike" onClick={() => window.location.reload()}>try again</button>.</p>
            )}

            {enginesState === 'ready' && (
              <div className="engine-grid">
                {engines.map((item) => {
                  const status = engineStatus(item)
                  return (
                    <button
                      key={item.id}
                      type="button"
                      className={`engine-card ${engine === item.id ? 'is-selected' : ''} ${status.ready ? '' : 'is-unavailable'}`}
                      onClick={() => { setEngine(item.id); setTestState('idle'); setTestMessage('') }}
                    >
                      <span className="engine-icon"><Sparkles size={24} /></span>
                      <span className="engine-content">
                        <strong>{pres(item.id, item.name).name} {pres(item.id, item.name).preferred && <em className="preferred-badge">Recommended</em>}</strong>
                        <small>{pres(item.id, item.name).blurb}</small>
                      </span>
                      <span className={`engine-status ${status.ready ? 'is-ready' : ''}`}>{status.label}</span>
                    </button>
                  )
                })}
              </div>
            )}

            <div className="setup-footer">
              <p>
                {selectedEngine
                  ? (engineStatus(selectedEngine).ready
                      ? <><CheckCircle2 size={18} /> {pres(selectedEngine.id, selectedEngine.name).name} is ready.</>
                      : <><LockKeyhole size={18} /> {pres(selectedEngine.id, selectedEngine.name).name}: {engineStatus(selectedEngine).label.toLowerCase()}.</>)
                  : <>Select a learning helper to continue.</>}
                {selectedEngine && engineStatus(selectedEngine).ready && (
                  <button type="button" className="linklike" onClick={runTest} disabled={testState === 'testing'}>
                    {testState === 'testing' ? 'Testing…' : testState === 'valid' ? 'Test passed ✓' : testState === 'invalid' ? 'Test failed — retry' : 'Test connection'}
                  </button>
                )}
              </p>
              <button className="primary-button" onClick={persistEngineAndContinue} type="button" disabled={!selectedEngine || !engineStatus(selectedEngine).ready || saving}>Continue <ArrowRight size={18} /></button>
            </div>
            {testMessage && <p className={`engine-note ${testState === 'invalid' ? 'is-error' : ''}`}>{testMessage}</p>}
            {selectedEngine && !engineStatus(selectedEngine).ready && selectedEngine.setup_hint && (
              <details className="engine-setup"><summary>Setup details</summary><p>{selectedEngine.setup_hint}</p></details>
            )}
          </section>
        )}

        {screen === 'child' && (
          <section className="learning-panel setup-panel">
            <span className="eyebrow">02 · Add your child</span>
            <h1>Create one calm learning space.</h1>
            <p className="lead">Tell the learning guide just enough to make each session feel personal.</p>
            <div className="child-form-card">
              <label>
                <span>Name or nickname</span>
                <input value={childName} onChange={(event) => setChildName(event.target.value)} />
              </label>
              <div className="form-row">
                <label>
                  <span>Grade</span>
                  <select value={grade} onChange={(event) => setGrade(event.target.value)}>
                    {GRADES.map((g) => <option key={g} value={g}>Grade {g}</option>)}
                  </select>
                </label>
                <label>
                  <span>School board</span>
                  <select value={board} onChange={(event) => setBoard(event.target.value)}>
                    {BOARDS.map((b) => <option key={b} value={b}>{b}</option>)}
                  </select>
                </label>
              </div>
              <div className="profile-preview">
                <span className="avatar-preview">{initial}</span>
                <span><strong>{childName || 'Your child'}</strong><small>Grade {grade} · {board} · English</small></span>
              </div>
            </div>
            <div className="setup-footer">
              <p><LockKeyhole size={18} /> Next, set a parent PIN.</p>
              <div className="setup-actions">
                <button className="setup-back" onClick={() => move('engine')} type="button"><ArrowLeft size={16} /> Back</button>
                <button className="primary-button" onClick={createChildAndContinue} type="button" disabled={!childName.trim() || saving}>Continue <ArrowRight size={18} /></button>
              </div>
            </div>
          </section>
        )}

        {screen === 'pin' && (
          <section className="learning-panel setup-panel">
            <span className="eyebrow">03 · Set a parent PIN</span>
            <h1>Create your parent PIN.</h1>
            <p className="lead">This keeps Parent Mode — your notes, answer keys, grading, and settings — separate from {childName || 'your child'}’s space on this shared computer.</p>
            <div className="child-form-card">
              <div className="form-row">
                <label>
                  <span>Parent PIN</span>
                  <input type="password" inputMode="numeric" value={pin} onChange={(event) => setPin(event.target.value.replace(/\D/g, '').slice(0, 8))} placeholder="4–8 digits" />
                </label>
                <label>
                  <span>Confirm PIN</span>
                  <input type="password" inputMode="numeric" value={pinConfirm} onChange={(event) => setPinConfirm(event.target.value.replace(/\D/g, '').slice(0, 8))} placeholder="Re-enter" />
                </label>
              </div>
              <p className="pin-hint">You’ll enter this to return to Parent Mode after handing the computer to {childName || 'your child'}.</p>
            </div>
            <div className="setup-footer">
              <p>{pinError ? <span className="pin-error"><LockKeyhole size={18} /> {pinError}</span> : <><LockKeyhole size={18} /> Only you should know this PIN.</>}</p>
              <div className="setup-actions">
                <button className="setup-back" onClick={() => move('child')} type="button"><ArrowLeft size={16} /> Back</button>
                <button className="primary-button" onClick={savePinAndContinue} type="button" disabled={saving}>Enter SparkQuill <ArrowRight size={18} /></button>
              </div>
            </div>
          </section>
        )}
      </section>
      {startBurst && <StartBurst onDone={() => setStartBurst(false)} />}
    </main>
  )
}
