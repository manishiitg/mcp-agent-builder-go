package schedulepolicy

import "testing"

func TestPulsePolicyValidation(t *testing.T) {
	for _, tc := range []struct {
		mode, reason string
		valid        bool
	}{
		{"", "Inherited", false}, {"inherit", "Default", false}, {"basic", " \n\t", false},
		{"off", "Owner intentionally disabled post-run stewardship", true},
		{"basic", "Routine queue processing; retain backup and summary", true},
		{"full", "Weekly review of accumulated route evidence", true},
	} {
		if err := ValidatePulse(tc.mode, tc.reason); (err == nil) != tc.valid {
			t.Errorf("%q: %v", tc.mode, err)
		}
	}
}

func TestPulseStampRequiresEverySchedule(t *testing.T) {
	for _, raw := range []string{
		`{"schedules":[{"id":"old","enabled":false}]}`,
		`{"schedules":[{"id":"calendar","schedule_type":"calendar","pulse_mode":"basic"}]}`,
		`{"schedules":[{"pulse_mode":"basic","pulse_mode_reason":"Routine"},{"pulse_mode":"full"}]}`,
	} {
		if err := ValidatePulseStamp([]byte(raw), ExplicitPulseContractVersion); err == nil {
			t.Errorf("incomplete stamp accepted: %s", raw)
		}
		if err := ValidatePulseStamp([]byte(raw), "1.0.40"); err != nil {
			t.Errorf("older migration blocked: %v", err)
		}
	}
	for _, raw := range []string{`{"schedules":[]}`, `{"schedules":[{"pulse_mode":"basic","pulse_mode_reason":"Routine queue processing"}]}`} {
		if err := ValidatePulseStamp([]byte(raw), ExplicitPulseContractVersion); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPulseContractVersionBoundary(t *testing.T) {
	for _, version := range []string{"", "1.0.40", "invalid"} {
		if RequiresExplicitPulse(version) {
			t.Errorf("unexpected new contract: %s", version)
		}
	}
	for _, version := range []string{"1.0.41", "1.0.42", "1.1.0", "2.0.0"} {
		if !RequiresExplicitPulse(version) {
			t.Errorf("missed contract: %s", version)
		}
	}
}
