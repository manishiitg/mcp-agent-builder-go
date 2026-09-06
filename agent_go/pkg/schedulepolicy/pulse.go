// Package schedulepolicy defines the shared schedule Pulse authoring contract.
package schedulepolicy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const ExplicitPulseContractVersion = "1.0.41"

func RequiresExplicitPulse(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return false
	}
	var values [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return false
		}
		values[i] = n
	}
	return values[0] > 1 || (values[0] == 1 && (values[1] > 0 || values[2] >= 41))
}

func ValidatePulse(mode, reason string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "off", "basic", "full":
	default:
		return fmt.Errorf("pulse_mode is required and must be off, basic, or full; choose explicitly for this schedule")
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("pulse_mode_reason is required; explain this schedule's purpose, frequency and review needs")
	}
	return nil
}

// ValidatePulseStamp checks the persisted schedules before acknowledging the
// migration. It intentionally includes disabled and calendar schedules.
func ValidatePulseStamp(content []byte, target string) error {
	if !RequiresExplicitPulse(target) {
		return nil
	}
	var manifest struct {
		Schedules []struct {
			ID     string `json:"id"`
			Mode   string `json:"pulse_mode"`
			Reason string `json:"pulse_mode_reason"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return err
	}
	for i, s := range manifest.Schedules {
		if err := ValidatePulse(s.Mode, s.Reason); err != nil {
			return fmt.Errorf("schedules[%d] (%s): %w", i, s.ID, err)
		}
	}
	return nil
}
