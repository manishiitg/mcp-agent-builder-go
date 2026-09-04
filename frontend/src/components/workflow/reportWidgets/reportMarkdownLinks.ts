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

export const REPORT_OPEN_ATTR = 'data-report-open'
export const REPORT_SRC_ATTR = 'data-report-src'

export function isWorkspaceRelativeReference(ref: string): boolean {
  const value = ref.trim()
  if (!value) return false
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) return false // http:, https:, mailto:, data:, blob:, …
  if (value.startsWith('#') || value.startsWith('/') || value.startsWith('//')) return false
  return true
}

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
  if (!html || typeof DOMParser === 'undefined') return html
  const doc = new DOMParser().parseFromString(`<div id="__root">${html}</div>`, 'text/html')
  const root = doc.getElementById('__root')
  if (!root) return html

  // A reference is either workspace-root-relative (`db/assets/x.png`, the
  // documented convention) or relative to the markdown file itself
  // (`./x.png`, `assets/x.png`) when the file's own folder is known.
  const resolve = (ref: string): string => {
    const value = ref.trim().split('#')[0].split('?')[0].replace(/^(\.\/)+/, '')
    const rootRelative = /^(db|knowledgebase|docs|planning|evaluation|costs|variables)\//.test(value)
    const joined = basePath && !rootRelative ? `${basePath.replace(/\/+$/, '')}/${value}` : value
    return allow(joined)
  }

  root.querySelectorAll('a[href]').forEach((anchor) => {
    const href = anchor.getAttribute('href') || ''
    if (!isWorkspaceRelativeReference(href)) return
    const allowed = resolve(href)
    if (!allowed) return
    anchor.setAttribute(REPORT_OPEN_ATTR, allowed)
    anchor.setAttribute('href', '#')
  })

  root.querySelectorAll('img[src]').forEach((img) => {
    const src = img.getAttribute('src') || ''
    if (!isWorkspaceRelativeReference(src)) return
    const allowed = resolve(src)
    if (!allowed) return
    img.setAttribute(REPORT_SRC_ATTR, allowed)
    img.removeAttribute('src')
  })

  return root.innerHTML
}
