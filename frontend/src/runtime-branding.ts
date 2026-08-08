export type RuntimeBrandingConfig = {
  appName?: unknown
  faviconUrl?: unknown
}

function hasControlCharacters(value: string): boolean {
  return Array.from(value).some((character) => {
    const codePoint = character.codePointAt(0)
    return codePoint !== undefined && (codePoint <= 0x1f || codePoint === 0x7f)
  })
}

function safeRuntimeAssetPath(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const path = value.trim()
  if (!path.startsWith('/') || path.startsWith('//') || hasControlCharacters(path)) {
    return null
  }
  return path
}

export function applyRuntimeBranding(config: RuntimeBrandingConfig | null | undefined, doc: Document = document) {
  if (!config || typeof config !== 'object') return

  if (typeof config.appName === 'string') {
    const appName = config.appName.trim()
    if (appName && appName.length <= 120 && !hasControlCharacters(appName)) {
      doc.title = appName
    }
  }

  const faviconUrl = safeRuntimeAssetPath(config.faviconUrl)
  if (!faviconUrl) return

  let favicon = doc.querySelector<HTMLLinkElement>('link[rel~="icon"]')
  if (!favicon) {
    favicon = doc.createElement('link')
    favicon.rel = 'icon'
    doc.head.appendChild(favicon)
  }
  favicon.type = faviconUrl.toLowerCase().endsWith('.svg') ? 'image/svg+xml' : 'image/png'
  favicon.href = faviconUrl
}
