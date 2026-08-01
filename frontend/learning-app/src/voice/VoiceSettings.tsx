// The Settings → Voice panel: the speech-to-text tier catalog.
//
// Tier availability is computed SERVER-side against this actual machine (see
// cmd/family-server/voice_hardware.go), so this component never has to guess
// what an Intel vs Apple Silicon Mac can run — it just renders what it's told.

import { useState } from 'react'
import type { VoiceStatus } from '../stores'
import { VoiceTierCard } from './VoiceTierCard'
import { FAMILY_API } from '../apiBase'

export function VoiceSettings({
  status,
  onRefresh,
}: {
  status: VoiceStatus | null
  childName: string
  /** Re-fetch /api/voice/status — the parent owns the polling that keeps
   *  download progress live while an install is running. */
  onRefresh: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const modelAction = (path: string, id: string) => {
    setBusy(true)
    setActionError(null)
    fetch(`${FAMILY_API}/api/voice/model/${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id }),
    })
      .then(async (r) => {
        const data = await r.json().catch(() => ({}))
        // A refused removal (e.g. "this is the only model") is a real,
        // actionable message — show it rather than failing silently.
        if (!r.ok) throw new Error(data?.error || `Failed (${r.status})`)
      })
      .catch((err) => setActionError(err instanceof Error ? err.message : 'Something went wrong'))
      .finally(() => { setBusy(false); onRefresh() })
  }

  return (
    <>
      <p className="fl-drawer-label" style={{ marginTop: '20px' }}>Voice</p>
      <p className="fl-note">
        Talk to Quill instead of typing. This happens on this computer — nothing is sent over the
        internet, so it keeps working offline and costs nothing.
      </p>

      {!status ? (
        <p className="fl-note">Checking what this computer can do…</p>
      ) : (
        <>
          <p className="fl-voice-group-label">Talking instead of typing</p>
          <p className="fl-note">Used by the microphone in the message box, and for voice notes on WhatsApp.</p>
          <div className="fl-settings-engines">
            {status.stt_tiers.map((t) => (
              <VoiceTierCard
                key={t.id}
                tier={t}
                busy={busy}
                testable
                onInstall={(id) => modelAction('install', id)}
                onRemove={(id) => modelAction('remove', id)}
              />
            ))}
          </div>
          {actionError && <p className="fl-voice-tier-error">{actionError}</p>}
        </>
      )}
    </>
  )
}
