// The one seam between the SparkQuill UI and whatever serves it.
//
// Every backend call the app makes goes through this interface. Today the
// only implementation is the standalone family server (standaloneApi.ts);
// the AgentWorks platform implementation slots in behind the same shape,
// so the 4,900-line component never has to know which one it is talking to.
//
// Shapes mirror the family server's JSON exactly (see cmd/family-server).
import type { ApiEngine, Activity, StoredMsg, ToolCallRecord, TreeNode, VoiceStatus } from '../stores/types'

export type SetupState = {
  engine?: string
  child?: { name?: string; grade?: string; board?: string } | null
  pin_set?: boolean
  setup_complete?: boolean
  next_step?: string
  parent_label?: string
}

/**
 * `replace` carries the whole live preview so far (the platform joins chunks
 * per provider: verbatim for fragment streams, block-wise for claude-code);
 * `delta` is only used by the standalone backend which streams raw text.
 */
export type TurnStreamEvent = { type?: 'delta' | 'replace' | 'status' | 'tool_call' | string; text?: string; tool_call?: ToolCallRecord }

/** Side-effect signals a turn produced; the same struct for parent and child. */
export type ToolEvent = {
  tool: string
  name?: string
  grade?: string
  board?: string
  path?: string
  focus?: string
  package?: string
  stars?: number
  total?: number
  reason?: string
  parent_label?: string
}

export type Suggestion = { label: string; message: string }

export type TurnResult = {
  reply?: string
  error?: string
  suggestions?: Suggestion[]
  tool_events?: ToolEvent[]
  tool_calls?: ToolCallRecord[]
  /** child turns only: a transient HTML scene to show inline */
  scene?: string
}

export type TurnMessage = { role: string; text: string; source?: string }

export type FileContent = { path?: string; is_text?: boolean; content?: string; size?: number }
export type TreeResponse = TreeNode[] | { nodes?: TreeNode[]; total_size?: number }
export type UploadResult = { name?: string; path?: string; error?: string }

export type ScheduleEntry = { day: string; start: string; end: string; label: string }
export type WeekActivityEntry = { date: string; activity_dir: string; title: string; duration_seconds?: number }
export type WeekDeadline = { title: string; subject?: string; due_date?: string; kind?: string }
export type WeekDay = { date: string; weekday: string; schedule?: ScheduleEntry[]; activities?: WeekActivityEntry[]; deadlines?: WeekDeadline[] }
export type WeekResponse = { week_start: string; week_end: string; days: WeekDay[]; upcoming_deadlines?: WeekDeadline[] }

export type ModelInfo = { provider: string; selected: string; default: string; models: { id: string; label: string }[] }
export type FastMode = { enabled: boolean; child_enabled: boolean }

export type WhatsAppVoiceTranscription = { enabled: boolean; installed: boolean; installing: boolean; model_size_mb: number; available: boolean; error?: string }
export type WhatsAppStatus = {
  accounts: { jid: string; connected: boolean }[]
  pairing: { qr_available: boolean; qr_expires_at?: string }
  voice_transcription?: WhatsAppVoiceTranscription
}

export type PulseConfig = { enabled: boolean; cadence_hours: number; last_run_at?: string; watch_sites?: string[]; preferred_hour: number; preferred_hour_set: boolean }
export type PulseConfigPatch = Partial<Pick<PulseConfig, 'enabled' | 'cadence_hours' | 'watch_sites' | 'preferred_hour' | 'preferred_hour_set'>>

export type StoredConversation = { messages?: StoredMsg[] }

export interface FamilyApi {
  /** Where the backend lives; shown in the "can't reach" message. */
  readonly baseUrl: string

  // ---- setup ------------------------------------------------------------
  setup(): Promise<SetupState>
  engines(): Promise<ApiEngine[]>
  validateEngine(provider: string): Promise<{ valid: boolean; message?: string }>
  selectEngine(engine: string): Promise<void>
  saveChild(child: { name: string; grade: string; board: string }): Promise<void>
  setPin(pin: string): Promise<{ error?: string }>
  verifyPin(pin: string): Promise<{ ok?: boolean }>

  // ---- parent conversation -------------------------------------------------
  /**
   * Runs one parent turn: onEvent receives the live preview (delta / status /
   * tool_call) while the promise resolves with the authoritative result.
   */
  sendParentTurn(req: { messages: TurnMessage[]; conversationId: string; viewerPath?: string }, onEvent: (e: TurnStreamEvent) => void): Promise<TurnResult>
  steerParent(conversationId: string, message: string): Promise<{ steered?: boolean }>
  /** Watches a conversation for turns started elsewhere (WhatsApp, Pulse). Returns unsubscribe. */
  watchParent(conversationId: string, onEvent: (e: TurnStreamEvent) => void): () => void
  loadParentConversation(): Promise<StoredConversation | null>

  // ---- child conversation --------------------------------------------------
  childActivity(): Promise<Activity | null>
  handoff(dir: string, resume: boolean): Promise<{ new_session?: boolean; dir?: string; goal?: string }>
  sendChildTurn(req: { messages: TurnMessage[]; conversationId: string }, onEvent: (e: TurnStreamEvent) => void): Promise<TurnResult>
  steerChild(conversationId: string, message: string): Promise<{ steered?: boolean }>
  watchChild(activityDir: string, onEvent: (e: TurnStreamEvent) => void): () => void
  loadChildConversation(activityDir: string): Promise<StoredConversation | null>

  // ---- workspace -----------------------------------------------------------
  tree(): Promise<TreeResponse>
  readFile(path: string): Promise<FileContent>
  /** URL a browser element can load the file's bytes from. */
  rawUrl(path: string, opts?: { download?: boolean; print?: boolean }): string
  /** URL of a static asset the server ships (jsxgraph, ...). */
  assetUrl(relPath: string): string
  upload(file: File, scope: 'parent' | 'child'): Promise<UploadResult>
  saveState(key: string, data: unknown): Promise<void>
  loadState(key: string): Promise<unknown>
  activities(): Promise<Activity[]>
  week(offset: number): Promise<WeekResponse>
  saveSchedule(entries: ScheduleEntry[]): Promise<void>

  // ---- settings ------------------------------------------------------------
  models(): Promise<ModelInfo | null>
  saveModel(modelId: string): Promise<void>
  fastMode(): Promise<FastMode>
  saveFastMode(patch: { enabled: boolean; child_enabled?: boolean }): Promise<void>
  secrets(): Promise<string[]>
  saveSecret(name: string, value: string): Promise<string[]>
  deleteSecret(name: string): Promise<string[]>
  voiceStatus(): Promise<VoiceStatus>
  browserStatus(): Promise<{ cli_installed: boolean }>

  // ---- connectors & pulse --------------------------------------------------
  whatsappStatus(): Promise<WhatsAppStatus>
  whatsappPairImageUrl(nonce: number): string
  whatsappUnpair(jid: string): Promise<void>
  whatsappVoice(enabled: boolean): Promise<WhatsAppVoiceTranscription>
  pulseConfig(): Promise<PulseConfig>
  savePulseConfig(patch: PulseConfigPatch): Promise<PulseConfig>
  runPulse(): Promise<{ ok: boolean; error?: string }>
}
