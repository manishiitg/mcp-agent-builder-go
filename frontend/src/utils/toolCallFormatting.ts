export type ToolCallValueFormat = 'json' | 'shell' | 'text'

export interface FormattedToolCallValue {
  text: string
  format: ToolCallValueFormat
  isError: boolean
}

type JsonRecord = Record<string, unknown>

const MAX_NESTED_JSON_DEPTH = 5

function isRecord(value: unknown): value is JsonRecord {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function tryParseJson(text: string): unknown | undefined {
  const trimmed = text.trim()
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) return undefined

  try {
    return JSON.parse(trimmed)
  } catch {
    return undefined
  }
}

function expandNestedJson(value: unknown, depth = 0): unknown {
  if (depth >= MAX_NESTED_JSON_DEPTH) return value

  if (typeof value === 'string') {
    const parsed = tryParseJson(value)
    return parsed === undefined ? value : expandNestedJson(parsed, depth + 1)
  }

  if (Array.isArray(value)) {
    return value.map(item => expandNestedJson(item, depth + 1))
  }

  if (isRecord(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, expandNestedJson(item, depth + 1)]),
    )
  }

  return value
}

function unwrapMcpTextEnvelope(value: unknown): unknown {
  let current = value

  for (let depth = 0; depth < MAX_NESTED_JSON_DEPTH; depth += 1) {
    if (typeof current === 'string') {
      const parsed = tryParseJson(current)
      if (parsed === undefined) return current
      current = parsed
      continue
    }

    if (!isRecord(current) || !Array.isArray(current.content)) return current

    const textBlocks = current.content
      .filter(isRecord)
      .filter(block => block.type === 'text' && typeof block.text === 'string')
      .map(block => block.text as string)

    if (textBlocks.length === 0) return current
    current = textBlocks.join('\n')
  }

  return current
}

function prettyJson(value: unknown): string {
  return JSON.stringify(expandNestedJson(value), null, 2)
}

function formatTextThatMayBeJson(text: string): FormattedToolCallValue {
  const parsed = tryParseJson(text)
  if (parsed === undefined) return { text, format: 'text', isError: false }
  return { text: prettyJson(parsed), format: 'json', isError: jsonValueIsError(parsed) }
}

function isShellResult(value: unknown): value is JsonRecord {
  if (!isRecord(value)) return false
  return (
    Object.prototype.hasOwnProperty.call(value, 'stdout') ||
    Object.prototype.hasOwnProperty.call(value, 'stderr') ||
    Object.prototype.hasOwnProperty.call(value, 'exit_code') ||
    Object.prototype.hasOwnProperty.call(value, 'execution_time_ms')
  )
}

function jsonValueIsError(value: unknown): boolean {
  if (!isRecord(value)) return false
  if (value.success === false || value.isError === true || value.is_error === true) return true
  if (typeof value.exit_code === 'number' && value.exit_code !== 0) return true
  return typeof value.error === 'string' && value.error.trim().length > 0
}

function shellResultText(result: JsonRecord): string {
  const sections: string[] = []
  const metadata: string[] = []

  if (typeof result.exit_code === 'number') metadata.push(`exit code ${result.exit_code}`)
  if (typeof result.execution_time_ms === 'number') metadata.push(`${result.execution_time_ms}ms`)
  if (metadata.length > 0) sections.push(metadata.join(' · '))

  if (typeof result.stdout === 'string' && result.stdout.length > 0) {
    sections.push(`stdout\n${formatTextThatMayBeJson(result.stdout).text}`)
  }
  if (typeof result.stderr === 'string' && result.stderr.length > 0) {
    sections.push(`stderr\n${formatTextThatMayBeJson(result.stderr).text}`)
  }
  if (typeof result.error === 'string' && result.error.length > 0) {
    sections.push(`error\n${result.error}`)
  }

  return sections.length > 0 ? sections.join('\n\n') : prettyJson(result)
}

export function formatToolCallArguments(value: string): FormattedToolCallValue {
  return formatTextThatMayBeJson(value)
}

export function formatToolCallResult(value: string): FormattedToolCallValue {
  const initiallyParsed = tryParseJson(value)
  if (initiallyParsed === undefined) return { text: value, format: 'text', isError: false }

  const unwrapped = unwrapMcpTextEnvelope(initiallyParsed)
  if (isShellResult(unwrapped)) {
    return {
      text: shellResultText(unwrapped),
      format: 'shell',
      isError: jsonValueIsError(unwrapped),
    }
  }
  if (typeof unwrapped === 'string') {
    return formatTextThatMayBeJson(unwrapped)
  }

  return {
    text: prettyJson(unwrapped),
    format: 'json',
    isError: jsonValueIsError(unwrapped),
  }
}
