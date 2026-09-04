// Markdown rendered inside a report (window.report.getHtml / renderMarkdown)
// lands in a srcdoc iframe whose base URL is about:srcdoc, so a relative
// workspace link (`[proof](db/reports/proof.pdf)`) or image
// (`![chart](db/assets/chart.png)`) resolves to nothing: the link navigates
// nowhere and the image is a broken icon. Rewrite them to the report's own
// file channel instead: links open in the in-report preview modal
// (window.report.openFile) and images load through an authenticated blob URL
// (window.report.fileUrl). HtmlReportFrame handles both markers at runtime.
//
// Absolute URLs, mailto:, and in-page #anchors are left alone; the frame
// already opens external links in a new tab and scrolls anchors manually.
//
// Regex-based rather than DOMParser: the input is always renderToStaticMarkup
// output from ReactMarkdown + remarkGfm, a small, predictable, well-formed
// shape (double-quoted attributes, no raw `>` inside them), so a tag-level
// rewrite is reliable without pulling in a DOM implementation -- this module
// also runs in the headless preview build and in tests, neither of which
// carries a browser DOM.

export const REPORT_OPEN_ATTR = 'data-report-open'
export const REPORT_SRC_ATTR = 'data-report-src'

export function isWorkspaceRelativeReference(ref: string): boolean {
  const value = ref.trim()
  if (!value) return false
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) return false // http:, https:, mailto:, data:, blob:, …
  if (value.startsWith('#') || value.startsWith('/') || value.startsWith('//')) return false
  return true
}

function decodeHTMLAttrValue(value: string): string {
  return value
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#0*39;|&apos;/g, "'")
}

function encodeHTMLAttrValue(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

const ANCHOR_TAG = /<a\b([^>]*)\bhref="([^"]*)"([^>]*)>/gi
const IMG_TAG = /<img\b([^>]*)\bsrc="([^"]*)"([^>]*)>/gi

/**
 * Rewrite relative links and images in an HTML fragment so they work inside
 * the report frame. `allow` decides which workspace paths are reachable (the
 * same rule window.report.getText applies); references it rejects are left
 * untouched rather than silently broken differently.
 */
export function rewriteReportMarkdownReferences(
  html: string,
  allow: (path: string) => string,
  basePath = '',
): string {
  if (!html) return html

  // A reference is either workspace-root-relative (`db/assets/x.png`, the
  // documented convention) or relative to the markdown file itself
  // (`./x.png`, `assets/x.png`) when the file's own folder is known.
  const resolve = (ref: string): string => {
    const value = decodeHTMLAttrValue(ref).trim().split('#')[0].split('?')[0].replace(/^(\.\/)+/, '')
    const rootRelative = /^(db|knowledgebase|docs|planning|evaluation|costs|variables)\//.test(value)
    const joined = basePath && !rootRelative ? `${basePath.replace(/\/+$/, '')}/${value}` : value
    return allow(joined)
  }

  let out = html.replace(ANCHOR_TAG, (full, before: string, href: string, after: string) => {
    if (!isWorkspaceRelativeReference(href)) return full
    const allowed = resolve(href)
    if (!allowed) return full
    return `<a${before}href="#"${after} ${REPORT_OPEN_ATTR}="${encodeHTMLAttrValue(allowed)}">`
  })

  out = out.replace(IMG_TAG, (full, before: string, src: string, after: string) => {
    if (!isWorkspaceRelativeReference(src)) return full
    const allowed = resolve(src)
    if (!allowed) return full
    // React's server renderer emits void elements self-closed (`<img .../>`);
    // `after` can carry that trailing slash. Move it back to just before the
    // final `>` rather than leaving it stranded ahead of the new attribute.
    const selfClosing = /\/\s*$/.test(after)
    const attrs = after.replace(/\/\s*$/, '')
    return `<img${before}${attrs} ${REPORT_SRC_ATTR}="${encodeHTMLAttrValue(allowed)}"${selfClosing ? ' /' : ''}>`
  })

  return out
}
