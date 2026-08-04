import { lazy, Suspense, useCallback, useEffect, useId, useState, type ComponentType, type ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize'
import { visit } from 'unist-util-visit'
import type { Element as HastElement } from 'hast'
import './chat-renderer.css'

const SyntaxHighlightedCode = lazy(() => import('./SyntaxHighlightedCode'))

let mermaidModule: Promise<typeof import('mermaid').default> | null = null
function loadMermaid() {
  mermaidModule ??= import('mermaid').then((module) => module.default)
  return mermaidModule
}

export function stabilizeStreamingMarkdown(text: string): string {
  let output = text
  for (const token of ['**', '`', '*', '_']) {
    const parts = output.split(token)
    if (parts.length > 1 && (parts.length - 1) % 2 === 1) output = parts.slice(0, -1).join(token)
  }
  return output.replace(/\n[#>\-*+]+[ \t]*$/, '')
}

const colorSpanSchema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    span: [...(defaultSchema.attributes?.span ?? []), 'style'],
  },
}

function restrictSpanStyleToColor() {
  return (tree: HastElement) => {
    visit(tree, 'element', (node: HastElement) => {
      if (node.tagName !== 'span' || typeof node.properties?.style !== 'string') return
      const match = /color\s*:\s*([#a-zA-Z0-9(),.\s%]+)/i.exec(node.properties.style)
      if (match) node.properties.style = `color: ${match[1].trim()}`
      else delete node.properties.style
    })
  }
}

function MermaidDiagram({ content, theme }: { content: string; theme: 'light' | 'dark' }) {
  const reactId = useId()
  const [svg, setSvg] = useState('')
  const [error, setError] = useState('')
  const render = useCallback(async () => {
    try {
      const mermaid = await loadMermaid()
      mermaid.initialize({ startOnLoad: false, theme: theme === 'dark' ? 'dark' : 'default', securityLevel: 'strict' })
      const rendered = await mermaid.render(`chat-mermaid-${reactId.replace(/[^a-z0-9]/gi, '')}`, content)
      setSvg(rendered.svg)
      setError('')
    } catch (reason) {
      setSvg('')
      setError(reason instanceof Error ? reason.message : 'Could not render diagram')
    }
  }, [content, reactId, theme])
  useEffect(() => { void render() }, [render])
  if (error) return <div className="shared-mermaid-state is-error">Diagram could not be rendered: {error}</div>
  if (!svg) return <div className="shared-mermaid-state">Rendering diagram…</div>
  return <div className="shared-mermaid" dangerouslySetInnerHTML={{ __html: svg }} />
}

export interface ChatMarkdownLinkProps {
  href?: string
  children?: ReactNode
}

export interface ChatMarkdownProps {
  text: string
  streaming?: boolean
  theme?: 'light' | 'dark'
  linkComponent?: ComponentType<ChatMarkdownLinkProps>
  workspaceLinkRoots?: readonly string[]
}

interface MarkdownNode {
  type: string
  value?: string
  children?: MarkdownNode[]
  url?: string
}

function remarkWorkspaceLinks({ roots }: { roots: readonly string[] }) {
  const escapedRoots = roots.map((root) => root.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).filter(Boolean)
  if (escapedRoots.length === 0) return () => undefined
  const pathPattern = new RegExp(`(^|[\\s(])((?:${escapedRoots.join('|')})\\/[^\\s<>()\\[\\]{}'"\\x60]+)`, 'g')
  return (tree: MarkdownNode) => {
    function transform(node: MarkdownNode) {
      if (!node.children || ['link', 'linkReference', 'definition', 'code', 'inlineCode', 'html'].includes(node.type)) return
      const nextChildren: MarkdownNode[] = []
      for (const child of node.children) {
        if (child.type !== 'text' || !child.value) {
          transform(child)
          nextChildren.push(child)
          continue
        }
        let cursor = 0
        pathPattern.lastIndex = 0
        for (const match of child.value.matchAll(pathPattern)) {
          const prefix = match[1]
          const path = match[2]
          const start = (match.index ?? 0) + prefix.length
          if (start > cursor) nextChildren.push({ type: 'text', value: child.value.slice(cursor, start) })
          nextChildren.push({ type: 'link', url: path, children: [{ type: 'text', value: path }] })
          cursor = start + path.length
        }
        if (cursor === 0) nextChildren.push(child)
        else if (cursor < child.value.length) nextChildren.push({ type: 'text', value: child.value.slice(cursor) })
      }
      node.children = nextChildren
    }
    transform(tree)
  }
}

function ExternalLink({ href, children }: ChatMarkdownLinkProps) {
  return <a href={href} target="_blank" rel="noreferrer">{children}</a>
}

export function ChatMarkdown({ text, streaming = false, theme = 'light', linkComponent: Link = ExternalLink, workspaceLinkRoots = [] }: ChatMarkdownProps) {
  const renderedText = streaming ? stabilizeStreamingMarkdown(text) : text
  return <div className="shared-markdown">
    <ReactMarkdown
      remarkPlugins={[remarkGfm, [remarkWorkspaceLinks, { roots: workspaceLinkRoots }]]}
      rehypePlugins={[rehypeRaw, [rehypeSanitize, colorSpanSchema], restrictSpanStyleToColor]}
      components={{
        a: ({ href, children }) => <Link href={href}>{children}</Link>,
        pre: ({ children }) => <>{children}</>,
        code: (props) => {
          const { className, children, ...rest } = props as { className?: string; children?: ReactNode; node?: unknown }
          const match = /language-([\w-]+)/.exec(className || '')
          const value = String(children)
          const isInline = !match && !value.includes('\n')
          if (isInline) return <code className="shared-inline-code" {...rest}>{children}</code>
          const code = value.replace(/\n$/, '')
          const language = match?.[1] ?? 'text'
          if (language.toLowerCase() === 'mermaid') return <MermaidDiagram content={code} theme={theme} />
          if (['text', 'txt', 'plain', 'plaintext', 'terminal'].includes(language.toLowerCase())) {
            return <div className="shared-code-block"><span className="shared-code-language">{language}</span><pre>{code}</pre></div>
          }
          return <div className="shared-code-block"><span className="shared-code-language">{language}</span><Suspense fallback={<pre>{code}</pre>}><SyntaxHighlightedCode code={code} language={language} isDark={theme === 'dark'} /></Suspense></div>
        },
      }}
    >{renderedText}</ReactMarkdown>
    {streaming && <span className="shared-stream-cursor" aria-hidden="true" />}
  </div>
}

export interface SharedToolCall {
  id?: string
  tool: string
  args?: string
  result?: string
  error?: string
  status?: 'running' | 'completed' | 'failed'
  durationMs?: number
}

export function ToolCallSummary({ calls, defaultExpanded = false }: { calls: SharedToolCall[]; defaultExpanded?: boolean }) {
  const [open, setOpen] = useState(defaultExpanded)
  const [openIndex, setOpenIndex] = useState<number | null>(null)
  if (calls.length === 0) return null
  return <div className="shared-tool-summary">
    <button type="button" className="shared-tool-toggle" onClick={() => setOpen((value) => !value)} aria-expanded={open}>
      <span className="shared-tool-count">{calls.length}</span> tool{calls.length === 1 ? '' : 's'} <span className="shared-tool-caret">{open ? '▲' : '▼'}</span>
    </button>
    {open && <div className="shared-tool-list">{calls.map((call, index) => {
      const status = call.status ?? (call.error ? 'failed' : call.result === undefined ? 'running' : 'completed')
      const detail = call.error ? `Error: ${call.error}` : call.result
      return <div className="shared-tool-call" key={call.id ?? `${call.tool}-${index}`}>
        <button type="button" className="shared-tool-call-row" onClick={() => detail && setOpenIndex((value) => value === index ? null : index)}>
          <i className={`shared-tool-status ${status}`} />
          <span className="shared-tool-label"><span className="shared-tool-name">{call.tool}</span>{call.args && <span className="shared-tool-args">{call.args}</span>}</span>
          <span className="shared-tool-duration">{status === 'running' ? 'Running' : call.durationMs === undefined ? status === 'failed' ? 'Failed' : 'Done' : `${call.durationMs} ms`}</span>
        </button>
        {openIndex === index && detail && <pre className="shared-tool-response">{detail}</pre>}
      </div>
    })}</div>}
  </div>
}
