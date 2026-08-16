import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useShallow } from "zustand/react/shallow";
import { useEffect, useCallback, useRef, useState, forwardRef, lazy, Suspense } from "react";
import { ThemeProvider } from "./contexts/ThemeContext.tsx";
import { UpdateProgressToast } from "./components/UpdateProgressToast";
import { GlobalHumanFeedbackPrompt } from "./components/GlobalHumanFeedbackPrompt";
import Workspace from "./components/Workspace.tsx";
import { ChiefTasksPanel } from "./components/org/OrgHtmlPanels";
import { ORG_HTML_PREVIEW_PREFERENCE_CHANGED_EVENT, getOrgHtmlPreviewDevice, setOrgHtmlPreviewDevice as persistOrgHtmlPreviewDevice, type OrgHtmlPreviewDevice } from "./components/org/orgHtmlPreview";
import ChatArea, { type ChatAreaRef } from "./components/ChatArea.tsx";
import { FileContentViewer } from "./components/FileContentViewer";
import { resetSessionId, agentApi } from "./services/api";
import { AuthWrapper } from "./components/AuthWrapper";
import { findBlockingMultiAgentSession, shouldConfirmForSessionStatus, shouldConfirmNewMultiAgentChat } from "./utils/newChatConfirmation";
import { Loader2, PanelRightClose, PanelRightOpen, Smartphone, Laptop } from "lucide-react";
import { WorkflowLayout } from "./components/workflow";
import { ModePresetBar } from "./components/ModePresetBar";
import { ChatTabs } from "./components/ChatTabs";
import ConfirmationDialog from "./components/ui/ConfirmationDialog";
import { useAppStore, useMCPStore, useGlobalPresetStore, useWorkspaceStore, useWorkflowStore, useChatStore } from "./stores";
import { useModeStore } from "./stores/useModeStore";
import { useProductSurfaceStore } from "./stores/useProductSurfaceStore";
import { useLLMStore } from "./stores/useLLMStore";
import { normalizeEventViewMode, waitForChatStoreHydration, type ChatTab } from "./stores/useChatStore";
import { isChiefOfStaffTab, isChiefOfStaffScheduleTab, isInteractiveChiefOfStaffTab } from "./utils/chiefOfStaff";
import { useLLMDefaults } from "./hooks/useLLMDefaults";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./components/ui/tooltip";
import "./App.css";

// Extend window interface for global functions
declare global {
  interface Window {
    highlightFile?: (filepath: string) => void;
    toggleAutoScroll?: () => void;
    perfDiag?: () => void;
  }
}

import { copyToClipboard } from './utils/textUtils'
import LazyModalFallback from './components/ui/LazyModalFallback'

const queryClient = new QueryClient();

const QuickSwitcher = lazy(() => import('./components/QuickSwitcher'))
const WorkflowsOverviewPage = lazy(() => import('./components/WorkflowsOverviewPage').then(module => ({ default: module.WorkflowsOverviewPage })))
const VideoStudioSurface = lazy(() => import('./products/video-studio/VideoStudioSurface').then(module => ({ default: module.VideoStudioSurface })))
const ChiefOfStaffSurface = lazy(() => import('./products/chief-of-staff/ChiefOfStaffSurface').then(module => ({ default: module.ChiefOfStaffSurface })))

const FileSurfaceFallback = () => (
  <div className="flex h-full min-h-40 items-center justify-center text-muted-foreground">
    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
    Loading viewer...
  </div>
)

const WORKSPACE_COLLAPSING_POPUP_SELECTOR = [
  '[data-workspace-popup="true"]',
  '[role="dialog"]',
  '[class~="fixed"][class~="inset-0"]'
].join(',')
const WORKSPACE_COLLAPSE_IGNORE_SELECTOR = '[data-workspace-collapse-ignore="true"]'
const READ_ONLY_WORKFLOW_RESTORE_SELECTION_WINDOW_MS = 60 * 1000

const workflowTabSortTimestamp = (tab: ChatTab) => tab.lastAccessedAt ?? tab.createdAt ?? 0

const isInteractiveWorkflowTab = (tab: ChatTab | null | undefined): tab is ChatTab =>
  !!tab && tab.metadata?.mode === 'workflow' && tab.metadata?.isViewOnly !== true

const isRecentExplicitReadOnlyWorkflowTab = (tab: ChatTab | null | undefined): tab is ChatTab => {
  const restoredAt = tab?.metadata?.readOnlyRestoredAt
  return !!tab &&
    tab.metadata?.mode === 'workflow' &&
    tab.metadata?.isViewOnly === true &&
    typeof restoredAt === 'number' &&
    Date.now() - restoredAt <= READ_ONLY_WORKFLOW_RESTORE_SELECTION_WINDOW_MS
}

const hasOpenWorkspaceCollapsingPopup = () => {
  if (typeof document === 'undefined') return false

  return Array.from(document.querySelectorAll<HTMLElement>(WORKSPACE_COLLAPSING_POPUP_SELECTOR))
    .some((element) => {
      if (element.closest(WORKSPACE_COLLAPSE_IGNORE_SELECTOR)) {
        return false
      }
      const style = window.getComputedStyle(element)
      return (
        style.display !== 'none' &&
        style.visibility !== 'hidden' &&
        style.position === 'fixed'
      )
    })
}


// Helper component to get observerId and render ChatArea
// Always renders ChatArea (even without observerId) so header with mode/preset selectors is visible
// Uses Zustand hooks to reactively update when tabs change
const ChatAreaWithObserverId = forwardRef<ChatAreaRef, { onNewChat: () => void; previousChatsCompact?: boolean }>(({ onNewChat, previousChatsCompact }, ref) => {
  // Pass null (not undefined) when the active tab is a workflow tab so this hidden
  // instance doesn't steal SSE connections, polling, or queue processing from
  // WorkflowLayout's ChatArea which is the primary instance for workflow tabs.
  const activeTabId = useChatStore(state => {
    const tabId = state.activeTabId
    const tab = tabId ? state.chatTabs[tabId] : null
    return tab?.metadata?.mode === 'workflow' ? null : (tabId || undefined)
  })

  return (
    <ChatArea
      ref={ref}
      onNewChat={onNewChat}
      tabId={activeTabId}
      previousChatsCompact={previousChatsCompact}
    />
  )
})

const multiAgentPanelTabClass = (active: boolean) =>
  `rounded px-2.5 py-1 text-xs font-medium whitespace-nowrap transition-colors ${
    active ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
  }`

function App() {
  // Ref for ChatArea component to access its methods
  const chatAreaRef = useRef<ChatAreaRef>(null)
  const [orgHtmlPreviewDevice, setOrgHtmlPreviewDevice] = useState<OrgHtmlPreviewDevice>(() => getOrgHtmlPreviewDevice())
  const productSurface = useProductSurfaceStore(state => state.productSurface)
  const chatAutoScroll = useChatStore(state => state.autoScroll)

  // Store subscriptions
  const setAgentMode = useAppStore(state => state.setAgentMode)
  const { hasCompletedInitialSetup, selectedModeCategory, setModeCategory, completeInitialSetup } = useModeStore(useShallow(state => ({
    hasCompletedInitialSetup: state.hasCompletedInitialSetup,
    selectedModeCategory: state.selectedModeCategory,
    setModeCategory: state.setModeCategory,
    completeInitialSetup: state.completeInitialSetup,
  })))
  const defaultsLoaded = useLLMStore(state => state.defaultsLoaded)
  const savedLLMs = useLLMStore(state => state.savedLLMs)
  const llmConfigLocked = useLLMStore(state => state.llmConfigLocked)
  const isConfigValid = useLLMStore(state => state.isConfigValid)
  const setShowLLMModal = useLLMStore(state => state.setShowLLMModal)
  
  // Load LLM defaults from backend
  useLLMDefaults()

  useEffect(() => {
    if (productSurface !== 'agentworks' || selectedModeCategory !== 'multi-agent') return
    let cancelled = false
    void waitForChatStoreHydration().then(async () => {
      if (cancelled || useProductSurfaceStore.getState().productSurface !== 'agentworks') return
      const chatStore = useChatStore.getState()
      const chiefTab = Object.values(chatStore.chatTabs).find(isInteractiveChiefOfStaffTab)
      if (chiefTab) {
        chatStore.switchTab(chiefTab.tabId)
      } else {
        await chatStore.createChatTab('Chief of Staff', { mode: 'multi-agent' })
      }
    })
    return () => { cancelled = true }
  }, [productSurface, selectedModeCategory])
  
  // App Store subscriptions for workspace and chat
  const {
    setSelectedPresetId,
    workspaceMinimized,
    workspaceMinimizedByMode,
    setWorkspaceMinimized,
    setWorkspaceMinimizedForLayout,
    multiAgentRightPanelView,
    setMultiAgentRightPanelView,
    showWorkflowsOverview,
    setShowWorkflowsOverview
  } = useAppStore(useShallow(state => ({
    setSelectedPresetId: state.setSelectedPresetId,
    workspaceMinimized: state.workspaceMinimized,
    workspaceMinimizedByMode: state.workspaceMinimizedByMode,
    setWorkspaceMinimized: state.setWorkspaceMinimized,
    setWorkspaceMinimizedForLayout: state.setWorkspaceMinimizedForLayout,
    multiAgentRightPanelView: state.multiAgentRightPanelView,
    setMultiAgentRightPanelView: state.setMultiAgentRightPanelView,
    showWorkflowsOverview: state.showWorkflowsOverview,
    setShowWorkflowsOverview: state.setShowWorkflowsOverview,
  })))
  const [hasOpenedWorkflowsOverview, setHasOpenedWorkflowsOverview] = useState(showWorkflowsOverview)
  
  useEffect(() => {
    const handler = (event: Event) => {
      const preference = (event as CustomEvent).detail?.preference
      if (preference === 'mobile' || preference === 'desktop') {
        setOrgHtmlPreviewDevice(preference)
      }
    }
    window.addEventListener(ORG_HTML_PREVIEW_PREFERENCE_CHANGED_EVENT, handler)
    return () => window.removeEventListener(ORG_HTML_PREVIEW_PREFERENCE_CHANGED_EVENT, handler)
  }, [])

  const submitMultiAgentPanelCommand = useCallback((query: string) => {
    setWorkspaceMinimized(false)
    chatAreaRef.current?.submitQuery(query).catch(error => {
      console.error('[App] Failed to submit org panel command:', error)
    })
  }, [setWorkspaceMinimized])

  // Expose performance diagnostics on window for DevTools console
  useEffect(() => {
    window.perfDiag = () => {
      const chatState = useChatStore.getState()
      const tabs = Object.values(chatState.chatTabs)
      const workflowTabs = tabs.filter(t => t.metadata?.mode === 'workflow')
      const streamingTabs = tabs.filter(t => t.isStreaming)
      const sseConns = chatState.sseConnections
      const sseCount = Object.keys(sseConns).length

      let totalEvents = 0
      let totalEventBytes = 0
      let largestEventBytes = 0
      const eventDetails: Array<{ session: string; tabs: number; tabNames: string; events: number; evtSizeKB: number; avgEventKB: number; largestEventKB: number; largestEventType: string; mode: string; preset: string; streaming: boolean; hasSSE: boolean }> = []
      const duplicateSessionDetails: Array<{ session: string; tabs: number; tabNames: string; events: number; evtSizeKB: number }> = []
      const largestEvents: Array<{ session: string; type: string; sizeKB: number; id: string; tabNames: string }> = []
      const tabsBySession = new Map<string, typeof tabs>()
      for (const tab of tabs) {
        if (tab.sessionId) {
          tabsBySession.set(tab.sessionId, [...(tabsBySession.get(tab.sessionId) || []), tab])
        }
      }
      for (const [sid, sessionTabs] of tabsBySession.entries()) {
        const events = chatState.tabEvents[sid] || []
        const count = events.length
        const sizeEstimate = JSON.stringify(events).length
        const tabNames = sessionTabs.map(t => t.name.slice(0, 24)).join(', ')
        totalEvents += count
        totalEventBytes += sizeEstimate

        let largestForSessionBytes = 0
        let largestForSessionType = ''
        for (const event of events) {
          const eventBytes = JSON.stringify(event).length
          if (eventBytes > largestForSessionBytes) {
            largestForSessionBytes = eventBytes
            largestForSessionType = event.type || '(unknown)'
          }
          if (eventBytes > largestEventBytes) largestEventBytes = eventBytes
          if (eventBytes >= 50 * 1024) {
            largestEvents.push({
              session: sid.slice(0, 8),
              type: event.type || '(unknown)',
              sizeKB: Math.round(eventBytes / 1024),
              id: (event.id || '').slice(0, 16),
              tabNames,
            })
          }
        }

        if (count > 0) {
          const firstTab = sessionTabs[0]
          eventDetails.push({
            session: sid.slice(0, 8),
            tabs: sessionTabs.length,
            tabNames,
            events: count,
            evtSizeKB: Math.round(sizeEstimate / 1024),
            avgEventKB: count > 0 ? Math.round(sizeEstimate / count / 1024) : 0,
            largestEventKB: Math.round(largestForSessionBytes / 1024),
            largestEventType: largestForSessionType,
            mode: firstTab.metadata?.mode || '?',
            preset: (firstTab.metadata?.presetQueryId || '').slice(0, 8),
            streaming: sessionTabs.some(t => t.isStreaming),
            hasSSE: !!sseConns[sid]
          })
        }

        if (sessionTabs.length > 1) {
          duplicateSessionDetails.push({
            session: sid.slice(0, 8),
            tabs: sessionTabs.length,
            tabNames,
            events: count,
            evtSizeKB: Math.round(sizeEstimate / 1024),
          })
        }
      }
      largestEvents.sort((a, b) => b.sizeKB - a.sizeKB)

      // Streaming text sizes
      const streamingTextSizes: Array<{ session: string; sizeKB: number; chars: number }> = []
      let totalStreamingBytes = 0
      for (const [sid, text] of Object.entries(chatState.streamingText)) {
        if (text && text.length > 0) {
          const size = text.length * 2 // approx bytes (UTF-16)
          totalStreamingBytes += size
          streamingTextSizes.push({ session: sid.slice(0, 8), sizeKB: Math.round(size / 1024), chars: text.length })
        }
      }
      const completedStreamingTextSizes: Array<{ session: string; sizeKB: number; chars: number }> = []
      let totalCompletedStreamingBytes = 0
      for (const [sid, text] of Object.entries(chatState.completedStreamingText || {})) {
        if (text && text.length > 0) {
          const size = text.length * 2
          totalCompletedStreamingBytes += size
          completedStreamingTextSizes.push({ session: sid.slice(0, 8), sizeKB: Math.round(size / 1024), chars: text.length })
        }
      }

      // SSE connection details
      const sseDetails: Array<{ session: string; tab: string }> = []
      for (const [sid, _conn] of Object.entries(sseConns)) {
        const tab = tabs.find(t => t.sessionId === sid)
        sseDetails.push({ session: sid.slice(0, 8), tab: tab?.name?.slice(0, 25) || '(orphan!)' })
      }
      // Detect orphan SSE connections (no matching tab)
      const orphanSSE = sseDetails.filter(s => s.tab === '(orphan!)')

      // Orphan tabEvents (no matching tab)
      const tabSessionIds = new Set(tabs.map(t => t.sessionId).filter(Boolean))
      const orphanEvents: Array<{ session: string; events: number; sizeKB: number }> = []
      for (const [sid, events] of Object.entries(chatState.tabEvents)) {
        if (!tabSessionIds.has(sid)) {
          orphanEvents.push({ session: sid.slice(0, 8), events: events.length, sizeKB: Math.round(JSON.stringify(events).length / 1024) })
        }
      }

      // localStorage sizes
      const storeKeys = ['chat-store', 'workflow-store', 'global-preset-storage', 'mode-store', 'mcp-store']
      const storageSizes: Record<string, number> = {}
      let totalLSBytes = 0
      for (const key of storeKeys) {
        const val = localStorage.getItem(key)
        if (val) {
          storageSizes[key] = Math.round(val.length / 1024)
          totalLSBytes += val.length
        }
      }

      // Memory usage (if available)
      const mem = performance.memory
      const memInfo = mem ? {
        usedHeap: Math.round(mem.usedJSHeapSize / 1024 / 1024),
        totalHeap: Math.round(mem.totalJSHeapSize / 1024 / 1024),
        limit: Math.round(mem.jsHeapSizeLimit / 1024 / 1024)
      } : null

      // DOM node count + breakdown of heavy subtrees
      const domNodes = document.querySelectorAll('*').length
      const domBreakdown: Array<{ selector: string; nodes: number }> = []
      const selectors = ['[class*="chat"]', '[class*="event"]', '[class*="message"]', '[class*="editor"]', '[class*="monaco"]', 'svg', 'pre', 'code', '.react-flow', '.react-flow__node', '.react-flow__edge']
      for (const sel of selectors) {
        try {
          const count = document.querySelectorAll(sel).length
          if (count > 50) domBreakdown.push({ selector: sel, nodes: count })
        } catch { /* ignore invalid selectors */ }
      }

      // Active timers/intervals estimate
      // Check for event listeners on window
      const eventListenerCount = typeof window.getEventListeners === 'function'
        ? Object.values(window.getEventListeners(window)).reduce((sum, arr) => sum + arr.length, 0)
        : 'N/A (use DevTools)'

      // Long task detection — start monitoring
      if (!window.__longTaskObserver) {
        try {
          const longTasks: Array<{ duration: number; time: string }> = [];
          window.__longTasks = longTasks
          const observer = new PerformanceObserver((list) => {
            for (const entry of list.getEntries()) {
              longTasks.push({ duration: Math.round(entry.duration), time: new Date().toLocaleTimeString() })
              if (longTasks.length > 100) longTasks.shift()
            }
          })
          observer.observe({ entryTypes: ['longtask'] })
          window.__longTaskObserver = observer
        } catch { /* longtask not supported */ }
      }
      const longTasks = window.__longTasks || []
      const recentLongTasks = longTasks.slice(-10)

      // Frame rate measurement — start if not already running
      if (!window.__fpsSamples) {
        const fpsSamples: number[] = [];
        window.__fpsSamples = fpsSamples
        let lastTime = performance.now()
        let frameCount = 0
        const measureFPS = () => {
          frameCount++
          const now = performance.now()
          if (now - lastTime >= 1000) {
            fpsSamples.push(frameCount)
            if (fpsSamples.length > 30) fpsSamples.shift()
            frameCount = 0
            lastTime = now
          }
          requestAnimationFrame(measureFPS)
        }
        requestAnimationFrame(measureFPS)
      }
      const fpsSamples = window.__fpsSamples
      const avgFPS = fpsSamples.length > 0 ? Math.round(fpsSamples.reduce((a, b) => a + b, 0) / fpsSamples.length) : 'measuring...'
      const minFPS = fpsSamples.length > 0 ? Math.min(...fpsSamples) : 'measuring...'

      // --- OUTPUT ---
      console.log('%c === PERF DIAGNOSTICS ===', 'color: cyan; font-weight: bold; font-size: 14px')

      // Memory
      if (memInfo) {
        const usedPct = Math.round((memInfo.usedHeap / memInfo.limit) * 100)
        const memColor = usedPct > 80 ? 'red' : usedPct > 50 ? 'orange' : 'green'
        console.log(`%c Memory: ${memInfo.usedHeap} MB / ${memInfo.totalHeap} MB (limit: ${memInfo.limit} MB) [${usedPct}% used]`, `color: ${memColor}; font-weight: bold`)
      } else {
        console.log('Memory: N/A (use Chrome-based browser)')
      }

      // FPS
      const fpsColor = (typeof avgFPS === 'number' && avgFPS < 30) ? 'red' : (typeof avgFPS === 'number' && avgFPS < 50) ? 'orange' : 'green'
      console.log(`%c FPS: avg=${avgFPS}, min=${minFPS} (last ${fpsSamples.length}s)`, `color: ${fpsColor}; font-weight: bold`)

      // Tabs & SSE
      console.log(`\nTabs: ${tabs.length} total (${workflowTabs.length} workflow, ${streamingTabs.length} streaming)`)
      console.log(`SSE connections: ${sseCount}${orphanSSE.length > 0 ? ` ⚠️ ${orphanSSE.length} ORPHAN` : ''}`)
      if (orphanSSE.length > 0) {
        console.log('%c Orphan SSE connections (no tab):', 'color: red; font-weight: bold')
        console.table(orphanSSE)
      }

      // Events
      console.log(`\nEvents in memory: ${totalEvents} across ${eventDetails.length} unique sessions (~${Math.round(totalEventBytes / 1024)} KB, largest event ${Math.round(largestEventBytes / 1024)} KB)`)
      if (eventDetails.length > 0) {
        // Sort by estimated memory descending to show biggest first
        eventDetails.sort((a, b) => b.evtSizeKB - a.evtSizeKB)
        console.table(eventDetails)
      }
      if (duplicateSessionDetails.length > 0) {
        console.log(`%c Duplicate tab references: ${duplicateSessionDetails.length} session(s) shown in multiple tabs`, 'color: orange; font-weight: bold')
        console.table(duplicateSessionDetails)
      }
      if (largestEvents.length > 0) {
        console.log(`%c Large retained events (>=50 KB): ${largestEvents.length} total, top 20 shown`, 'color: orange; font-weight: bold')
        console.table(largestEvents.slice(0, 20))
      }

      // Orphan events
      if (orphanEvents.length > 0) {
        console.log(`%c Orphan tabEvents (no matching tab): ${orphanEvents.length} sessions, ${orphanEvents.reduce((s, o) => s + o.events, 0)} events`, 'color: red; font-weight: bold')
        console.table(orphanEvents)
      }

      // Streaming text
      if (streamingTextSizes.length > 0) {
        console.log(`\nStreaming text buffers: ${streamingTextSizes.length} active (~${Math.round(totalStreamingBytes / 1024)} KB)`)
        console.table(streamingTextSizes)
      }
      if (completedStreamingTextSizes.length > 0) {
        console.log(`\nCompleted streaming buffers: ${completedStreamingTextSizes.length} retained (~${Math.round(totalCompletedStreamingBytes / 1024)} KB)`)
        console.table(completedStreamingTextSizes.sort((a, b) => b.sizeKB - a.sizeKB))
      }

      // DOM
      const domColor = domNodes > 10000 ? 'red' : domNodes > 5000 ? 'orange' : 'green'
      console.log(`%c \nDOM nodes: ${domNodes}`, `color: ${domColor}; font-weight: bold`)
      if (domBreakdown.length > 0) {
        console.table(domBreakdown)
      }

      // Long tasks
      console.log(`\nLong tasks (>50ms): ${longTasks.length} total`)
      if (recentLongTasks.length > 0) {
        const avgDuration = Math.round(recentLongTasks.reduce((s, t) => s + t.duration, 0) / recentLongTasks.length)
        const maxDuration = Math.max(...recentLongTasks.map(t => t.duration))
        console.log(`  Recent ${recentLongTasks.length}: avg=${avgDuration}ms, max=${maxDuration}ms`)
        console.table(recentLongTasks)
      }

      // localStorage
      console.log(`\nlocalStorage: ${Math.round(totalLSBytes / 1024)} KB total`)
      console.table(storageSizes)

      // Window event listeners
      console.log(`\nWindow event listeners: ${eventListenerCount}`)

      // React Flow specific diagnostics
      const rfNodes = document.querySelectorAll('.react-flow__node').length
      const rfEdges = document.querySelectorAll('.react-flow__edge').length
      const rfContainers = document.querySelectorAll('.react-flow').length
      if (rfContainers > 0) {
        console.log(`%c \nReact Flow: ${rfContainers} container(s), ${rfNodes} nodes, ${rfEdges} edges`, 'color: #9C27B0; font-weight: bold')
        if (rfContainers > 1) {
          console.log(`%c  ⚠️ Multiple React Flow containers detected — possible leak!`, 'color: red')
        }
      }

      // Workflow store state
      try {
        const wfStore = window.__ZUSTAND_DEVTOOLS__?.['workflow-store'] || null
        if (!wfStore) {
          // Try direct import path
          const presetStates = JSON.parse(localStorage.getItem('workflow-store') || '{}')?.state?._presetStates
          if (presetStates) {
            const presetCount = Object.keys(presetStates).length
            const presetSizeKB = Math.round(JSON.stringify(presetStates).length / 1024)
            console.log(`\nWorkflow preset states cached: ${presetCount} (${presetSizeKB} KB in localStorage)`)
          }
        }
      } catch { /* ignore */ }

      // Summary warnings
      const warnings: string[] = []
      if (memInfo && memInfo.usedHeap > memInfo.limit * 0.8) warnings.push(`Heap at ${Math.round((memInfo.usedHeap / memInfo.limit) * 100)}% of limit!`)
      if (domNodes > 10000) warnings.push(`${domNodes} DOM nodes — UI will lag`)
      if (totalEvents > 5000) warnings.push(`${totalEvents} events in memory — consider clearing old tabs`)
      if (orphanSSE.length > 0) warnings.push(`${orphanSSE.length} orphan SSE connections leaking`)
      if (orphanEvents.length > 0) warnings.push(`${orphanEvents.reduce((s, o) => s + o.events, 0)} orphan events in memory (no tab)`)
      if (totalStreamingBytes > 5 * 1024 * 1024) warnings.push(`${Math.round(totalStreamingBytes / 1024 / 1024)} MB in streaming text buffers`)
      if (totalLSBytes > 5 * 1024 * 1024) warnings.push(`localStorage is ${Math.round(totalLSBytes / 1024 / 1024)} MB — may cause slow persistence`)
      if (typeof avgFPS === 'number' && avgFPS < 30) warnings.push(`FPS avg=${avgFPS} — UI is janky`)
      if (rfContainers > 1) warnings.push(`${rfContainers} React Flow containers — possible mount leak`)
      if (rfNodes > 200) warnings.push(`${rfNodes} React Flow nodes in DOM — heavy canvas`)

      if (warnings.length > 0) {
        console.log('%c \n⚠️  WARNINGS:', 'color: red; font-weight: bold; font-size: 13px')
        warnings.forEach(w => console.log(`%c  • ${w}`, 'color: red'))
      } else {
        console.log('%c \n✅ No obvious issues detected', 'color: green; font-weight: bold')
      }

      console.log('%c ========================', 'color: cyan; font-weight: bold')
      console.log('%c Tip: Run perfDiag() again after interacting to see trends', 'color: gray; font-style: italic')
    }
    return () => { delete window.perfDiag }
  }, [])

  const [showQuickSwitcher, setShowQuickSwitcher] = useState(false)
  const [quickSwitcherInitialQuery, setQuickSwitcherInitialQuery] = useState('')
  
  // Ref to prevent duplicate default tab creation (React StrictMode runs effects twice)
  const hasCreatedDefaultTabRef = useRef<string | null>(null)

  
  const clearActivePreset = useGlobalPresetStore(state => state.clearActivePreset)
  const applyPreset = useGlobalPresetStore(state => state.applyPreset)
  const getActivePreset = useGlobalPresetStore(state => state.getActivePreset)

  useEffect(() => {
    const handleOpenQuickSwitcher = (event: Event) => {
      const detail = (event as CustomEvent<{ query?: string }>).detail
      setQuickSwitcherInitialQuery(detail?.query || '')
      setShowQuickSwitcher(true)
    }

    window.addEventListener('open-quick-switcher', handleOpenQuickSwitcher)
    return () => window.removeEventListener('open-quick-switcher', handleOpenQuickSwitcher)
  }, [])


  const hasInitializedRef = useRef(false)
  const hasCheckedInitialLLMConfigRef = useRef(false)

  // Initialize stores on mount
  useEffect(() => {
    // Prevent double calls in React StrictMode
    if (hasInitializedRef.current) {
      return
    }
    hasInitializedRef.current = true

    // Initialize MCP store
    useMCPStore.getState().refreshTools()
    
    // LLM list is refreshed after loadDefaultsFromBackend() in useLLMDefaults (so supported_providers is set)
    
    // Initialize global preset store
    useGlobalPresetStore.getState().refreshPresets()
    
    // Initialize workflow store (load phases)
    useWorkflowStore.getState().loadPhases()
  }, [])

  // First launch now defaults directly to Chat. If no LLM is configured once
  // backend defaults are loaded, open the LLM configuration dialog instead of
  // asking the user to choose between Chat and Workflow.
  useEffect(() => {
    if (!hasCompletedInitialSetup || !selectedModeCategory) {
      setModeCategory('multi-agent')
      completeInitialSetup()
    }
  }, [hasCompletedInitialSetup, selectedModeCategory, setModeCategory, completeInitialSetup])

  useEffect(() => {
    if (hasCheckedInitialLLMConfigRef.current) return
    if (!defaultsLoaded) return

    hasCheckedInitialLLMConfigRef.current = true
    const hasConfiguredLLM = isConfigValid() || savedLLMs.length > 0 || llmConfigLocked
    if (!hasConfiguredLLM) {
      setShowLLMModal(true)
    }
  }, [defaultsLoaded, isConfigValid, llmConfigLocked, savedLLMs.length, setShowLLMModal])
  
  // Create default tab on page load (only for multi-agent mode, not workflow mode)
  // In workflow mode, tabs are created when user starts a phase/execution
  useEffect(() => {
    if (!hasCompletedInitialSetup) return

    // Only create default tab for multi-agent mode
    // (workflow tabs are created by WorkflowLayout)
    if (selectedModeCategory !== 'multi-agent') {
      return
    }

    let cancelled = false

    const createDefaultTab = async () => {
      await waitForChatStoreHydration()
      if (cancelled) return

      // Prevent duplicate execution (React StrictMode runs effects twice)
      if (hasCreatedDefaultTabRef.current === selectedModeCategory) {
        return
      }

      const chatStore = useChatStore.getState()
      const modeTabs = Object.values(chatStore.chatTabs).filter(isInteractiveChiefOfStaffTab)

      // If tabs already exist for this mode, skip
      if (modeTabs.length > 0) {
        return
      }

      // Mark as in progress for this mode
      hasCreatedDefaultTabRef.current = selectedModeCategory

      try {
        // This effect only runs for multi-agent mode (guarded above); workflow
        // tabs are created by WorkflowLayout.
        await chatStore.createChatTab('Chief of Staff', { mode: 'multi-agent' })
      } catch (error) {
        console.error('Failed to create default tab:', error)
        // Reset flag on error so it can retry
        hasCreatedDefaultTabRef.current = null
      }
    }

    void createDefaultTab()

    return () => {
      cancelled = true
    }
  }, [hasCompletedInitialSetup, selectedModeCategory])

  // Ensure a chat tab is selected after restore (fix for page reload issue)
  // This ensures that when tabs are restored from localStorage, we select the first tab of the current mode
  // if activeTabId is null or invalid or belongs to a different mode
  useEffect(() => {
    if (!hasCompletedInitialSetup) return

    let cancelled = false

    const ensureActiveTab = async () => {
      await waitForChatStoreHydration()
      if (cancelled) return

      // When switching back to workflow mode, restore the active workflow execution tab and
      // ensure the chat panel is visible — otherwise activeTabId stays on whatever mode the
      // user came from (e.g. multi-agent) and the ChatArea inside WorkflowLayout shows wrong content.
      if (selectedModeCategory === 'workflow') {
        const chatStore = useChatStore.getState()
        const workflowStore = useWorkflowStore.getState()
        const activeTabId = chatStore.activeTabId
        const activeTab = activeTabId ? chatStore.getTab(activeTabId) : null
        const activePresetId = useGlobalPresetStore.getState().activePresetIds.workflow

        const activeTabMatchesPreset = activeTab &&
          activeTab.metadata?.mode === 'workflow' &&
          activeTab.metadata?.presetQueryId === activePresetId
        const explicitReadOnlyActiveTab = activeTabMatchesPreset && isRecentExplicitReadOnlyWorkflowTab(activeTab)
          ? activeTab
          : null
        // Tab must match workflow mode and the active preset. Read-only Schedule/Bot
        // tabs only stay active immediately after an explicit open action.
        const hasValidActiveTab = activeTabMatchesPreset &&
          (isInteractiveWorkflowTab(activeTab) || !!explicitReadOnlyActiveTab)

        // Prefer the workflow tab the user last had active for this preset.
        let workflowTabs = Object.values(chatStore.chatTabs)
          .filter(tab => isInteractiveWorkflowTab(tab) && (tab.sessionId || tab.isStreaming))
          .sort((a, b) => workflowTabSortTimestamp(b) - workflowTabSortTimestamp(a))

        if (activePresetId) {
          const presetTabs = workflowTabs.filter(tab => tab.metadata?.presetQueryId === activePresetId)
          if (presetTabs.length > 0) workflowTabs = presetTabs
        }

        const rememberedWorkflowTab = workflowStore.activeWorkflowTabId
          ? chatStore.getTab(workflowStore.activeWorkflowTabId)
          : null
        const rememberedWorkflowTabMatchesPreset = rememberedWorkflowTab &&
          isInteractiveWorkflowTab(rememberedWorkflowTab) &&
          rememberedWorkflowTab.metadata?.presetQueryId === activePresetId
        const builderTab = workflowTabs.find(tab => tab.metadata?.phaseId === 'workflow-builder')
        const streamingTab = workflowTabs.find(tab => chatStore.getTabStreamingStatus(tab.tabId) || tab.isStreaming)
        const activeWorkflowViewMode = normalizeEventViewMode(
          activeTab?.metadata?.mode === 'workflow'
            ? activeTab.viewMode
            : chatStore.eventViewModePreference
        )
        const targetWorkflowTab = explicitReadOnlyActiveTab || (
          activeWorkflowViewMode === 'terminal'
            ? streamingTab ||
              (hasValidActiveTab ? activeTab : null) ||
              (rememberedWorkflowTabMatchesPreset ? rememberedWorkflowTab : null) ||
              builderTab ||
              workflowTabs[0]
            : builderTab ||
              (hasValidActiveTab ? activeTab : null) ||
              (rememberedWorkflowTabMatchesPreset ? rememberedWorkflowTab : null) ||
              streamingTab ||
              workflowTabs[0]
        )

        if (targetWorkflowTab) {
          if (!hasValidActiveTab || activeTabId !== targetWorkflowTab.tabId) {
            chatStore.switchTab(targetWorkflowTab.tabId)
          }

          const shouldShowWorkflowChat =
            workflowStore.showChatArea ||
            targetWorkflowTab.metadata?.phaseId === 'workflow-builder'

          if (shouldShowWorkflowChat) {
            workflowStore.setShowChatArea(true)
          }
        } else {
          // No active workflow tabs - clear activeTabId so WorkflowLayout's ChatArea
          // doesn't display content from another mode
          useChatStore.setState({ activeTabId: null })
        }
        return
      }

      // For multi-agent: select the first tab of the current mode
      // if activeTabId is null, invalid, or belongs to a different mode
      if (selectedModeCategory !== 'multi-agent') {
        return
      }

      const chatStore = useChatStore.getState()

      // Chief of Staff has two independent lanes: one interactive chat and one
      // read-only schedule. Collapse duplicates within each lane, never across
      // lanes, so opening a schedule cannot replace the chat.
      const multiAgentTabs = Object.values(chatStore.chatTabs).filter(tab =>
        isChiefOfStaffTab(tab) &&
        tab.metadata?.isOrganizationAssistant !== true
      )
      const lanes = [
        multiAgentTabs.filter(isInteractiveChiefOfStaffTab),
        multiAgentTabs.filter(isChiefOfStaffScheduleTab),
      ]
      for (const laneTabs of lanes) {
        if (laneTabs.length <= 1) continue
        const keep = laneTabs.reduce((best, tab) =>
          (tab.lastAccessedAt ?? tab.createdAt ?? 0) > (best.lastAccessedAt ?? best.createdAt ?? 0) ? tab : best
        , laneTabs[0])
        for (const tab of laneTabs) {
          if (tab.tabId !== keep.tabId) {
            await chatStore.closeTab(tab.tabId, false)
          }
        }
      }

      const activeTabId = chatStore.activeTabId

      // Check if activeTabId is null, points to a non-existent tab, or belongs to a different mode
      const activeTab = activeTabId ? chatStore.getTab(activeTabId) : null
      const hasValidActiveTab = activeTab &&
        isChiefOfStaffTab(activeTab) &&
        activeTab.metadata?.isOrganizationAssistant !== true

      if (!hasValidActiveTab && multiAgentTabs.length > 0) {
        const interactiveTab = multiAgentTabs
          .filter(isInteractiveChiefOfStaffTab)
          .sort((a, b) => workflowTabSortTimestamp(b) - workflowTabSortTimestamp(a))[0]
        chatStore.switchTab((interactiveTab || multiAgentTabs[0]).tabId)
      }
    }

    void ensureActiveTab()

    return () => {
      cancelled = true
    }
  }, [hasCompletedInitialSetup, selectedModeCategory])

  // Restore active presets after stores are initialized
  const hasRestoredPresetRef = useRef(false)
  useEffect(() => {
    // Only restore presets if initial setup is completed and we have a mode category
    if (hasCompletedInitialSetup && selectedModeCategory) {
      // Add a small delay to ensure stores are fully initialized
      const timer = setTimeout(() => {
        const activePreset = getActivePreset(selectedModeCategory)
        if (activePreset) {
          hasRestoredPresetRef.current = true
          const result = applyPreset(activePreset.id, selectedModeCategory)
          if (!result.success) {
            console.error('[APP] Failed to restore preset:', result.error)
          }
        } else if (selectedModeCategory === 'multi-agent') {
          // For multi-agent mode, if there's no active preset, clear any stale preset server state
          // This prevents old preset servers from persisting when no preset is selected
          hasRestoredPresetRef.current = true
          const { setCurrentPresetServers } = useGlobalPresetStore.getState()
          setCurrentPresetServers([])
        }
        // For workflow mode with no preset found, don't mark as restored — retry below
      }, 500) // 500ms delay to ensure stores are ready

      return () => clearTimeout(timer)
    }
  }, [hasCompletedInitialSetup, selectedModeCategory, getActivePreset, applyPreset])

  // Retry preset restoration for workflow mode after presets finish loading from manifests
  // The 500ms timer above may fire before refreshPresets() completes
  const workflowPresetsForRestore = useGlobalPresetStore(state => state.workflowPresets)
  useEffect(() => {
    if (hasRestoredPresetRef.current) return
    if (!hasCompletedInitialSetup || selectedModeCategory !== 'workflow') return
    if (workflowPresetsForRestore.length === 0) return // Presets not loaded yet

    const activePreset = getActivePreset('workflow')
    if (activePreset) {
      hasRestoredPresetRef.current = true
      applyPreset(activePreset.id, 'workflow')
    }
  }, [hasCompletedInitialSetup, selectedModeCategory, workflowPresetsForRestore, getActivePreset, applyPreset])


  // Start new chat function
  const startNewChat = useCallback(() => {
    
    // Use ChatArea's resetChatState method to clear all chat state without circular call
    if (chatAreaRef.current) {
      chatAreaRef.current.resetChatState();
    }
    
    // Preserve active preset for workflow mode, clear for other modes
    if (selectedModeCategory === 'workflow') {
      // For workflow mode, preserve the active preset
      const { getActivePreset } = useGlobalPresetStore.getState()
      const activePreset = getActivePreset(selectedModeCategory)
      if (activePreset) {
        // Keep the preset selected, just clear the filter
        setSelectedPresetId(null) // Clear filter but keep preset active
        // Don't clear the activePresetId in global store for these modes
        // The preset will be re-applied after chat state is reset
      } else {
        // No preset selected, clear everything
        setSelectedPresetId(null)
        clearActivePreset(selectedModeCategory)
      }
    } else {
      // For other modes (chat, multi-agent), clear preset state as before
      setSelectedPresetId(null); // Clear selected preset filter
      if (selectedModeCategory) {
        clearActivePreset(selectedModeCategory); // Also clear in global store
      }
    }
    
    // Reset the global session ID to force generation of a new one
    resetSessionId();
    
    // Clear the requiresNewChat flag after successful new chat initialization
    useAppStore.getState().clearRequiresNewChat();
    
    // Re-apply active preset for workflow mode after chat reset
    if (selectedModeCategory === 'workflow') {
      const { getActivePreset } = useGlobalPresetStore.getState()
      const activePreset = getActivePreset(selectedModeCategory)
      if (activePreset) {
        // Use setTimeout to ensure chat state reset is complete before applying preset
        setTimeout(() => {
          const result = applyPreset(activePreset.id, selectedModeCategory)
          if (!result.success) {
            console.error('[NEW_CHAT] Failed to re-apply preset:', result.error)
          }
        }, 100)
      }
    }
  }, [setSelectedPresetId, clearActivePreset, selectedModeCategory, applyPreset]);

  // Minimize toggle functions
  const toggleWorkspaceMinimize = useCallback(() => {
    setWorkspaceMinimized(!workspaceMinimized)
  }, [workspaceMinimized, setWorkspaceMinimized])

  // "New Chat" resets the one interactive Chief of Staff lane in place. A
  // concurrently running schedule remains in its separate read-only lane.
  const [showNewChatConfirm, setShowNewChatConfirm] = useState(false)
  const newChatCheckInFlightRef = useRef(false)
  const pendingNewChiefOfStaffTabIdRef = useRef<string | null>(null)
  const requestNewMultiAgentChat = useCallback(() => {
    if (newChatCheckInFlightRef.current) return

    const startFreshChat = (tabId: string) => {
      useChatStore.getState().switchTab(tabId)
      void chatAreaRef.current?.handleNewChat(tabId)
    }

    const checkAndRoute = async () => {
      newChatCheckInFlightRef.current = true
      try {
        const chatStore = useChatStore.getState()
        let activeTab: ChatTab | null = Object.values(chatStore.chatTabs)
          .filter(isInteractiveChiefOfStaffTab)
          .sort((a, b) => workflowTabSortTimestamp(b) - workflowTabSortTimestamp(a))[0] || null
        if (!activeTab) {
          const tabId = await chatStore.createChatTab('Chief of Staff', { mode: 'multi-agent' })
          activeTab = chatStore.getTab(tabId) || null
        }
        if (!activeTab) return
        pendingNewChiefOfStaffTabIdRef.current = activeTab.tabId

        if (shouldConfirmNewMultiAgentChat(activeTab)) {
          setShowNewChatConfirm(true)
          return
        }

        let activeSessionsChecked = false
        let activeSessionFound = false
        try {
          const response = await agentApi.getActiveSessions()
          const activeSession = activeTab?.sessionId
            ? response.active_sessions?.find(session => session.session_id === activeTab.sessionId)
            : undefined
          activeSessionsChecked = true
          activeSessionFound = !!activeSession
          if (shouldConfirmNewMultiAgentChat(activeTab, activeSession)) {
            setShowNewChatConfirm(true)
            return
          }
          const blockingMultiAgentSession = findBlockingMultiAgentSession(response.active_sessions, activeTab?.sessionId)
          if (blockingMultiAgentSession) {
            setShowNewChatConfirm(true)
            return
          }
        } catch (activeSessionsError) {
          console.warn('[NewChat] Failed to check active sessions before resetting chat:', activeSessionsError)
        }

        if (!activeTab?.sessionId) {
          startFreshChat(activeTab.tabId)
          return
        }

        try {
          const status = await agentApi.getSessionStatus(activeTab.sessionId)
          if (shouldConfirmForSessionStatus(status)) {
            setShowNewChatConfirm(true)
            return
          }
          startFreshChat(activeTab.tabId)
        } catch (statusError) {
          console.warn('[NewChat] Failed to check session status before resetting chat:', statusError)
          if (activeSessionsChecked && !activeSessionFound) {
            startFreshChat(activeTab.tabId)
            return
          }
          setShowNewChatConfirm(true)
        }
      } finally {
        newChatCheckInFlightRef.current = false
      }
    }

    void checkAndRoute()
  }, [])
  const confirmNewMultiAgentChat = useCallback(() => {
    setShowNewChatConfirm(false)
    const tabId = pendingNewChiefOfStaffTabIdRef.current
    if (!tabId) return
    useChatStore.getState().switchTab(tabId)
    void chatAreaRef.current?.handleNewChat(tabId)
  }, [])

  // After Ctrl+1/Ctrl+2 mode switch, restore the most recently-accessed
  // tab matching the new mode. Without this the activeTabId stays on
  // whatever was selected before (often a tab in the *other* mode), so
  // the workflow's chat panel doesn't pick up the running session and
  // the user has to click the tab manually.
  const restoreMostRecentTabForMode = useCallback((mode: 'workflow' | 'multi-agent') => {
    const chatStore = useChatStore.getState()
    const currentTab = chatStore.activeTabId ? chatStore.chatTabs[chatStore.activeTabId] : null
    if (
      currentTab &&
      currentTab.metadata?.mode === mode &&
      (mode !== 'workflow' || isInteractiveWorkflowTab(currentTab) || isRecentExplicitReadOnlyWorkflowTab(currentTab))
    ) return
    const candidates = Object.values(chatStore.chatTabs).filter(t =>
      t.metadata?.mode === mode &&
      (mode !== 'workflow' || isInteractiveWorkflowTab(t))
    )
    if (candidates.length === 0) return
    const mostRecent = candidates.reduce((best, t) =>
      workflowTabSortTimestamp(t) > workflowTabSortTimestamp(best) ? t : best
    , candidates[0])
    chatStore.switchTab(mostRecent.tabId)
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // Ctrl/Cmd + 1 for Workflow mode
      if ((event.ctrlKey || event.metaKey) && event.key === '1') {
        event.preventDefault()
        const { setModeCategory } = useModeStore.getState()
        setModeCategory('workflow')
        setShowWorkflowsOverview(false)
        restoreMostRecentTabForMode('workflow')
        return
      }
      // Ctrl/Cmd + 3 for Organization view
      if ((event.ctrlKey || event.metaKey) && event.key === '3') {
        event.preventDefault()
        setShowWorkflowsOverview(true)
        return
      }
      // Ctrl/Cmd + 6 for workspace minimize
      if ((event.ctrlKey || event.metaKey) && event.key === '6') {
        event.preventDefault()
        toggleWorkspaceMinimize()
        return
      }
      // Ctrl/Cmd + 7 for auto-scroll
      if ((event.ctrlKey || event.metaKey) && event.key === '7') {
        event.preventDefault()
        const chatStore = useChatStore.getState()
        chatStore.setAutoScroll(!chatStore.autoScroll)
        return
      }
      // Ctrl/Cmd + K for the global quick switcher
      if ((event.ctrlKey || event.metaKey) && event.key === 'k') {
        event.preventDefault()
        setQuickSwitcherInitialQuery('')
        setShowQuickSwitcher(prev => !prev)
        return
      }
      // Ctrl/Cmd + N for new chat
      if ((event.ctrlKey || event.metaKey) && event.key === 'n') {
        event.preventDefault()
        if (selectedModeCategory === 'multi-agent' && !showWorkflowsOverview) {
          requestNewMultiAgentChat()
          return
        }
        // Outside chat mode, preserve the existing reset-current-chat behavior.
        if (chatAreaRef.current) {
          chatAreaRef.current.handleNewChat()
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [requestNewMultiAgentChat, restoreMostRecentTabForMode, selectedModeCategory, showWorkflowsOverview, toggleWorkspaceMinimize, setAgentMode, setShowWorkflowsOverview, startNewChat])

  useEffect(() => {
    if (showWorkflowsOverview) {
      setHasOpenedWorkflowsOverview(true)
    }
  }, [showWorkflowsOverview])

  useEffect(() => {
    if (showWorkflowsOverview) {
      setWorkspaceMinimizedForLayout(true)
      return
    }

    if (selectedModeCategory === 'workflow' || selectedModeCategory === 'multi-agent') {
      const { workspaceMinimizedByMode } = useAppStore.getState()
      setWorkspaceMinimizedForLayout(Boolean(workspaceMinimizedByMode?.[selectedModeCategory]))
    }
  }, [selectedModeCategory, showWorkflowsOverview, setWorkspaceMinimizedForLayout])

  useEffect(() => {
    const collapseWorkspaceForPopup = () => {
      const { workspaceMinimized: currentWorkspaceMinimized } = useAppStore.getState()
      if (!currentWorkspaceMinimized && hasOpenWorkspaceCollapsingPopup()) {
        setWorkspaceMinimizedForLayout(true)
      }
    }

    collapseWorkspaceForPopup()

    const observer = new MutationObserver(collapseWorkspaceForPopup)
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['class', 'style', 'role', 'data-workspace-popup', 'data-workspace-collapse-ignore'],
    })

    return () => observer.disconnect()
  }, [setWorkspaceMinimizedForLayout])

  // Responsive split mirroring WorkflowLayout's splitGridCols, keyed off the
  // device tier (OrgHtmlPreviewDevice = 'mobile' | 'desktop'; the UI labels
  // 'desktop' as "Laptop"). Chat is grid col 1, org content is grid col 2.
  //   'mobile' (default) → chat fills (1fr), org content = 480px panel on the right
  //   'desktop' (UI "Laptop") → chat/terminal column is hidden; org content fills
  //     the full width as a single grid column.
  const isMultiAgentLaptopFull = orgHtmlPreviewDevice === 'desktop'
  const multiAgentSplitGridCols =
    isMultiAgentLaptopFull ? 'md:grid-cols-[minmax(0,1fr)]'
      : 'md:grid-cols-[minmax(0,1fr)_480px]'
  const layoutWorkspaceMinimized =
    showWorkflowsOverview
      ? true
      : selectedModeCategory === 'workflow' || selectedModeCategory === 'multi-agent'
        ? Boolean(workspaceMinimizedByMode?.[selectedModeCategory])
        : workspaceMinimized

  useEffect(() => {
    if (layoutWorkspaceMinimized) return
    if (multiAgentRightPanelView !== 'files') return
    if (selectedModeCategory === 'workflow') return

    const workspace = useWorkspaceStore.getState()
    workspace.setActiveFolder(null)
    workspace.fetchFiles(undefined, { force: true, maxDepth: 2 }).catch(error => {
      console.error('[Workspace] Failed to load multi-agent files panel:', error)
    })
  }, [layoutWorkspaceMinimized, multiAgentRightPanelView, selectedModeCategory])

  const toggleMultiAgentPanelMinimize = useCallback(() => {
    setWorkspaceMinimized(!layoutWorkspaceMinimized)
  }, [layoutWorkspaceMinimized, setWorkspaceMinimized])
  // Device-preview toggle (Mobile / Laptop). Lives alongside the panel tabs so it
  // renders in EVERY right-panel view header (Pulse/Goals/Tasks/Files) — critical
  // because Laptop hides the chat/terminal column, so this toggle is the only way
  // back to Mobile to interact with the agent. Persists + broadcasts the choice;
  // App's own listener mirrors it into local state.
  const multiAgentDeviceToggle = (
    <div className="inline-flex flex-none items-center gap-0.5 rounded-lg border border-border bg-muted/70 p-0.5 shadow-sm backdrop-blur-sm">
      {([
        { mode: 'mobile' as const, Icon: Smartphone, label: 'Mobile' },
        { mode: 'desktop' as const, Icon: Laptop, label: 'Laptop' },
      ]).map(({ mode, Icon: DeviceIcon, label }) => (
        <button
          key={mode}
          type="button"
          onClick={() => persistOrgHtmlPreviewDevice(mode)}
          title={`${label} layout`}
          aria-label={`${label} layout`}
          className={`inline-flex h-6 w-6 items-center justify-center rounded transition-colors ${
            orgHtmlPreviewDevice === mode ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <DeviceIcon className="h-3.5 w-3.5" />
        </button>
      ))}
    </div>
  )
  const multiAgentPanelTabs = (
    <div className="inline-flex min-w-0 flex-none items-center gap-1">
      <div className="inline-flex min-w-0 flex-none items-center gap-0.5 rounded-lg border border-border bg-muted/70 p-0.5 shadow-sm backdrop-blur-sm">
        <button
          type="button"
          onClick={() => setMultiAgentRightPanelView('tasks')}
          className={multiAgentPanelTabClass(multiAgentRightPanelView === 'tasks')}
        >
          Tasks
        </button>
        <button
          type="button"
          onClick={() => setMultiAgentRightPanelView('files')}
          className={multiAgentPanelTabClass(multiAgentRightPanelView === 'files')}
        >
          Files
        </button>
      </div>
      {multiAgentDeviceToggle}
    </div>
  )
  const multiAgentPanelCloseButton = (
    <button
      type="button"
      onClick={toggleMultiAgentPanelMinimize}
      title="Hide panel"
      aria-label="Hide panel"
      className="inline-flex h-7 w-7 flex-none items-center justify-center rounded-lg border border-border bg-background/90 text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground"
    >
      <PanelRightClose className="h-4 w-4" />
    </button>
  )

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthWrapper>
        {productSurface === 'video-studio' ? (
          <Suspense fallback={<FileSurfaceFallback />}><VideoStudioSurface /></Suspense>
        ) : productSurface === 'chief-of-staff' ? (
          <Suspense fallback={<FileSurfaceFallback />}><ChiefOfStaffSurface /></Suspense>
        ) : (
        <TooltipProvider>
        <UpdateProgressToast />
        <GlobalHumanFeedbackPrompt />
        <div className="h-screen bg-background flex">
          {/* Main Content Area - WorkflowLayout (workflow mode) or ChatArea (other modes).
              The former left sidebar was removed; its controls now live in the top bar
              (ModePresetBar → WorkspaceTopBarControls). */}
          <div className="flex-1 flex flex-col min-w-0 min-h-0 relative z-10 overflow-hidden">
            {/* Quick Switcher (Ctrl+K) - constrained to the main content area */}
            {showQuickSwitcher && (
              <Suspense fallback={<LazyModalFallback label="Loading switcher..." />}>
                <QuickSwitcher
                  isOpen
                  onClose={() => setShowQuickSwitcher(false)}
                  initialQuery={quickSwitcherInitialQuery}
                />
              </Suspense>
            )}

            {/* Global Mode & Preset Bar - only above middle content area, not sidebars */}
            <ModePresetBar />
            
            {/* Chat Tabs - global navigation for both chat and workflow modes */}
            <ChatTabs
              onNewChat={requestNewMultiAgentChat}
              autoScroll={chatAutoScroll}
              onSubmitOrgCommand={submitMultiAgentPanelCommand}
              onToggleAutoScroll={() => {
                const chatStore = useChatStore.getState()
                chatStore.setAutoScroll(!chatStore.autoScroll)
              }}
            />

            <ConfirmationDialog
              isOpen={showNewChatConfirm}
              onClose={() => setShowNewChatConfirm(false)}
              onConfirm={confirmNewMultiAgentChat}
              title="Start a new chat?"
              message="This stops the current chat session and clears the conversation. This can't be undone."
              confirmText="New Chat"
              cancelText="Cancel"
              type="warning"
            />
            
              <div className="flex-1 min-h-0 overflow-hidden relative">
                {hasOpenedWorkflowsOverview && (
                  <div className={showWorkflowsOverview ? 'h-full' : 'hidden'}>
                    <Suspense fallback={<FileSurfaceFallback />}>
                      <WorkflowsOverviewPage />
                    </Suspense>
                  </div>
                )}
                <div className={!showWorkflowsOverview ? 'h-full' : 'hidden'}>
                  <div className={selectedModeCategory === 'workflow' ? 'h-full' : 'hidden'}>
                    <WorkflowLayout
                      className="h-full"
                      onNewChat={startNewChat}
                    />
                  </div>
                  <div className={selectedModeCategory !== 'workflow' ? 'h-full relative' : 'hidden'}>
                    {layoutWorkspaceMinimized && (
                      <button
                        type="button"
                        onClick={() => setWorkspaceMinimized(false)}
                        title="Show side panel"
                        aria-label="Show side panel"
                        className="absolute right-0 top-1/2 z-30 hidden -translate-y-1/2 flex-col items-center gap-1.5 rounded-l-lg border border-r-0 border-border bg-background/95 py-3 pl-1.5 pr-1 text-muted-foreground shadow-md backdrop-blur-sm transition-colors hover:bg-muted hover:text-foreground md:flex"
                      >
                        <PanelRightOpen className="h-4 w-4" />
                        <span className="[writing-mode:vertical-rl] text-[10px] font-semibold uppercase tracking-wider">Panel</span>
                      </button>
                    )}
                    {/* Mirrors WorkflowLayout: chat (col 1) + org content (col 2)
                        laid out as a responsive grid so the org content resizes with
                        the device selector (Laptop/Tablet/Mobile). The chat rail is
                        always visible; only the user-initiated minimize toggle hides
                        the org panel, at which point the chat flexes to fill. */}
                    <div
                      className={`h-full min-h-0 min-w-0 ${
                        layoutWorkspaceMinimized
                          ? 'flex'
                          : `flex flex-col md:grid ${multiAgentSplitGridCols} md:grid-rows-[minmax(0,1fr)]`
                      }`}
                    >
                      {/* Chat rail = grid col 1, mirroring the workflow. Minimized →
                          flexes to fill the width; Laptop → hidden entirely so the
                          org content takes the full width; otherwise it's the col-1
                          rail. */}
                      <div
                        className={`flex min-w-0 flex-col overflow-hidden bg-background ${
                          layoutWorkspaceMinimized
                            ? 'flex-1'
                            : isMultiAgentLaptopFull
                              ? 'hidden'
                              : 'w-full border-b border-gray-200 dark:border-gray-700 md:col-start-1 md:border-b-0 md:border-r'
                        }`}
                      >
                        <ChatAreaWithObserverId
                          ref={chatAreaRef}
                          onNewChat={startNewChat}
                          previousChatsCompact={!layoutWorkspaceMinimized}
                        />
                      </div>
                      {!layoutWorkspaceMinimized && (
                        <div
                          className={`flex min-w-0 flex-1 flex-col overflow-hidden bg-background ${isMultiAgentLaptopFull ? 'md:col-start-1' : 'md:col-start-2'}`}
                        >
                          {multiAgentRightPanelView === 'files' && (
                            <div className="flex flex-wrap items-center justify-between gap-1 border-b border-border bg-muted/40 px-2 py-2">
                              {multiAgentPanelTabs}
                              {multiAgentPanelCloseButton}
                            </div>
                          )}
                          <div className="min-h-0 flex-1 overflow-hidden">
                            {multiAgentRightPanelView === 'files' ? (
                              <Workspace
                                minimized={false}
                                onToggleMinimize={toggleMultiAgentPanelMinimize}
                              />
                            ) : (
                              <ChiefTasksPanel
                                toolbarLeading={multiAgentPanelTabs}
                                onClosePanel={toggleMultiAgentPanelMinimize}
                              />
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>

            {/* File Content View - overlay when showing file content */}
            <FileContentViewer />
          </div>

        </div>

        </TooltipProvider>
        )}
        </AuthWrapper>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

export default App;
