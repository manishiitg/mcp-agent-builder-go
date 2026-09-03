import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { X, Loader2, AlertCircle, Share2, Trash2, Plus } from 'lucide-react'
import { authApi, type WorkflowAccessInfo, type WorkflowAccessUser } from '../../services/api'
import { useAuthStore } from '../../stores/useAuthStore'
import { useWorkflowManifestStore } from '../../stores/useWorkflowManifestStore'
import ModalPortal from '../ui/ModalPortal'

interface WorkflowSharePopupProps {
  isOpen: boolean
  onClose: () => void
  workspacePath: string
}

/**
 * Share one workflow: who owns it (edit, run, share, delete) and who may
 * read it (chat, run, watch — nothing changes). Owners and admins only; the
 * server refuses anything else and never lets the last owner go.
 * docs/design/user_accounts_and_workflow_sharing.md, phase 3.
 */
const WorkflowSharePopup: React.FC<WorkflowSharePopupProps> = ({ isOpen, onClose, workspacePath }) => {
  const me = useAuthStore((s) => s.user)
  const refreshWorkflows = useWorkflowManifestStore((s) => s.refreshWorkflows)
  const [info, setInfo] = useState<WorkflowAccessInfo | null>(null)
  const [directory, setDirectory] = useState<WorkflowAccessUser[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pickId, setPickId] = useState('')
  const [pickRole, setPickRole] = useState<'reader' | 'owner'>('reader')

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [access, dir] = await Promise.all([authApi.getWorkflowAccess(workspacePath), authApi.listUserDirectory()])
      setInfo(access)
      setDirectory(dir.users || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [workspacePath])

  useEffect(() => {
    if (isOpen) void load()
  }, [isOpen, load])

  const save = useCallback(async (owners: string[], readers: string[]) => {
    setSaving(true)
    setError(null)
    try {
      const next = await authApi.setWorkflowAccess(workspacePath, owners, readers)
      setInfo(next)
      void refreshWorkflows()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }, [workspacePath, refreshWorkflows])

  const ownerIds = useMemo(() => (info?.owners ?? []).map((u) => u.id), [info])
  const readerIds = useMemo(() => (info?.readers ?? []).map((u) => u.id), [info])
  const candidates = useMemo(
    () => directory.filter((u) => !ownerIds.includes(u.id) && !readerIds.includes(u.id)),
    [directory, ownerIds, readerIds],
  )

  const add = () => {
    if (!pickId) return
    if (pickRole === 'owner') void save([...ownerIds, pickId], readerIds)
    else void save(ownerIds, [...readerIds, pickId])
    setPickId('')
  }
  const remove = (id: string) => void save(ownerIds.filter((x) => x !== id), readerIds.filter((x) => x !== id))
  const promote = (id: string) => void save([...ownerIds, id], readerIds.filter((x) => x !== id))
  const demote = (id: string) => void save(ownerIds.filter((x) => x !== id), [...readerIds, id])

  if (!isOpen) return null
  const label = (u: WorkflowAccessUser) => (u.id === me?.id ? `${u.username} (you)` : u.username)

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
        <div className="bg-background border border-border rounded-lg shadow-xl w-full max-w-xl max-h-[85vh] flex flex-col" onClick={(e) => e.stopPropagation()}>
          <div className="flex items-center justify-between p-4 border-b border-border">
            <div className="flex items-center gap-2">
              <Share2 className="w-5 h-5 text-muted-foreground" />
              <h2 className="text-lg font-semibold">Share this workflow</h2>
            </div>
            <button onClick={onClose} className="p-1 rounded hover:bg-accent" aria-label="Close"><X className="w-4 h-4" /></button>
          </div>
          <div className="px-4 pt-3 pb-2 text-xs text-muted-foreground">
            <span className="font-mono">{workspacePath}</span>. Owners edit, run, share and delete. Read-only people can chat, run and watch, but change nothing.
            {info?.legacy && <span className="block mt-1 text-amber-600">Nothing recorded yet: every member can edit this workflow until you save a first grant.</span>}
          </div>
          <div className="px-4 pb-3 border-b border-border flex flex-col gap-2 sm:flex-row sm:items-end">
            <div className="flex-1">
              <label className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Add a person</label>
              <select value={pickId} onChange={(e) => setPickId(e.target.value)} className="w-full mt-1 px-2 py-1.5 text-sm bg-muted/40 border border-border rounded">
                <option value="">Choose a user…</option>
                {candidates.map((u) => <option key={u.id} value={u.id}>{u.username}{u.email ? ` · ${u.email}` : ''}</option>)}
              </select>
            </div>
            <div className="sm:w-36">
              <label className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">As</label>
              <select value={pickRole} onChange={(e) => setPickRole(e.target.value as 'reader' | 'owner')} className="w-full mt-1 px-2 py-1.5 text-sm bg-muted/40 border border-border rounded">
                <option value="reader">Read-only</option>
                <option value="owner">Owner</option>
              </select>
            </div>
            <button onClick={add} disabled={!pickId || saving} className="inline-flex items-center gap-1 px-3 py-1.5 text-sm rounded bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50">
              {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Plus className="w-3.5 h-3.5" />} Add
            </button>
          </div>
          <div className="flex-1 overflow-auto px-4 py-3 text-sm">
            {error && (
              <div className="mb-2 flex items-start gap-2 text-xs text-destructive bg-destructive/10 p-2 rounded"><AlertCircle className="w-4 h-4 mt-0.5" /><span>{error}</span></div>
            )}
            {loading || !info ? (
              <div className="flex items-center justify-center py-8 text-muted-foreground"><Loader2 className="w-5 h-5 animate-spin" /></div>
            ) : (
              <>
                <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground mb-1">Owners</p>
                {info.owners.length === 0 && <p className="text-xs text-muted-foreground mb-3">No owner recorded.</p>}
                {info.owners.map((u) => (
                  <div key={u.id} className="flex items-center justify-between py-1.5 border-t border-border">
                    <span>{label(u)}{u.email && <span className="ml-1 text-xs text-muted-foreground">{u.email}</span>}</span>
                    <span className="flex items-center gap-1">
                      <button className="text-xs px-2 py-0.5 rounded border border-border hover:bg-accent" disabled={saving || info.owners.length < 2} title={info.owners.length < 2 ? 'A workflow needs at least one owner' : 'Make read-only'} onClick={() => demote(u.id)}>Make read-only</button>
                      <button className="p-1 rounded hover:bg-destructive/10 text-destructive" disabled={saving || info.owners.length < 2} title="Remove" onClick={() => remove(u.id)}><Trash2 className="w-3.5 h-3.5" /></button>
                    </span>
                  </div>
                ))}
                <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground mt-4 mb-1">Read-only</p>
                {info.readers.length === 0 && <p className="text-xs text-muted-foreground">Nobody yet.</p>}
                {info.readers.map((u) => (
                  <div key={u.id} className="flex items-center justify-between py-1.5 border-t border-border">
                    <span>{label(u)}{u.email && <span className="ml-1 text-xs text-muted-foreground">{u.email}</span>}</span>
                    <span className="flex items-center gap-1">
                      <button className="text-xs px-2 py-0.5 rounded border border-border hover:bg-accent" disabled={saving} onClick={() => promote(u.id)}>Make owner</button>
                      <button className="p-1 rounded hover:bg-destructive/10 text-destructive" disabled={saving} title="Remove" onClick={() => remove(u.id)}><Trash2 className="w-3.5 h-3.5" /></button>
                    </span>
                  </div>
                ))}
              </>
            )}
          </div>
        </div>
      </div>
    </ModalPortal>
  )
}

export default WorkflowSharePopup
