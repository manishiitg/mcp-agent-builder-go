import { useState } from 'react'
import { Download, Trash2, Loader2, Volume2 } from 'lucide-react'
import { speakText } from './speech'
import type { VoiceTier } from '../stores'

function sizeLabel(mb?: number): string {
  if (!mb) return 'no download'
  return mb >= 1000 ? `${(mb / 1000).toFixed(1)}GB download` : `${mb}MB download`
}

/**
 * One STT/TTS option.
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
  sampleable,
}: {
  tier: VoiceTier
  onInstall?: (id: string) => void
  onRemove?: (id: string) => void
  busy?: boolean
  /** Read-aloud tiers can be previewed; speech-to-text tiers can't. */
  sampleable?: boolean
}) {
  const [sampling, setSampling] = useState(false)
  const playSample = () => {
    setSampling(true)
    speakText("Hi! This is how I'll read things out to you.", tier.id)
      .then(() => setSampling(false))
      .catch(() => setSampling(false))
  }
  const pct = tier.total_bytes
    ? Math.min(100, Math.round(((tier.got_bytes ?? 0) / tier.total_bytes) * 100))
    : 0

  return (
    <div className={`fl-voice-tier${tier.installed ? ' is-installed' : ''}${!tier.available ? ' is-unavailable' : ''}`}>
      <div className="fl-voice-tier-head">
        <span className="fl-voice-tier-name">{tier.label}</span>
        {tier.installed && <span className="fl-voice-tier-badge is-on">Installed</span>}
        {tier.coming_soon && tier.available && !tier.installed && <span className="fl-voice-tier-badge">Coming soon</span>}
        {!tier.available && <span className="fl-voice-tier-badge is-off">Unavailable</span>}

        <span className="fl-voice-tier-actions">
          {tier.installing ? (
            <span className="fl-voice-tier-progress"><Loader2 size={12} className="fl-mic-spin" /> {pct}%</span>
          ) : (
            <>
              {sampleable && tier.installed && (
                <button className="fl-ghost-btn is-tiny" type="button" disabled={sampling} onClick={playSample} title="Hear what this voice sounds like">
                  <Volume2 size={12} /> {sampling ? 'Playing…' : 'Hear it'}
                </button>
              )}
              {tier.can_install && onInstall && (
                <button className="fl-ghost-btn is-tiny" type="button" disabled={busy} onClick={() => onInstall(tier.id)}>
                  <Download size={12} /> Install
                </button>
              )}
              {tier.can_remove && onRemove && (
                <button className="fl-ghost-btn is-tiny" type="button" disabled={busy} onClick={() => onRemove(tier.id)} title="Delete this model and reclaim the space">
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
      {tier.install_error && <p className="fl-voice-tier-error">{tier.install_error}</p>}
      {!tier.available && tier.unavailable_reason && (
        <p className="fl-voice-tier-why">{tier.unavailable_reason}</p>
      )}
    </div>
  )
}
