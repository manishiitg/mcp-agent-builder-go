import { describe, expect, it } from 'vitest'
import { buildPulseTimelineHtml } from './pulseTimelineHtml'

describe('buildPulseTimelineHtml', () => {
  it('installs a reusable module renderer without baking in one module', () => {
    const result = buildPulseTimelineHtml('<html><body><div class="wrap"></div></body></html>')

    expect(result).toContain('window.__runloopRenderPulseModule = render')
    expect(result).toContain("querySelectorAll('.pulse-record,.run,.entry')")
    expect(result).toContain('removeVisibleRuntimeIds(clone)')
    expect(result).toContain('pulse_run_id|review_run_id|session_id|execution_id|run_id')
    expect(result).not.toContain("return 'bug_review'")
  })

  it('preserves the report and appends the filter before body close', () => {
    const result = buildPulseTimelineHtml('<html><body><div id="original">Pulse</div></body></html>')

    expect(result).toContain('<div id="original">Pulse</div>')
    expect(result.indexOf('__runloop_pulse_section_script')).toBeLessThan(result.indexOf('</body>'))
  })

  it('routes historical cards into their current gated lanes', () => {
    const result = buildPulseTimelineHtml('<html><body><div class="wrap"></div></body></html>')

		expect(result).toContain("['bug_review','artifact_review','report_health','eval_health','stores_health','learning_health','knowledgebase_health','db_health']")
    expect(result).toContain("['llm_ops_review','cost_llm_time']")
    expect(result).toContain("value.indexOf('cost')")
    expect(result).toContain("return 'llm_ops_review'")
  })
})
