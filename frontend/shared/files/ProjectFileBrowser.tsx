import { useState } from 'react'
import { ChevronRight, File, FileAudio, FileImage, FileText, FileVideo, Folder, FolderOpen } from 'lucide-react'
import './project-file-browser.css'

export interface ProjectFileNode {
  name: string
  path: string
  type: 'file' | 'folder'
  size: number
  children?: ProjectFileNode[]
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(bytes < 10 * 1024 * 1024 ? 1 : 0)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

function FileGlyph({ name }: { name: string }) {
  const extension = name.split('.').pop()?.toLowerCase()
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(extension ?? '')) return <FileImage size={15} />
  if (['mp4', 'mov', 'webm', 'm4v'].includes(extension ?? '')) return <FileVideo size={15} />
  if (['mp3', 'wav', 'm4a', 'aac', 'ogg'].includes(extension ?? '')) return <FileAudio size={15} />
  if (['md', 'txt', 'json', 'csv', 'html', 'pdf', 'doc', 'docx'].includes(extension ?? '')) return <FileText size={15} />
  return <File size={15} />
}

function FileTree({ nodes, depth, expanded, onToggle, onOpen }: {
  nodes: ProjectFileNode[]
  depth: number
  expanded: Set<string>
  onToggle: (path: string) => void
  onOpen: (path: string) => void
}) {
  return <ul className="project-file-tree">{nodes.map((node) => {
    const isFolder = node.type === 'folder'
    const isOpen = expanded.has(node.path)
    return <li key={node.path}>
      <button
        type="button"
        className={`project-file-row ${isFolder ? 'folder' : 'file'}`}
        style={{ paddingLeft: `${10 + depth * 16}px` }}
        onClick={() => isFolder ? onToggle(node.path) : onOpen(node.path)}
        aria-expanded={isFolder ? isOpen : undefined}
        aria-label={`${isFolder ? isOpen ? 'Collapse' : 'Open' : 'Preview'} ${node.name}`}
      >
        {isFolder ? <ChevronRight className={`project-file-chevron ${isOpen ? 'open' : ''}`} size={13} /> : <span className="project-file-chevron" />}
        <span className={`project-file-icon ${isFolder ? 'is-folder' : ''}`}>{isFolder ? isOpen ? <FolderOpen size={15} /> : <Folder size={15} /> : <FileGlyph name={node.name} />}</span>
        <span className="project-file-name">{node.name}</span>
        <span className="project-file-size">{formatBytes(node.size)}</span>
      </button>
      {isFolder && isOpen && <FileTree nodes={node.children ?? []} depth={depth + 1} expanded={expanded} onToggle={onToggle} onOpen={onOpen} />}
    </li>
  })}</ul>
}

export function ProjectFileBrowser({ nodes, onOpen }: { nodes: ProjectFileNode[]; onOpen: (path: string) => void }) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set(nodes.map((node) => node.path)))
  const total = nodes.reduce((sum, node) => sum + node.size, 0)
  return <div className="project-file-browser">
    <div className="project-file-summary"><span>Project files</span><span>{formatBytes(total)}</span></div>
    <FileTree
      nodes={nodes}
      depth={0}
      expanded={expanded}
      onToggle={(path) => setExpanded((current) => {
        const next = new Set(current)
        if (next.has(path)) next.delete(path)
        else next.add(path)
        return next
      })}
      onOpen={onOpen}
    />
  </div>
}
