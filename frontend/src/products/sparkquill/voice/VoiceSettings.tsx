// The Settings → Voice panel: the speech-to-text engine.
//
// The platform runs one shared engine (agent_go/pkg/voicestt) whose state
// comes from /api/voice/status, so this renders what the server reports and
// never guesses what this Mac can run. "Install" is the platform's warm
// call: it starts the one-time model download and keeps the status polling
// (owned by the parent) showing progress.

import { useState } from 'react'
import type { VoiceStatus } from '../stores'
import { VoiceTierCard } from './VoiceTierCard'
import { FAMILY_API } from '../apiBase'
import { authHeaders } from './useMicDictation'

const PROFILE_ID = 'sparkquill'

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

  const install = () => {
    setBusy(true)
    setActionError(null)
    fetch(`${FAMILY_API}/api/voice/warm?profile_id=${PROFILE_ID}`, { method: 'POST', headers: authHeaders() })
      .then(async (r) => {
        const data = await r.json().catch(() => ({}))
        if (!r.ok) throw new Error((data as { error?: string })?.error || `Failed (${r.status})`)
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
          <p className="fl-note">Used by the microphone in the message box.</p>
          <div className="fl-settings-engines">
            {(status.stt_tiers ?? []).map((t) => (
              <VoiceTierCard
                key={t.id}
                tier={t}
                busy={busy}
                testable
                onInstall={install}
              />
            ))}
          </div>
          {actionError && <p className="fl-voice-tier-error">{actionError}</p>}
        </>
      )}
    </>
  )
}
