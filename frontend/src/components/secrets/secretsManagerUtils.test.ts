import { describe, expect, it } from 'vitest'
import { serverOnlySecretNames, secretsStateFromServer } from './secretsManagerUtils'

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

describe('secretsStateFromServer', () => {
  it('rebuilds the list from the server without needing the browser to have seen it', () => {
    // The reported bug: a key saved through another route was on the server and
    // working, while the browser-local list showed nothing.
    const state = secretsStateFromServer(
      [{ id: 's1', name: 'GEMINI_API_KEY', encrypted_value: 'enc' }],
      [],
    )
    expect(state.secrets).toEqual([
      expect.objectContaining({ id: 's1', name: 'GEMINI_API_KEY', encryptedValue: 'enc' }),
    ])
    expect(state.botEnabledNames.has('GEMINI_API_KEY')).toBe(true)
  })

  it('keeps existing timestamps so the list does not reorder on every refresh', () => {
    const state = secretsStateFromServer(
      [{ name: 'A', encrypted_value: 'new' }],
      [{ id: 'old-id', name: 'A', encryptedValue: 'old', createdAt: 111, updatedAt: 222 }],
    )
    expect(state.secrets[0]).toMatchObject({ id: 'old-id', createdAt: 111, updatedAt: 222 })
    // The server's value wins -- it is the authority on what the secret is.
    expect(state.secrets[0].encryptedValue).toBe('new')
  })

  it('drops a secret the server no longer has', () => {
    const state = secretsStateFromServer([], [{ id: 'x', name: 'GONE', encryptedValue: 'e', createdAt: 1, updatedAt: 1 }])
    expect(state.secrets).toEqual([])
  })
})
