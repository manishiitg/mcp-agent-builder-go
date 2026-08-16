import type { ReactNode } from 'react'
import type { WorkspacePresentation } from './presentationData'
import { getPresentationRenderer } from './presentationRegistry'

export type PresentationRendererProps = {
  presentation: WorkspacePresentation
  workspacePath: string
}

export function PresentationRenderer({ presentation, workspacePath, fallback }: PresentationRendererProps & { fallback?: ReactNode }) {
  const Renderer = getPresentationRenderer(presentation.kind)
  if (!Renderer) return fallback ?? null
  return <Renderer presentation={presentation} workspacePath={workspacePath} />
}
