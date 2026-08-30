// Clipboard image extraction for the chat composer.
//
// Extracted from ChatInput.tsx so it can be tested directly: importing
// ChatInput pulls the whole app graph (stores -> api -> workspace profile) and
// fails at import time under vitest, which is why this logic previously had no
// coverage — and why the duplicate-attachment bug below shipped unnoticed.

const CLIPBOARD_IMAGE_EXTENSIONS: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/webp': 'webp',
  'image/gif': 'gif',
  'image/bmp': 'bmp',
  'image/svg+xml': 'svg',
  'image/tiff': 'tiff',
}

const CLIPBOARD_IMAGE_FILE_EXTENSION_PATTERN = /\.(png|jpe?g|webp|gif|bmp|svg|tiff?)$/i

export const isClipboardImageFile = (file: File): boolean => {
  return file.type.toLowerCase().startsWith('image/')
    || CLIPBOARD_IMAGE_FILE_EXTENSION_PATTERN.test(file.name)
}

export const clipboardImageExtension = (file: File): string => {
  const mimeExtension = CLIPBOARD_IMAGE_EXTENSIONS[file.type.toLowerCase()]
  if (mimeExtension) return mimeExtension

  const nameExtension = file.name.match(/\.([a-z0-9]{1,8})$/i)?.[1]?.toLowerCase()
  return nameExtension || 'png'
}

export const pastedImageFileName = (file: File, index: number): string => {
  const timestamp = new Date()
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d{3}Z$/, 'Z')
  const suffix = index > 0 ? `-${index + 1}` : ''
  return `pasted-image-${timestamp}${suffix}.${clipboardImageExtension(file)}`
}

export const getClipboardImageFiles = (clipboardData?: DataTransfer | null): File[] => {
  if (!clipboardData) return []

  const images: File[] = []
  const seen = new Set<string>()
  const addFile = (file: File | null) => {
    if (!file || !isClipboardImageFile(file)) return
    // Deliberately NOT keyed on lastModified. `.files` and `.items` are two
    // views of the SAME clipboard payload, but `item.getAsFile()` mints a new
    // File whose lastModified is the moment it was called — so one pasted
    // image produced two different keys, defeated this dedup, and was attached
    // twice (observed live 2026-08-19: pasted-image-…065103Z.png alongside
    // pasted-image-…065103Z-2.png, the -2 being the collision rename).
    const key = `${file.name}:${file.type}:${file.size}`
    if (seen.has(key)) return
    seen.add(key)
    images.push(file)
  }

  // Read ONE view, not both. `.files` is the modern representation and already
  // contains every pasted file; `.items` is the fallback for clipboards that
  // only populate the item list. Merging them is what created the duplicate,
  // and no dedup key is fully reliable across the two: a browser typically
  // names every clipboard image "image.png", so name, type and size can all
  // legitimately collide between two genuinely different images pasted
  // together. Picking a single source removes the ambiguity instead of trying
  // to resolve it after the fact.
  const fromFiles = Array.from(clipboardData.files || [])
  if (fromFiles.length > 0) {
    fromFiles.forEach(addFile)
  } else {
    Array.from(clipboardData.items || []).forEach(item => {
      if (item.kind === 'file') {
        addFile(item.getAsFile())
      }
    })
  }

  return images.map((file, index) => new File([file], pastedImageFileName(file, index), {
    type: file.type || 'image/png',
    lastModified: Date.now(),
  }))
}
