import { describe, it, expect } from 'vitest'
import { withReportBootstrap } from './HtmlWidgetFrame'

describe('withReportBootstrap', () => {
  it('injects the stub immediately after <head> so it precedes any author script', () => {
    const html = '<html><head><title>t</title></head><body><script>go()</script></body></html>'
    const out = withReportBootstrap(html)
    expect(out.indexOf('__reportQueuedCallbacks')).toBeLessThan(out.indexOf('<title>'))
  })

  it('falls back to after <html> when there is no <head>', () => {
    const out = withReportBootstrap('<html><body>x</body></html>')
    expect(out.indexOf('__reportQueuedCallbacks')).toBeLessThan(out.indexOf('<body>'))
  })

  it('handles a bare fragment with no document shell', () => {
    expect(withReportBootstrap('<div>x</div>')).toContain('__reportQueuedCallbacks')
  })

  it('is idempotent — re-wrapping does not inject a second stub', () => {
    const once = withReportBootstrap('<html><head></head><body></body></html>')
    const twice = withReportBootstrap(once)
    expect(twice).toBe(once)
  })

  it('queues callbacks registered before the API exists, and runs them on flush', () => {
    const win: Record<string, unknown> = {}
    // Execute the real stub source against a bare window, as the iframe would.
    const stub = withReportBootstrap('<div></div>').match(/<script>([\s\S]*?)<\/script>/)![1]
    new Function('window', stub)(win)

    const calls: string[] = []
    ;(win.report as { ready: (f: () => void) => void }).ready(() => calls.push('render'))

    // Nothing runs while the API is not yet live.
    expect(calls).toEqual([])
    expect(win.__reportQueuedCallbacks).toHaveLength(1)

    // Host flush (what inject() does).
    ;(win.__reportQueuedCallbacks as Array<() => void>).forEach((fn) => fn())
    expect(calls).toEqual(['render'])
  })
})
