import { describe, expect, it } from 'vitest'
import { applyRuntimeBranding } from './runtime-branding'

function fakeDocument() {
  const favicon = { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }
  return {
    favicon,
    document: {
      title: 'AgentWorks',
      head: { appendChild: () => undefined },
      querySelector: () => favicon,
      createElement: () => favicon,
    } as unknown as Document,
  }
}

describe('runtime branding', () => {
  it('applies an isolated app name and same-origin favicon', () => {
    const target = fakeDocument()
    applyRuntimeBranding({
      appName: 'Video Studio (Dev)',
      faviconUrl: '/video-studio-favicon.svg',
    }, target.document)

    expect(target.document.title).toBe('Video Studio (Dev)')
    expect(target.favicon.href).toBe('/video-studio-favicon.svg')
    expect(target.favicon.type).toBe('image/svg+xml')
  })

  it('rejects an external favicon URL', () => {
    const target = fakeDocument()
    applyRuntimeBranding({ faviconUrl: 'https://example.com/tracker.svg' }, target.document)
    expect(target.favicon.href).toBe('/logo.svg')
  })

  it('rejects control characters in runtime branding values', () => {
    const target = fakeDocument()
    applyRuntimeBranding({
      appName: 'Video Studio\nInjected',
      faviconUrl: '/video-studio-favicon.svg\u0000.png',
    }, target.document)

    expect(target.document.title).toBe('AgentWorks')
    expect(target.favicon.href).toBe('/logo.svg')
  })
})
