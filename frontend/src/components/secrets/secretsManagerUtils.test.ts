import { describe, expect, it } from 'vitest'
import { serverOnlySecretNames } from './secretsManagerUtils'

describe('serverOnlySecretNames', () => {
  it('surfaces a secret the server holds that this browser never stored', () => {
    // The reported bug: a key saved through the agent's own secret tool was on
    // the server and working, while the modal showed an empty list.
    expect(serverOnlySecretNames([{ name: 'GEMINI_API_KEY' }], [])).toEqual(['GEMINI_API_KEY'])
  })

  it('does not duplicate a secret that is already in the editable list', () => {
    expect(
      serverOnlySecretNames(
        [{ name: 'GEMINI_API_KEY' }, { name: 'PEXELS_API_KEY' }],
        [{ name: 'GEMINI_API_KEY' }],
      ),
    ).toEqual(['PEXELS_API_KEY'])
  })

  it('returns nothing when the server knows only what this browser already shows', () => {
    expect(serverOnlySecretNames([{ name: 'FAL_KEY' }], [{ name: 'FAL_KEY' }])).toEqual([])
  })

  it('drops duplicates and blanks rather than rendering empty rows', () => {
    expect(
      serverOnlySecretNames([{ name: 'A' }, { name: 'A' }, { name: '' }], []),
    ).toEqual(['A'])
  })
})
