package server

import "strings"

// scheduleModeOrDefault and scheduleTimezoneOrDefault are scheduler-domain
// normalization helpers. They used to live beside the removed chat schedule
// tools even though the scheduler and its HTTP routes also depend on them.
func scheduleModeOrDefault(mode string) string {
	switch strings.TrimSpace(mode) {
	case "multi-agent":
		return "multi-agent"
	default:
		return "workshop"
	}
}

func scheduleTimezoneOrDefault(timezone string) string {
	if timezone == "" {
		return "UTC"
	}
	return timezone
}
