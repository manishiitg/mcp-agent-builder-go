export const REPORT_PREVIEW_PREFERENCE_KEY = 'workflow_report_preview_preference'
export const REPORT_PREVIEW_PREFERENCE_CHANGED_EVENT = 'workflow-report-preview-preference-changed'

export type ReportPreviewDevice = 'mobile' | 'tablet' | 'desktop'
export const DEFAULT_REPORT_PREVIEW_DEVICE: ReportPreviewDevice = 'tablet'

export function reportPreviewPreferenceKey(scopeId?: string | null): string {
  return scopeId ? `${REPORT_PREVIEW_PREFERENCE_KEY}:${scopeId}` : REPORT_PREVIEW_PREFERENCE_KEY
}

export function isReportPreviewDevice(value: unknown): value is ReportPreviewDevice {
  return value === 'mobile' || value === 'tablet' || value === 'desktop'
}

export function readReportPreviewPreference(scopeId?: string | null): ReportPreviewDevice {
  if (typeof window === 'undefined') return DEFAULT_REPORT_PREVIEW_DEVICE
  try {
    const saved = window.localStorage.getItem(reportPreviewPreferenceKey(scopeId))
    return isReportPreviewDevice(saved) ? saved : DEFAULT_REPORT_PREVIEW_DEVICE
  } catch {
    return DEFAULT_REPORT_PREVIEW_DEVICE
  }
}

export function writeReportPreviewPreference(
  scopeId: string | null | undefined,
  preference: ReportPreviewDevice,
): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(reportPreviewPreferenceKey(scopeId), preference)
  } catch {
    // UI preference only.
  }
  window.dispatchEvent(new CustomEvent(REPORT_PREVIEW_PREFERENCE_CHANGED_EVENT, {
    detail: { preference, scopeId: scopeId ?? null },
  }))
}

export function openWorkflowInDefaultPreview(scopeId?: string | null): void {
  writeReportPreviewPreference(scopeId, DEFAULT_REPORT_PREVIEW_DEVICE)
}
