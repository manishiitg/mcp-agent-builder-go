// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { isWorkspaceRelativeReference, rewriteReportMarkdownReferences } from './reportMarkdownLinks'

const allow = (path: string) => (path.startsWith('db/') || path.startsWith('knowledgebase/') ? path : '')

describe('isWorkspaceRelativeReference', () => {
  it('is true only for a bare relative path', () => {
    expect(isWorkspaceRelativeReference('db/assets/x.png')).toBe(true)
    expect(isWorkspaceRelativeReference('./notes.md')).toBe(true)
    expect(isWorkspaceRelativeReference('https://x.test/a')).toBe(false)
    expect(isWorkspaceRelativeReference('mailto:a@b.c')).toBe(false)
    expect(isWorkspaceRelativeReference('data:image/png;base64,AAA')).toBe(false)
    expect(isWorkspaceRelativeReference('#section')).toBe(false)
    expect(isWorkspaceRelativeReference('/absolute')).toBe(false)
    expect(isWorkspaceRelativeReference('')).toBe(false)
  })
})

describe('rewriteReportMarkdownReferences', () => {
  it('routes workspace links through the preview and images through the blob channel', () => {
    const html = '<p><a href="db/reports/proof.pdf">proof</a> <img src="db/assets/chart.png" alt="c"></p>'
    const out = rewriteReportMarkdownReferences(html, allow)
    expect(out).toContain('data-report-open="db/reports/proof.pdf"')
    expect(out).toContain('href="#"')
    expect(out).toContain('data-report-src="db/assets/chart.png"')
    expect(out).not.toContain('src="db/assets/chart.png"')
  })

  it('leaves external links, anchors and data URIs alone', () => {
    const html = '<a href="https://x.test">x</a><a href="#top">top</a><img src="data:image/png;base64,AAA">'
    expect(rewriteReportMarkdownReferences(html, allow)).toBe(html)
  })

  it('leaves a reference the allow-list rejects untouched', () => {
    const html = '<a href="runs/iteration-1/out.txt">scratch</a>'
    expect(rewriteReportMarkdownReferences(html, allow)).toBe(html)
  })

  it('resolves file-relative references against the markdown file folder, root-relative ones as-is', () => {
    const html = '<img src="./chart.png"><img src="assets/b.png"><a href="db/notes/other.md">o</a>'
    const out = rewriteReportMarkdownReferences(html, allow, 'db/notes')
    expect(out).toContain('data-report-src="db/notes/chart.png"')
    expect(out).toContain('data-report-src="db/notes/assets/b.png"')
    expect(out).toContain('data-report-open="db/notes/other.md"')
  })

  it('drops a query string or fragment from the resolved path', () => {
    const out = rewriteReportMarkdownReferences('<a href="db/notes/a.md#h2?x=1">a</a>', allow)
    expect(out).toContain('data-report-open="db/notes/a.md"')
  })
})
