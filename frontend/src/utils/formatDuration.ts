// Utility function to convert duration to human readable format
export const formatDuration = (duration: string | number): string => {
  if (typeof duration === 'string') {
    // Parse duration string like "23042" (nanoseconds) or "2.3s"
    const num = parseFloat(duration)
    if (isNaN(num)) return duration
    
    // If it's a large number, assume it's nanoseconds
    if (num > 1000) {
      const seconds = num / 1000000000
      if (seconds < 1) {
        return `${Math.round(num / 1000000)}ms`
      } else if (seconds < 60) {
        return `${seconds.toFixed(1)}s`
      } else {
        const minutes = Math.floor(seconds / 60)
        const remainingSeconds = seconds % 60
        return `${minutes}m ${remainingSeconds.toFixed(0)}s`
      }
    } else {
      return duration
    }
  } else {
    // Handle number input
    const seconds = duration / 1000000000
    if (seconds < 1) {
      return `${Math.round(duration / 1000000)}ms`
    } else if (seconds < 60) {
      return `${seconds.toFixed(1)}s`
    } else {
      const minutes = Math.floor(seconds / 60)
      const remainingSeconds = seconds % 60
      return `${minutes}m ${remainingSeconds.toFixed(0)}s`
    }
  }
} 