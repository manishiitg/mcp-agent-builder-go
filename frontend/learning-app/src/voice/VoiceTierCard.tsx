import { Download, Trash2, Loader2 } from 'lucide-react'
import { MicTestButton } from './MicTestButton'
import type { VoiceTier } from '../stores'

function sizeLabel(mb?: number): string {
  if (!mb) return 'no download'
  return mb >= 1000 ? `${(mb / 1000).toFixed(1)}GB download` : `${mb}MB download`
}

/**
 * One speech-to-text option.
 *
 * Deliberately shows the REAL tradeoff rather than just a name — size,
 * language coverage, and why an option is unavailable on this specific Mac —
 * since "which is better" genuinely depends on the family (an English-only
 * tier is faster but useless to a household that also speaks Hindi).
 *
 * A download of several hundred MB with no visible progress reads as a hang,
 * so an in-flight install shows a real percentage rather than a spinner.
 */
export function VoiceTierCard({
  tier,
  onInstall,
  onRemove,
  busy,
  testable,
}: {
  tier: VoiceTier
  onInstall?: (id: string) => void
  onRemove?: (id: string) => void
  busy?: boolean
  /** Speech tiers preview by recording you and showing what they heard. */
  testable?: boolean
}) {
  const pct = tier.total_bytes
    ? Math.min(100, Math.round(((tier.got_bytes ?? 0) / tier.total_bytes) * 100))
    : 0

  return (
    <div className={`fl-voice-tier${tier.installed ? ' is-installed' : ''}${!tier.available ? ' is-unavailable' : ''}`}>
      <div className="fl-voice-tier-head">
        <span className="fl-voice-tier-name">{tier.label}</span>
        {tier.installed && (
          tier.warm === false
            // Installed but the model isn't loaded in memory yet (first use
            // this session, or it unloaded itself after being idle) — the
            // NEXT use will take a few real seconds rather than being
            // instant, so this says so instead of over-claiming "ready".
            ? <span className="fl-voice-tier-badge is-warming">Warming up…</span>
            : <span className="fl-voice-tier-badge is-on">Installed</span>
        )}
        {tier.coming_soon && tier.available && !tier.installed && <span className="fl-voice-tier-badge">Coming soon</span>}
        {!tier.available && <span className="fl-voice-tier-badge is-off">Unavailable</span>}

        <span className="fl-voice-tier-actions">
          {tier.installing ? (
            <span className="fl-voice-tier-progress"><Loader2 size={12} className="fl-mic-spin" /> {pct}%</span>
          ) : (
            <>
              {tier.can_install && onInstall && (
                <button className="fl-ghost-btn is-tiny" type="button" disabled={busy} onClick={() => onInstall(tier.id)}>
                  <Download size={12} /> Install
                </button>
              )}
              {tier.can_remove && onRemove && (
                <button className="fl-ghost-btn is-tiny is-danger" type="button" disabled={busy} onClick={() => onRemove(tier.id)} title="Delete this and free up the space">
                  <Trash2 size={12} /> Remove
                </button>
              )}
            </>
          )}
        </span>
      </div>

      <p className="fl-voice-tier-desc">{tier.description}</p>
      <p className="fl-voice-tier-meta">{tier.languages} · {sizeLabel(tier.size_mb)}</p>

      {tier.installing && (
        <div className="fl-voice-progress-track" aria-label={`Downloading, ${pct}%`}>
          <span className="fl-voice-progress-fill" style={{ width: `${pct}%` }} />
        </div>
      )}
      {testable && tier.installed && !tier.installing && <MicTestButton />}
      {tier.install_error && <p className="fl-voice-tier-error">{tier.install_error}</p>}
      {!tier.available && tier.unavailable_reason && (
        <p className="fl-voice-tier-why">{tier.unavailable_reason}</p>
      )}
    </div>
  )
}
