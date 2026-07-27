// Shared types used across LearningApp.tsx and the Zustand stores.

export type Screen = 'engine' | 'child' | 'pin' | 'parent' | 'tutor'
export type DrawerTab = 'assets' | 'map' | 'progress' | 'files' | 'allfiles' | 'uploaded'

export type ApiEngine = {
  id: string
  name: string
  runtime_command?: string
  runtime_available: boolean
  auth_configured: boolean
  usable: boolean
  setup_hint?: string
  deprecated?: boolean
}

export type ConvMeta = { id: string; title: string; when: string; scope: 'parent' | 'child'; updated: string }

export type DebugToolCall = { tool: string; args?: string; result?: string; err?: string }

// Voice (speech-to-text / read-aloud) — mirrors the Go side's voiceTier /
// voiceStatusResponse in cmd/family-server/voice_hardware.go.
export type VoiceTier = {
  id: string
  label: string
  description: string
  size_mb?: number
  languages: string
  available: boolean
  unavailable_reason?: string
  installed: boolean
  // Distinct from `installed`: a model can be fully installed on disk yet
  // still cold (the background process that keeps it loaded in memory was
  // never started this session, or unloaded itself after 15 idle minutes).
  // Only `warm` means the next use is actually instant.
  warm?: boolean
  coming_soon?: boolean
  installing?: boolean
  got_bytes?: number
  total_bytes?: number
  install_error?: string
  can_install?: boolean
  can_remove?: boolean
}
export type VoiceChoice = { id: string; label: string; accent?: string; gender?: 'male' | 'female' }
export type VoiceStatus = {
  hardware: { arch: string; is_apple_silicon: boolean; total_ram_bytes: number }
  stt_tiers: VoiceTier[]
  tts_tiers: VoiceTier[]
}
export type ParentMsg = { role: 'user' | 'assistant' | 'tool'; text?: string; tool?: string; name?: string; grade?: string; board?: string; stars?: number; reason?: string; source?: string; html?: string; toolCalls?: DebugToolCall[] }
export type StoredMsg = { role: string; text?: string; tool?: string; stars?: number; reason?: string; source?: string; html?: string }

export type TreeNode = { name: string; path: string; type: 'dir' | 'file'; children?: TreeNode[]; size?: number }

export type WsFile = { path: string; name: string; scope: string; subject: string; topic: string }

export type ActivityItem = { path: string; name: string }

// Activity mirrors the backend's activityResp (packages.go) — the
// activity-folder model: a self-contained <Subject>/<Topic>/<slug>/ folder,
// keyed by its own workspace-relative `dir` (not a separate manifest path).
export type Activity = {
  dir: string
  title: string
  subject?: string
  topic?: string
  items: ActivityItem[]
  guide_note?: string
  goal?: string
  teaching_mode?: string
  persona?: string
  created_at?: string
  attempts?: ActivityItem[]
}

// toParentMsg reconstructs a persisted transcript entry (incl. a celebrate
// event) into what the UI renders — so reloading a conversation replays star
// moments exactly where they happened, not just the surrounding text.
export function toParentMsg(m: StoredMsg): ParentMsg {
  return { role: m.role as ParentMsg['role'], text: m.text, tool: m.tool, stars: m.stars, reason: m.reason, source: m.source, html: m.html }
}
