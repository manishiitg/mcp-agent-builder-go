// The Settings → Voice panel: STT/TTS tier catalogs, the auto-read-aloud
// toggle, and a "hear a sample" preview.
//
// Tier availability is computed SERVER-side against this actual machine (see
// cmd/family-server/voice_hardware.go), so this component never has to guess
// what an Intel vs Apple Silicon Mac can run — it just renders what it's told.

import { useState } from 'react'
import { Volume2 } from 'lucide-react'
import type { VoiceStatus, VoiceTier } from '../stores'
import { speakText } from './speech'

/**
 * One STT/TTS option. Deliberately shows the REAL tradeoff rather than just a
 * name — size, language coverage, and why an option is unavailable on this
 * specific Mac — since "which is better" genuinely depends on the family (an
 * English-only tier is faster but useless to a household that also speaks
 * Hindi).
 *
 * Not a button: tiers marked coming_soon have no install path wired up yet, so
 * this presents the tradeoff honestly rather than offering a click that would
 * do nothing.
 */
export function VoiceTierCard({ tier }: { tier: VoiceTier }) {
  const usable = tier.available && !tier.coming_soon
  return (
    <div className={`fl-voice-tier${tier.installed ? ' is-installed' : ''}${!tier.available ? ' is-unavailable' : ''}`}>
      <div className="fl-voice-tier-head">
        <span className="fl-voice-tier-name">{tier.label}</span>
        {tier.installed && <span className="fl-voice-tier-badge is-on">Installed</span>}
        {tier.coming_soon && tier.available && <span className="fl-voice-tier-badge">Coming soon</span>}
        {!tier.available && <span className="fl-voice-tier-badge is-off">Unavailable</span>}
      </div>
      <p className="fl-voice-tier-desc">{tier.description}</p>
      <p className="fl-voice-tier-meta">
        {tier.languages}
        {tier.size_mb
          ? <> · {tier.size_mb >= 1000 ? `${(tier.size_mb / 1000).toFixed(1)}GB` : `${tier.size_mb}MB`} download</>
          : <> · no download</>}
      </p>
      {!tier.available && tier.unavailable_reason && (
        <p className="fl-voice-tier-why">{tier.unavailable_reason}</p>
      )}
      {usable && !tier.installed && (
        <p className="fl-voice-tier-why">Downloads when you turn it on, and is deleted again if you turn it off.</p>
      )}
    </div>
  )
}

export function VoiceSettings({
  status,
  childName,
}: {
  status: VoiceStatus | null
  childName: string
}) {
  const [previewing, setPreviewing] = useState(false)

  // Play a short sample, so a parent can actually HEAR a voice before
  // committing to it — the whole point of offering tiers is that "more
  // natural" is a judgment only they can make.
  const preview = () => {
    setPreviewing(true)
    speakText(`Hi ${childName || 'there'}! This is how I'll read things out to you.`)
      .then(() => setPreviewing(false))
      .catch(() => setPreviewing(false))
  }

  return (
    <>
      <p className="fl-drawer-label" style={{ marginTop: '20px' }}>Voice</p>
      <p className="fl-note">
        Talking to Quill instead of typing, and having replies read aloud. Everything runs on this Mac —
        nothing is sent to a cloud service, so it works offline and costs nothing per use.
        {status && (
          <> Detected: <strong>{status.hardware.is_apple_silicon ? 'Apple Silicon' : 'Intel'}</strong>
            {status.hardware.total_ram_bytes > 0 && (
              <> · {Math.round(status.hardware.total_ram_bytes / 1024 / 1024 / 1024)}GB RAM</>
            )}.</>
        )}
      </p>

      {!status ? (
        <p className="fl-note">Checking what this Mac can run…</p>
      ) : (
        <>
          <p className="fl-voice-group-label">Speech to text — talking instead of typing</p>
          <p className="fl-note">Used both here (the mic in the message box) and for WhatsApp voice notes.</p>
          <div className="fl-settings-engines">
            {status.stt_tiers.map((t) => <VoiceTierCard key={t.id} tier={t} />)}
          </div>

          <p className="fl-voice-group-label">Read aloud — hearing Quill's replies</p>
          <div className="fl-settings-engines">
            {status.tts_tiers.map((t) => <VoiceTierCard key={t.id} tier={t} />)}
          </div>

          <button className="fl-ghost-btn" type="button" style={{ marginTop: '8px' }} onClick={preview} disabled={previewing}>
            <Volume2 size={14} /> {previewing ? 'Playing…' : 'Hear a sample'}
          </button>
        </>
      )}
    </>
  )
}
