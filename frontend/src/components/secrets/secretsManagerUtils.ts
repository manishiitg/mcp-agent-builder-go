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
