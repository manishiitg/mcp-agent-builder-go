import { useState } from 'react'
import { KeyRound, X } from 'lucide-react'
import ModalPortal from '../ui/ModalPortal'
import { authApi } from '../../services/api'

interface ChangePasswordDialogProps {
  isOpen: boolean
  onClose: () => void
}

/**
 * ChangePasswordDialog - lets a signed-in user change their own password.
 *
 * The backend (`POST /api/auth/password`) has existed since the user
 * directory landed, but only admins had a UI (the reset in Users & access);
 * a normal member or read-only account had no way to rotate their own
 * password. The server re-checks the current password and the 8-character
 * minimum; an SSO-only account (no password hash) is told so by the server
 * and the message is shown as-is.
 */
export default function ChangePasswordDialog({ isOpen, onClose }: ChangePasswordDialogProps) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  if (!isOpen) return null

  const reset = () => {
    setCurrent('')
    setNext('')
    setConfirm('')
    setError(null)
    setDone(false)
  }
  const close = () => {
    reset()
    onClose()
  }

  const mismatch = confirm.length > 0 && next !== confirm
  const canSubmit = current.length > 0 && next.length >= 8 && next === confirm && !submitting

  const submit = async () => {
    if (!canSubmit) return
    setSubmitting(true)
    setError(null)
    try {
      await authApi.changeOwnPassword(current, next)
      setDone(true)
      setCurrent('')
      setNext('')
      setConfirm('')
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setError(e.response?.data?.error || e.message || 'Could not change the password')
    } finally {
      setSubmitting(false)
    }
  }

  const inputClass =
    'w-full mt-1 px-2 py-1.5 text-sm bg-muted/40 border border-border rounded focus:outline-none focus:ring-1 focus:ring-primary'

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={close}>
        <div
          className="bg-background border border-border rounded-lg shadow-xl w-full max-w-sm flex flex-col"
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            if (e.key === 'Escape') close()
            if (e.key === 'Enter') void submit()
          }}
        >
          <div className="flex items-center justify-between p-4 border-b border-border">
            <div className="flex items-center gap-2">
              <KeyRound className="w-5 h-5 text-muted-foreground" />
              <h2 className="text-lg font-semibold">Change password</h2>
            </div>
            <button onClick={close} className="p-1 rounded hover:bg-accent" aria-label="Close">
              <X className="w-4 h-4" />
            </button>
          </div>

          {done ? (
            <div className="p-4 flex flex-col gap-3">
              <p className="text-sm">Your password has been changed. Use the new one next time you sign in.</p>
              <button onClick={close} className="self-end px-3 py-1.5 text-sm rounded bg-primary text-primary-foreground hover:bg-primary/90">
                Done
              </button>
            </div>
          ) : (
            <div className="p-4 flex flex-col gap-3">
              <label className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Current password
                <input type="password" autoFocus autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} className={inputClass} />
              </label>
              <label className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                New password
                <input type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} placeholder="min 8 characters" className={inputClass} />
              </label>
              <label className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Confirm new password
                <input type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} className={inputClass} />
              </label>
              {mismatch && <p className="text-xs text-destructive">The two new passwords do not match.</p>}
              {error && <p className="text-xs text-destructive">{error}</p>}
              <div className="flex justify-end gap-2 pt-1">
                <button onClick={close} className="px-3 py-1.5 text-sm rounded border border-border hover:bg-accent">Cancel</button>
                <button
                  disabled={!canSubmit}
                  onClick={() => void submit()}
                  className="px-3 py-1.5 text-sm rounded bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                >
                  {submitting ? 'Saving' : 'Save password'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </ModalPortal>
  )
}
