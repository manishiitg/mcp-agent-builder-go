/**
 * The editable secrets list is localStorage-backed, so it only ever contained
 * secrets added through this modal in this browser. Anything saved another way
 * -- an agent's own secret tools, a product's secrets dropdown, or the same
 * user on another machine -- exists on the server and was simply invisible
 * here, which reads as "my key was never saved" rather than "this list is
 * partial".
 *
 * The server returns names only, never values, which is why these entries are
 * surfaced as read-only rather than merged into the editable list.
 */
export function serverOnlySecretNames(
  storedUserSecrets: { name: string }[],
  localSecrets: { name: string }[],
): string[] {
  const local = new Set(localSecrets.map((secret) => secret.name))
  const seen = new Set<string>()
  return storedUserSecrets
    .map((stored) => stored.name)
    .filter((name) => {
      if (!name || local.has(name) || seen.has(name)) return false
      seen.add(name)
      return true
    })
    .sort()
}

/** Mirrors StoredSecret in useSecretsStore, declared here so this stays a
 *  dependency-free module the store can import without a cycle. */
type StoredSecret = {
  id: string
  name: string
  encryptedValue: string
  createdAt: number
  updatedAt: number
}

// The server is the source of truth for which secrets exist and what they
// contain. Local timestamps are preserved where we already had the secret so
// ordering does not jump around on refresh, but nothing here depends on the
// browser having seen a secret before -- that dependency is exactly what made
// a key saved elsewhere look like it had never been saved.
export function secretsStateFromServer(
  rows: { id?: string; name: string; encrypted_value?: string }[],
  existing: StoredSecret[],
) {
  const previous = new Map(existing.map((secret) => [secret.name, secret]))
  const secrets: StoredSecret[] = rows.map((row) => {
    const prior = previous.get(row.name)
    return {
      id: row.id || prior?.id || row.name,
      name: row.name,
      encryptedValue: row.encrypted_value ?? prior?.encryptedValue ?? '',
      createdAt: prior?.createdAt ?? 0,
      updatedAt: prior?.updatedAt ?? 0,
    }
  })
  return {
    secrets,
    storedUserSecrets: rows.map((row) => ({ name: row.name })),
    botEnabledNames: new Set(rows.map((row) => row.name)),
  }
}
