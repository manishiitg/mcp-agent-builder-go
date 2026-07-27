import { useState, useEffect } from 'react'
import { Download, Trash2, Loader2, Volume2 } from 'lucide-react'
import { speakText } from './speech'
import { MicTestButton } from './MicTestButton'
import type { VoiceTier, VoiceChoice } from '../stores'
import { FAMILY_API } from '../apiBase'

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
  testable,
}: {
  tier: VoiceTier
  onInstall?: (id: string) => void
  onRemove?: (id: string) => void
  busy?: boolean
  /** Read-aloud tiers preview with a play button. */
  sampleable?: boolean
  /** Speech tiers preview by recording you and showing what they heard. */
  testable?: boolean
}) {
  const [sampling, setSampling] = useState(false)
  // Voice options for this tier, loaded only when it's actually usable —
  // there's nothing to choose between for a tier that isn't installed.
  const [voices, setVoices] = useState<VoiceChoice[]>([])
  const [voice, setVoice] = useState('')
  useEffect(() => {
    if (!sampleable || !tier.installed) return
    let cancelled = false
    fetch(`${FAMILY_API}/api/voice/voices?tier=${encodeURIComponent(tier.id)}`)
      .then((r) => r.json())
      .then((d: { voices?: VoiceChoice[]; selected?: string }) => {
        if (cancelled) return
        setVoices(d.voices ?? [])
        setVoice(d.selected ?? '')
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [sampleable, tier.installed, tier.id])

  const chooseVoice = (id: string) => {
    setVoice(id)
    fetch(`${FAMILY_API}/api/voice/voices`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tier: tier.id, voice: id }),
    }).catch(() => {})
  }
  const playSample = () => {
    setSampling(true)
    speakText("Hi! Here's how I'll read things out to you. The Prime Meridian passes through Greenwich, in England, and it's the line we measure longitude from. Everything to the east of it is ahead in time, and everything to the west is behind.", tier.id, voice)
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
      {voices.length > 1 && (
        <label className="fl-voice-pick">
          <span>Voice</span>
          <select value={voice} onChange={(e) => chooseVoice(e.target.value)}>
            {voices.map((v) => (
              <option key={v.id} value={v.id}>{v.accent ? `${v.label} · ${v.accent}` : v.label}</option>
            ))}
          </select>
        </label>
      )}
      {testable && tier.installed && !tier.installing && <MicTestButton tier={tier.id} />}
      {tier.install_error && <p className="fl-voice-tier-error">{tier.install_error}</p>}
      {!tier.available && tier.unavailable_reason && (
        <p className="fl-voice-tier-why">{tier.unavailable_reason}</p>
      )}
    </div>
  )
}
