import { AlertTriangle, CheckCircle2 } from 'lucide-react'
import type { ProviderManifestEntry } from '../../services/llm-config-api'

/** Shared "is this provider usable" label for the library grid and the
 * workflow provider picker, so both surfaces agree on what Ready means. */
export const providerStatus = (provider: ProviderManifestEntry, locked: boolean) => {
  if (locked) {
    return { label: 'Managed', tone: 'text-blue-600 dark:text-blue-400', icon: CheckCircle2 }
  }
  if (provider.usable) {
    return { label: 'Ready', tone: 'text-emerald-600 dark:text-emerald-400', icon: CheckCircle2 }
  }
  if (provider.runtime_available === false) {
    return { label: 'Not installed', tone: 'text-amber-600 dark:text-amber-400', icon: AlertTriangle }
  }
  if (provider.requires_api_key && !provider.auth_configured) {
    return { label: 'Needs key', tone: 'text-amber-600 dark:text-amber-400', icon: AlertTriangle }
  }
  return { label: 'Setup needed', tone: 'text-amber-600 dark:text-amber-400', icon: AlertTriangle }
}
