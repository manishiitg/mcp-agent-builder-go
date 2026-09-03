import type { ReactNode } from 'react'
import type { WorkspacePresentation } from './presentationData'
import { getPresentationRenderer } from './presentationRegistry'

export type PresentationRendererProps = {
  presentation: WorkspacePresentation
  workspacePath: string
  // Product surfaces can request playback for a presentation that has just
  // arrived from a tool event. Renderers that do not handle media simply
  // ignore this optional hint.
  autoPlay?: boolean
}

export function PresentationRenderer({ presentation, workspacePath, autoPlay, fallback }: PresentationRendererProps & { fallback?: ReactNode }) {
  const Renderer = getPresentationRenderer(presentation.kind)
  if (!Renderer) return fallback ?? null
  return <Renderer presentation={presentation} workspacePath={workspacePath} autoPlay={autoPlay} />
}
