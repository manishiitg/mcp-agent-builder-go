import { useEffect, useRef, useState, type CSSProperties, type FormEvent } from 'react'
import {
  ArrowLeft, ArrowUp, Clock3, FolderOpen, KeyRound,
  LayoutGrid, ListChecks, LogOut, Menu, MessageSquareText, MoreHorizontal, Paperclip, Play,
  Plus, RefreshCw, Search, Settings, Sparkles, Video, X,
} from 'lucide-react'
import { api, mediaURL, projectFileURL, type ApiUser } from './api'
import { useVideoStore, type AppSection } from './store'
import type { ProjectWorkflow, VideoProject, WorkflowRun } from './types'
import { ChatMarkdown, type ChatMarkdownLinkProps } from '../../shared/chat/ChatRenderer'
import { ProjectFileBrowser } from '../../shared/files/ProjectFileBrowser'
import { ExecutionActivityFeed, useExecutionEvents } from '../../packages/execution-events'

type InspectorTab = 'videos' | 'assets' | 'workflows' | 'file'
const PIPELINE_FALLBACK_DESCRIPTION = 'How an idea moves from a brief to a finished video.'

// The newest run is first (the API orders runs by updated_at DESC). Used only
// for the header pill and the Workflow tab's running-dot — the panel itself is
// a static reference view of the pipeline definition, not a run tracker: it
// looks identical before, during, and after a run.
function latestRun(workflow: ProjectWorkflow): WorkflowRun | undefined { return workflow.runs[0] }
function runningStep(run?: WorkflowRun) { return run?.status === 'running' ? run.steps.find((step) => step.status === 'running') : undefined }

function WorkflowPanel({ workflow }: { workflow: ProjectWorkflow }) {
  return <div className="inspector-body">
    <div className="inspector-title"><div><h2>Supported workflows</h2><p>The agent chooses the right approach for each request.</p></div></div>
    <div className="workflow-catalog">
      {workflow.workflows.map((definition) => <section className="workflow-card" key={definition.id}>
        <header><h3>{definition.name}</h3><p>{definition.description || PIPELINE_FALLBACK_DESCRIPTION}</p></header>
        <div className="workflow-template">
          {definition.steps.map((stage, index) => <div key={stage.id}>
            <i>{stage.position || index + 1}</i>
            <span><strong>{stage.title}</strong>{stage.summary && <small>{stage.summary}</small>}</span>
          </div>)}
        </div>
      </section>)}
    </div>
  </div>
}

function initials(name: string) { return name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]).join('').toUpperCase() || 'U' }
function projectArtStyle(palette: [string, string]): CSSProperties { return { '--art-a': palette[0], '--art-b': palette[1] } as CSSProperties }
function videoPreviewTime(duration: number) { return Number.isFinite(duration) && duration > 0 ? Math.min(0.25, duration / 10) : 0 }

function LoginScreen({ onSubmit }: { onSubmit: (username: string, password: string) => Promise<void> }) {
  const [username, setUsername] = useState('manish')
  const [password, setPassword] = useState('12345')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit(event: FormEvent) {
    event.preventDefault(); if (!username.trim() || !password.trim()) return
    setBusy(true); setError('')
    try { await onSubmit(username.trim(), password) } catch (err) { setError(err instanceof Error ? err.message : 'Could not sign in') } finally { setBusy(false) }
  }
  return <main className="login-page">
    <section className="login-story" aria-label="Product introduction">
      <div className="login-brand"><span className="brand-mark"><Play size={18} fill="currentColor" /></span> Video Studio</div>
      <div className="login-copy"><span className="eyebrow light">YOUR CREATIVE PARTNER</span><h1>Make videos by<br />talking through ideas.</h1><p>One ongoing conversation for every project. Your references and previous work stay together.</p></div>
    </section>
    <section className="login-form-wrap"><form className="login-card" onSubmit={submit}>
      <div className="mobile-brand"><span className="brand-mark"><Play size={16} fill="currentColor" /></span> Video Studio</div>
      <p className="eyebrow">VIDEO STUDIO</p><h2>Welcome back</h2><p className="muted">Sign in to open your projects.</p>
      <label>Username<input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" /></label>
      <label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" /></label>
      {error && <p className="form-error">{error}</p>}
      <button className="primary-button login-button" type="submit" disabled={busy}>{busy ? 'Please wait…' : 'Continue'} <ArrowUp size={17} className="arrow-diagonal" /></button>
    </form></section>
  </main>
}

function Sidebar({ section, onSection, user, onLogout }: { section: AppSection; onSection: (section: AppSection) => void; user: ApiUser; onLogout: () => void }) {
  return <aside className="sidebar">
    <div className="app-brand"><span className="brand-mark"><Play size={16} fill="currentColor" /></span><span>Video Studio</span></div>
    <nav className="side-nav" aria-label="Primary navigation">
      <button className={section === 'projects' ? 'active' : ''} onClick={() => onSection('projects')}><LayoutGrid size={18} /> Projects</button>
      <button className={section === 'settings' ? 'active' : ''} onClick={() => onSection('settings')}><Settings size={18} /> Settings</button>
    </nav>
    <div className="sidebar-account"><span className="account-avatar">{initials(user.name)}</span><span className="account-copy"><strong>{user.name}</strong><small>{user.email}</small></span><button className="icon-button dark" aria-label="Sign out" title="Sign out" onClick={onLogout}><LogOut size={16} /></button></div>
  </aside>
}

function ProjectCard({ project, onOpen }: { project: VideoProject; onOpen: () => void }) {
  return <article className="project-card" role="button" aria-label={`Open ${project.title}`} onClick={onOpen} tabIndex={0} onKeyDown={(event) => event.key === 'Enter' && onOpen()}>
    <div className="project-card-body"><div className="project-card-title"><span className="project-card-icon"><MessageSquareText size={18} /></span><h3>{project.title}</h3><button className="icon-button" aria-label={`More options for ${project.title}`} onClick={(event) => event.stopPropagation()}><MoreHorizontal size={18} /></button></div><p>{project.description}</p><footer><span className="updated"><Clock3 size={14} /> {project.updatedAt}</span><span className="card-video-count"><Video size={13} /> {project.videos.length} {project.videos.length === 1 ? 'video' : 'videos'}</span></footer></div>
  </article>
}

function Dashboard({ projects, onOpen, onCreate }: { projects: VideoProject[]; onOpen: (id: string) => void; onCreate: () => void }) {
  const [query, setQuery] = useState('')
  const matching = projects.filter((project) => `${project.title} ${project.description}`.toLowerCase().includes(query.trim().toLowerCase()))
  return <div className="page-scroll dashboard-page">
    <header className="dashboard-topbar"><div><h1>Projects</h1><p className="dashboard-description">Open a project to continue its conversation, assets, and videos.</p></div><button className="primary-button" onClick={onCreate}><Plus size={18} /> New project</button></header>
    <section className="project-section"><div className="section-toolbar"><div><h2>All projects</h2><p>{matching.length} projects</p></div><label className="search-box"><Search size={17} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search projects" /></label></div>
      {matching.length ? <div className="project-grid">{matching.map((project) => <ProjectCard key={project.id} project={project} onOpen={() => onOpen(project.id)} />)}<button className="new-project-card" onClick={onCreate}><span><Plus size={22} /></span><strong>Create another project</strong><small>Start with an empty workspace</small></button></div> : <div className="empty-state"><FolderOpen size={30} /><h3>No projects yet</h3><p>Create a project to begin a conversation.</p></div>}
    </section>
  </div>
}

const PROJECT_FILE_ROOTS = ['uploads', 'work', 'outputs', 'runs'] as const

function projectRelativePath(href?: string) {
  if (!href || /^(https?:|mailto:|#)/i.test(href)) return null
  const clean = href.split(/[?#]/, 1)[0].replace(/\\/g, '/')
  for (const root of PROJECT_FILE_ROOTS) {
    if (clean.startsWith(`${root}/`)) return clean
    const marker = `/${root}/`
    const index = clean.lastIndexOf(marker)
    if (index >= 0) return clean.slice(index + 1)
  }
  return null
}

function ProjectFilePreview({ projectId, path, onClose }: { projectId: string; path: string; onClose: () => void }) {
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const extension = path.split('.').pop()?.toLowerCase() ?? ''
  const url = projectFileURL(projectId, path)
  const isText = ['md', 'markdown', 'txt', 'json', 'csv', 'js', 'ts', 'tsx', 'jsx', 'py', 'sh', 'css', 'html', 'htm'].includes(extension)
  useEffect(() => {
    if (!isText || extension === 'html' || extension === 'htm') return
    let cancelled = false
    setText(''); setError('')
    fetch(url, { credentials: 'include' }).then((response) => {
      if (!response.ok) throw new Error('Could not open this file')
      return response.text()
    }).then((content) => { if (!cancelled) setText(content) }).catch((reason: Error) => { if (!cancelled) setError(reason.message) })
    return () => { cancelled = true }
  }, [extension, isText, url])
  const name = path.split('/').pop() || path
  return <div className="inspector-body file-preview"><div className="file-preview-header"><button onClick={onClose}><ArrowLeft size={15} /> Back</button><div><h2>{name}</h2><p>{path}</p></div></div><div className="file-preview-content">
    {error ? <div className="file-preview-empty">{error}</div>
      : extension === 'md' || extension === 'markdown' ? text ? <ChatMarkdown text={text} /> : <div className="file-preview-empty">Opening file…</div>
        : extension === 'html' || extension === 'htm' ? <iframe title={name} sandbox="" src={url} />
          : ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(extension) ? <img src={url} alt={name} />
            : ['mp4', 'mov', 'webm', 'm4v'].includes(extension) ? <video src={url} controls />
              : ['mp3', 'wav', 'm4a', 'aac', 'ogg'].includes(extension) ? <audio src={url} controls />
                : isText ? text ? <pre>{text}</pre> : <div className="file-preview-empty">Opening file…</div>
                  : <a className="file-preview-open" href={url} target="_blank" rel="noreferrer">Open file</a>}
  </div></div>
}

function ProjectWorkspace({ project, onBack, onSend, onSteer, onCancel, onUpload }: {
  project: VideoProject; onBack: () => void;
  onSend: (body: string) => Promise<void>;
  onSteer: (body: string) => Promise<void>; onCancel: () => Promise<void>; onUpload: (files: File[]) => Promise<void>;
}) {
  const [draft, setDraft] = useState(''); const [tab, setTab] = useState<InspectorTab>('videos'); const [error, setError] = useState('')
  const [previewPath, setPreviewPath] = useState('')
  const [activeVideoID, setActiveVideoID] = useState('')
  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [refreshingProject, setRefreshingProject] = useState(false)
  const streaming = useVideoStore((state) => state.streams[project.id] ?? '')
  const refreshWorkflow = useVideoStore((state) => state.refreshWorkflow)
  const { events: executionEvents, refresh: refreshExecutionEvents } = useExecutionEvents({
    client: api.executionEvents,
    scopeId: project.id,
    refreshIntervalMs: 2_000,
  })
  const fileInput = useRef<HTMLInputElement>(null); const messages = useRef<HTMLDivElement>(null); const messageEnd = useRef<HTMLDivElement>(null)
  const videoPlayers = useRef<Record<string, HTMLVideoElement | null>>({})
  const videoCards = useRef<Record<string, HTMLElement | null>>({})
  const presentedVideo = useRef({ projectID: '', signature: '' })
  const pendingAutoPlay = useRef('')
  const followLatestMessage = useRef(true)
  useEffect(() => {
    if (!followLatestMessage.current) return
    messageEnd.current?.scrollIntoView({ behavior: streaming ? 'auto' : 'smooth', block: 'end' })
  }, [executionEvents, project.messages, project.sessionStatus, streaming])
  useEffect(() => {
    followLatestMessage.current = true
    const frame = window.requestAnimationFrame(() => {
      const element = messages.current
      if (element) element.scrollTop = element.scrollHeight
    })
    return () => window.cancelAnimationFrame(frame)
  }, [project.id])
  useEffect(() => {
    const newest = project.videos[0]
    const signature = newest ? `${newest.id}:${newest.presentedAt}` : ''
    if (presentedVideo.current.projectID !== project.id) {
      presentedVideo.current = { projectID: project.id, signature }
      return
    }
    if (!newest || signature === presentedVideo.current.signature) return
    presentedVideo.current.signature = signature
    pendingAutoPlay.current = newest.id
    setTab('videos')
    setActiveVideoID(newest.id)
    setInspectorOpen(true)
  }, [project.id, project.videos])
  useEffect(() => {
    if (!activeVideoID || pendingAutoPlay.current !== activeVideoID) return
    pendingAutoPlay.current = ''
    const player = videoPlayers.current[activeVideoID]
    videoCards.current[activeVideoID]?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    if (!player) return
    player.currentTime = 0
    // Playback can be rejected for an unmuted video after an asynchronous
    // agent turn. Keep the video expanded with native controls in that case.
    void player.play().catch(() => undefined)
  }, [activeVideoID])
  useEffect(() => {
    // Background workflow completions resume the persistent main-agent session
    // after the initiating HTTP stream has ended. Keep the open project synced
    // so that synthetic-turn reply appears without a manual refresh, including
    // when the completed run has already returned to its reusable "ready" state.
    const timer = window.setInterval(() => { void refreshWorkflow(project.id) }, 2000)
    return () => window.clearInterval(timer)
  }, [project.id, refreshWorkflow])
  async function sendMessage(event: FormEvent) {
    event.preventDefault(); const body = draft.trim(); if (!body) return; followLatestMessage.current = true; setDraft(''); setError('')
    try { if (project.sessionStatus === 'working') await onSteer(body); else await onSend(body) } catch (err) { setError(err instanceof Error ? err.message : 'Message failed') }
  }
  async function refreshProject() {
    setRefreshingProject(true); setError('')
    try { await Promise.all([refreshWorkflow(project.id), refreshExecutionEvents()]) }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not refresh this project') }
    finally { setRefreshingProject(false) }
  }
  async function addFiles(files: FileList | null) { if (!files?.length) return; setError(''); try { await onUpload(Array.from(files)); setTab('assets') } catch (err) { setError(err instanceof Error ? err.message : 'Upload failed') } finally { if (fileInput.current) fileInput.current.value = '' } }
  function ProjectChatLink({ href, children }: ChatMarkdownLinkProps) {
    const path = projectRelativePath(href)
    if (!path) return <a href={href} target="_blank" rel="noreferrer">{children}</a>
    return <a href={href} onClick={(event) => { event.preventDefault(); setPreviewPath(path); setTab('file') }}>{children}</a>
  }
  // A stage can still be running after the chat turn that started it has ended,
  // so "busy" has to consider the workflow too — otherwise the header reads
  // "Ready" while a run is actively working and the user assumes nothing happened.
  const activeRun = latestRun(project.workflow)?.status === 'running' ? latestRun(project.workflow) : undefined
  const activeStage = runningStep(activeRun)
  const busy = project.sessionStatus === 'working' || Boolean(activeRun)
  const busyLabel = project.sessionStatus === 'working' ? 'Working' : activeRun ? (activeStage ? activeStage.title : 'Working') : 'Ready'
  return <div className="project-workspace">
    <header className="project-header"><div className="project-header-left"><button className="header-back" onClick={onBack} aria-label="Back to projects"><ArrowLeft size={17} /><span>Projects</span></button></div><div className="project-header-title"><h1>{project.title}</h1></div><div className="project-actions"><span className={`session-pill ${busy ? 'working' : ''}`}><i /> {busyLabel}</span>{project.sessionStatus === 'working' && <button className="secondary-button compact-button" onClick={() => void onCancel()}>Cancel</button>}<button className="mobile-inspector-toggle" type="button" aria-label={`Open project videos, ${project.videos.length} available`} onClick={() => { setTab('videos'); setInspectorOpen(true) }}><Video size={16} /><span>{project.videos.length}</span></button></div></header>
    <div className="workspace-columns"><section className="chat-surface">
      <div ref={messages} className="messages" aria-live="polite" onScroll={(event) => {
        const element = event.currentTarget
        followLatestMessage.current = element.scrollHeight - element.scrollTop - element.clientHeight < 120
      }}>
        {project.messages.map((message) => <article key={message.id} className={`message ${message.role}${message.role === 'assistant' ? ' final-answer' : ''}`}>{message.role === 'assistant' && <div className="message-avatar"><Sparkles size={16} /></div>}<div className="message-content">{message.role === 'assistant' ? <ChatMarkdown text={message.body} linkComponent={ProjectChatLink} workspaceLinkRoots={PROJECT_FILE_ROOTS} /> : <p>{message.body}</p>}{message.role === 'assistant' && <time className="message-time">{message.time}</time>}</div></article>)}
        {project.sessionStatus === 'working' && <article className="message assistant working-message"><div className="message-avatar"><Sparkles size={16} /></div><div className="message-content">
          {streaming && <div className="stream-think"><span className="stream-think-label">Thinking</span><div className="stream-think-body is-streaming"><ChatMarkdown text={streaming} streaming linkComponent={ProjectChatLink} workspaceLinkRoots={PROJECT_FILE_ROOTS} /></div></div>}
          <div className="working-state"><div className="typing-dots"><i /><i /><i /></div><span className="working-label">{activeStage ? activeStage.title : 'Working on your request'}</span></div>
        </div></article>}
        {/* Shown whenever a run is live — including when the chat agent itself is
            idle, which is the normal state while background stages grind away. */}
        {(executionEvents.length > 0 || activeRun) && <article className="message assistant activity-message"><div className="message-avatar"><Sparkles size={16} /></div><div className="message-content">
          <ExecutionActivityFeed events={executionEvents} />
          {activeRun && <div className="working-state"><div className="typing-dots"><i /><i /><i /></div><span className="working-label">{activeStage ? `Working on ${activeStage.title.toLowerCase()}` : 'Starting…'}</span></div>}
        </div></article>}
        {error && <p className="chat-error">{error}</p>}<div ref={messageEnd} />
      </div>
      <div className="composer-dock"><form className="composer" onSubmit={sendMessage}><textarea value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => {
        // While an IME is composing (Japanese, Chinese, Korean), Enter confirms
        // the candidate rather than ending the message — submitting there sends a
        // half-typed word and loses the rest.
        if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); event.currentTarget.form?.requestSubmit() }
      }} placeholder={project.sessionStatus === 'working' ? 'Add another direction…' : 'Message your project…'} rows={3} /><div className="composer-tools"><button type="button" className="composer-icon" onClick={() => fileInput.current?.click()} aria-label="Attach files"><Paperclip size={18} /></button><button className="send-button" type="submit" disabled={!draft.trim()} aria-label="Send message"><ArrowUp size={18} /></button></div><input ref={fileInput} hidden type="file" multiple onChange={(event) => void addFiles(event.target.files)} /></form><p className="composer-hint">{project.sessionStatus === 'working' ? 'Your new message will guide the current work' : 'Enter to send · Shift + Enter for a new line'}</p></div>
    </section>{inspectorOpen && <button className="mobile-inspector-backdrop" type="button" aria-label="Close project panel" onClick={() => setInspectorOpen(false)} />}<aside className={`project-inspector${inspectorOpen ? ' is-open' : ''}`}><div className="mobile-inspector-header"><strong>Project</strong><button type="button" aria-label="Close project panel" onClick={() => setInspectorOpen(false)}><X size={18} /></button></div><div className="inspector-tabs" role="tablist"><button className={tab === 'videos' ? 'active' : ''} onClick={() => setTab('videos')}><Video size={16} /> Videos <span>{project.videos.length}</span></button><button className={tab === 'assets' ? 'active' : ''} onClick={() => setTab('assets')}><FolderOpen size={16} /> Assets</button><button className={tab === 'workflows' ? 'active' : ''} onClick={() => setTab('workflows')}><ListChecks size={16} /> Workflow{activeRun && <i className="tab-running-dot" aria-label="Running" />}</button></div>
      {tab === 'file' && previewPath ? <ProjectFilePreview projectId={project.id} path={previewPath} onClose={() => setTab('assets')} />
        : tab === 'videos' ? <div className="inspector-body"><div className="inspector-title"><div><h2>Project videos</h2><p>Everything created in this conversation</p></div><button className={`icon-button refresh-button${refreshingProject ? ' is-refreshing' : ''}`} type="button" aria-label="Refresh project videos" title="Refresh videos" disabled={refreshingProject} onClick={() => void refreshProject()}><RefreshCw size={16} /></button></div>{project.videos.length ? <div className="video-list">{project.videos.map((video) => <article ref={(element) => { videoCards.current[video.id] = element }} className={`video-row${activeVideoID === video.id ? ' is-active' : ''}`} key={video.id}><div className="project-video-preview"><video ref={(element) => { videoPlayers.current[video.id] = element }} className="project-video-player" src={mediaURL(video.contentUrl)} controls={activeVideoID === video.id} playsInline preload="metadata" aria-label={video.title} onLoadedMetadata={(event) => { if (activeVideoID !== video.id) event.currentTarget.currentTime = videoPreviewTime(event.currentTarget.duration) }} onEnded={(event) => { event.currentTarget.currentTime = videoPreviewTime(event.currentTarget.duration); setActiveVideoID('') }} />{activeVideoID !== video.id && <button className="project-video-play" type="button" aria-label={`Play ${video.title}`} onClick={(event) => { setActiveVideoID(video.id); const player = event.currentTarget.parentElement?.querySelector<HTMLVideoElement>('video'); if (player) { player.currentTime = 0; void player.play() } }}><Play size={20} fill="currentColor" /></button>}</div><div className="video-meta"><strong>{video.title}</strong><span>{video.note || video.createdAt}</span></div></article>)}</div> : <div className="inspector-empty"><Video size={26} /><h3>No videos yet</h3><p>Describe your first video in chat and it will appear here.</p></div>}</div>
          : tab === 'workflows' ? <WorkflowPanel workflow={project.workflow} />
            : <div className="inspector-body"><div className="inspector-title"><div><h2>Project files</h2><p>All files and folders in this project</p></div><button className="small-add" onClick={() => fileInput.current?.click()}><Plus size={16} /> Add</button></div><ProjectFileBrowser nodes={project.files} onOpen={(path) => { setPreviewPath(path); setTab('file') }} /></div>}
    </aside></div>
  </div>
}

// The token is what lets a session start at all, so this card carries its own
// state rather than sharing the generic key list: saving it round-trips through
// `claude auth status`, and a rejected token must say so instead of appearing
// saved.
function ProviderTokenCard() {
  const [configured, setConfigured] = useState<boolean | null>(null); const [value, setValue] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  useEffect(() => { void api.providerToken().then((result) => setConfigured(result.configured)).catch(() => setConfigured(false)) }, [])
  async function save(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError('')
    try { await api.putProviderToken(value); setConfigured(true); setValue('') }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not save the token') }
    finally { setBusy(false) }
  }
  async function remove() {
    setError('')
    try { await api.deleteProviderToken(); setConfigured(false) }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not remove the token') }
  }
  return <section className="setting-card featured secrets-card"><span className="setting-icon"><KeyRound size={20} /></span><div>
    <h2>Claude Code token</h2>
    <p>{configured ? 'Your token is saved. Projects run on your own Claude subscription.' : 'Required before any project can start. Run '}{!configured && <code>claude setup-token</code>}{!configured && ' in a terminal and paste the result here.'}</p>
    <form className="secret-form" onSubmit={save}><input type="password" value={value} onChange={(event) => setValue(event.target.value)} placeholder={configured ? 'Paste a new token to replace it' : 'sk-ant-oat…'} /><button className="primary-button" disabled={!value.trim() || busy}>{busy ? 'Checking…' : 'Save'}</button></form>
    {error && <p className="form-error">{error}</p>}
    {configured && <div className="secret-list"><div><code>Token saved</code><button onClick={() => void remove()}>Remove</button></div></div>}
  </div></section>
}

function SettingsPage() {
  const [names, setNames] = useState<string[]>([]); const [name, setName] = useState(''); const [value, setValue] = useState(''); const [error, setError] = useState('')
  useEffect(() => { void api.secretNames().then((result) => setNames(result.names)).catch((err: Error) => setError(err.message)) }, [])
  async function save(event: FormEvent) { event.preventDefault(); setError(''); try { const result = await api.putSecret(name, value); setNames((current) => [...new Set([...current, result.name])].sort()); setName(''); setValue('') } catch (err) { setError(err instanceof Error ? err.message : 'Could not save secret') } }
  async function remove(secretName: string) { try { await api.deleteSecret(secretName); setNames((current) => current.filter((item) => item !== secretName)) } catch (err) { setError(err instanceof Error ? err.message : 'Could not delete secret') } }
  return <div className="page-scroll settings-page"><header><h1>Settings</h1><p>Manage the keys used by your video tools.</p></header><div className="settings-grid">
    <ProviderTokenCard />
    <section className="setting-card featured secrets-card"><span className="setting-icon"><KeyRound size={20} /></span><div><h2>Saved keys</h2><p>Add the keys needed by the tools you want to use. Saved values remain hidden.</p><form className="secret-form" onSubmit={save}><input value={name} onChange={(event) => setName(event.target.value.toUpperCase())} placeholder="KEY_NAME" /><input type="password" value={value} onChange={(event) => setValue(event.target.value)} placeholder="Key value" /><button className="primary-button" disabled={!name || !value}>Save</button></form>{error && <p className="form-error">{error}</p>}<div className="secret-list">{names.map((secret) => <div key={secret}><code>{secret}</code><button onClick={() => void remove(secret)}>Remove</button></div>)}{!names.length && <small>No keys saved.</small>}</div></div></section>
  </div></div>
}

function CreateProjectDialog({ onClose, onCreate }: { onClose: () => void; onCreate: (title: string, description: string) => Promise<void> }) {
  const [title, setTitle] = useState(''); const [description, setDescription] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  async function submit(event: FormEvent) { event.preventDefault(); if (!title.trim()) return; setBusy(true); setError(''); try { await onCreate(title.trim(), description.trim()) } catch (err) { setError(err instanceof Error ? err.message : 'Could not create project'); setBusy(false) } }
  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><form className="dialog-card" onSubmit={submit}><button className="icon-button dialog-close" type="button" onClick={onClose} aria-label="Close"><X size={19} /></button><span className="dialog-icon"><MessageSquareText size={22} /></span><p className="eyebrow">NEW CONVERSATION</p><h2>Create a project</h2><p>Give the conversation a home. You can start creating immediately.</p><label>Project name<input autoFocus value={title} onChange={(event) => setTitle(event.target.value)} placeholder="e.g. Summer campaign" /></label><label>Description <span>Optional</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} placeholder="What will you create here?" rows={3} /></label>{error && <p className="form-error">{error}</p>}<div className="dialog-actions"><button className="secondary-button" type="button" onClick={onClose}>Cancel</button><button className="primary-button" type="submit" disabled={!title.trim() || busy}>{busy ? 'Creating…' : 'Create and open'} <ArrowUp size={17} className="arrow-diagonal" /></button></div></form></div>
}

export default function VideoApp() {
  const user = useVideoStore((state) => state.user)
  const projects = useVideoStore((state) => state.projects)
  const loading = useVideoStore((state) => state.loading)
  const section = useVideoStore((state) => state.section)
  const selectedProjectId = useVideoStore((state) => state.selectedProjectId)
  const creating = useVideoStore((state) => state.creating)
  const bootstrap = useVideoStore((state) => state.bootstrap)
  const authenticate = useVideoStore((state) => state.authenticate)
  const logout = useVideoStore((state) => state.logout)
  const setSection = useVideoStore((state) => state.setSection)
  const selectProject = useVideoStore((state) => state.selectProject)
  const setCreating = useVideoStore((state) => state.setCreating)
  const createProject = useVideoStore((state) => state.createProject)
  const sendMessage = useVideoStore((state) => state.sendMessage)
  const steer = useVideoStore((state) => state.steer)
  const cancel = useVideoStore((state) => state.cancel)
  const upload = useVideoStore((state) => state.upload)
  const selectedProject = projects.find((project) => project.id === selectedProjectId) ?? null

  useEffect(() => { void bootstrap() }, [bootstrap])

  if (loading) return <div className="app-loading">Loading Video Studio…</div>
  if (!user) return <LoginScreen onSubmit={authenticate} />
  return <div className="app-shell">{!selectedProject && <Sidebar section={section} onSection={setSection} user={user} onLogout={() => void logout()} />}<main className={selectedProject ? 'app-main project-main' : 'app-main'}>{selectedProject ? <ProjectWorkspace project={selectedProject} onBack={() => selectProject(null)} onSend={(body) => sendMessage(selectedProject.id, body)} onSteer={(body) => steer(selectedProject.id, body)} onCancel={() => cancel(selectedProject.id)} onUpload={(files) => upload(selectedProject.id, files)} /> : section === 'settings' ? <SettingsPage /> : <Dashboard projects={projects} onOpen={selectProject} onCreate={() => setCreating(true)} />}</main>{creating && <CreateProjectDialog onClose={() => setCreating(false)} onCreate={createProject} />}<button className="mobile-menu" aria-label="Open navigation"><Menu size={20} /></button></div>
}
