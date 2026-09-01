export const LARGE_PASTE_MIN_CHARS = 1000
export const LARGE_PASTE_MIN_LINES = 12

export function shouldUsePastedTextAttachment(text: string): boolean {
  if (!text) return false
  const lineCount = text.split(/\r\n|\r|\n/).length
  return text.length >= LARGE_PASTE_MIN_CHARS || lineCount >= LARGE_PASTE_MIN_LINES
}
