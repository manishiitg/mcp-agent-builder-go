/**
 * Duration formatting utilities for event components
 * 
 * Go's time.Duration serializes to JSON as nanoseconds (integer)
 * Frontend needs to convert nanoseconds to human-readable format
 */

/**
 * Formats a duration in nanoseconds to a human-readable string
 * @param durationNs - Duration in nanoseconds (from Go time.Duration)
 * @returns Formatted duration string (e.g., "1.2s", "150ms", "2.5m")
 */
export function formatDuration(durationNs: number): string {
  if (!durationNs || durationNs <= 0) {
    return '0ms'
  }

  // Convert nanoseconds to milliseconds
  const durationMs = durationNs / 1000000

  if (durationMs < 1) {
    // Less than 1ms, show in microseconds
    const durationUs = durationNs / 1000
    return `${Math.round(durationUs)}μs`
  } else if (durationMs < 1000) {
    // Less than 1 second, show in milliseconds
    return `${Math.round(durationMs)}ms`
  } else if (durationMs < 60000) {
    // Less than 1 minute, show in seconds
    return `${(durationMs / 1000).toFixed(1)}s`
  } else {
    // 1 minute or more, show in minutes
    return `${(durationMs / 60000).toFixed(1)}m`
  }
}

/**
 * Formats a duration in nanoseconds to milliseconds (for display in compact mode)
 * @param durationNs - Duration in nanoseconds (from Go time.Duration)
 * @returns Duration in milliseconds as string
 */
export function formatDurationMs(durationNs: number): string {
  if (!durationNs || durationNs <= 0) {
    return '0ms'
  }

  const durationMs = Math.round(durationNs / 1000000)
  return `${durationMs}ms`
}

/**
 * Formats a duration in nanoseconds to seconds (for display in compact mode)
 * @param durationNs - Duration in nanoseconds (from Go time.Duration)
 * @returns Duration in seconds as string
 */
export function formatDurationSeconds(durationNs: number): string {
  if (!durationNs || durationNs <= 0) {
    return '0s'
  }

  const durationSeconds = (durationNs / 1000000000).toFixed(1)
  return `${durationSeconds}s`
}

/**
 * Formats a duration in nanoseconds to a compact format suitable for inline display
 * @param durationNs - Duration in nanoseconds (from Go time.Duration)
 * @returns Compact duration string
 */
export function formatDurationCompact(durationNs: number): string {
  if (!durationNs || durationNs <= 0) {
    return '0ms'
  }

  const durationMs = durationNs / 1000000

  if (durationMs < 1000) {
    return `${Math.round(durationMs)}ms`
  } else if (durationMs < 60000) {
    return `${(durationMs / 1000).toFixed(1)}s`
  } else {
    return `${(durationMs / 60000).toFixed(1)}m`
  }
}
