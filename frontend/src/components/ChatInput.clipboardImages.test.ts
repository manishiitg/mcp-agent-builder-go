import { describe, it, expect } from 'vitest'
import { getClipboardImageFiles } from './clipboardImages'

// Minimal DataTransfer stand-in. jsdom's own DataTransfer cannot be populated
// with files, and the whole point here is controlling exactly what `.files` and
// `.items` each report for the same paste.
function clipboard(opts: { files?: File[]; items?: File[] }): DataTransfer {
  const items = (opts.items || []).map(file => ({
    kind: 'file' as const,
    type: file.type,
    getAsFile: () => file,
  }))
  return {
    files: opts.files || [],
    items,
  } as unknown as DataTransfer
}

const png = (name: string, lastModified: number) =>
  new File([new Uint8Array([1, 2, 3])], name, { type: 'image/png', lastModified })

describe('getClipboardImageFiles', () => {
  // The live bug, 2026-08-19: one pasted screenshot arrived as TWO attachments
  // (pasted-image-…065103Z.png and pasted-image-…065103Z-2.png). `.files` and
  // `.items` are two views of the same clipboard payload, but
  // `item.getAsFile()` mints a File whose lastModified is the call time, so the
  // old dedup key (which included lastModified) saw them as distinct.
  it('attaches a single image once when it appears in both files and items', () => {
    const inFiles = png('image.png', 1000)
    const inItems = png('image.png', 2000) // same image, different lastModified
    const result = getClipboardImageFiles(clipboard({ files: [inFiles], items: [inItems] }))
    expect(result).toHaveLength(1)
  })

  // Guards the other half of the fix: reading only `.files` must not drop
  // images from a clipboard that populates just the item list.
  it('falls back to items when files is empty', () => {
    const result = getClipboardImageFiles(clipboard({ files: [], items: [png('image.png', 1000)] }))
    expect(result).toHaveLength(1)
  })

  // Two genuinely different images pasted together must both survive — the
  // dedup must not over-collapse. Distinct sizes, since a browser names every
  // clipboard image "image.png".
  it('keeps two genuinely different images from one paste', () => {
    const a = new File([new Uint8Array([1])], 'image.png', { type: 'image/png' })
    const b = new File([new Uint8Array([1, 2, 3, 4])], 'image.png', { type: 'image/png' })
    const result = getClipboardImageFiles(clipboard({ files: [a, b] }))
    expect(result).toHaveLength(2)
    // and they are renamed distinctly, so neither overwrites the other
    expect(new Set(result.map(f => f.name)).size).toBe(2)
  })

  it('ignores non-image clipboard content', () => {
    const txt = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    expect(getClipboardImageFiles(clipboard({ files: [txt] }))).toHaveLength(0)
  })

  it('handles a null clipboard', () => {
    expect(getClipboardImageFiles(null)).toHaveLength(0)
  })
})
