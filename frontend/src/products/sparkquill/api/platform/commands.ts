import type { QuickCommand } from '../../stores/types'

/**
 * The product's `commands:` (GET /api/agent-profiles/{id}) as quick-menu
 * entries. The description is the label the parent sees (the name is the
 * slash-command identifier); a command without a prompt is dropped rather than
 * offered as a button that sends nothing.
 */
export function quickCommandsFromProfile(profile: { commands?: Array<Record<string, unknown>> } | null | undefined): QuickCommand[] {
  return (profile?.commands ?? []).flatMap((c) => {
    const label = String(c.description ?? '').trim() || String(c.name ?? '').trim()
    const message = String(c.prompt ?? '').trim()
    return label && message ? [{ label, message }] : []
  })
}
