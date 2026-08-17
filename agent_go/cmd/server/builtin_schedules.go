package server

import (
	"strings"
)

// Built-in schedules are defined in Go and run for every user unless the user
// has overridden them by adding an entry with the same ID (typically
// enabled:false) to their _users/<id>/multiagent-schedules.json.
//
// Each built-in may also register a pre-fire check in PreFireChecks. The
// scheduler calls the check before starting a session; if the check returns
// false the cron tick is skipped entirely — no LLM session is spawned.

// builtinOrgPulseID is kept as a recognizable constant (not a schedule that
// exists anymore) purely so IsSlashManagedBuiltinSchedule/IsOrgPulseSchedule/
// NormalizeOrgPulseSchedule below can still recognize a schedule ID an
// existing user's persisted _users/<id>/multiagent-schedules.json might still
// carry from before Org Pulse was removed, and degrade it gracefully (see
// NormalizeOrgPulseSchedule) rather than erroring on an unrecognized shape.
const builtinOrgPulseID = "builtin-org-pulse"
const deprecatedAutoEnrichMemoryID = "builtin-auto-enrich-memory"

// DefaultBuiltinSchedules returns the list of product-provided schedules that
// run for every user unless overridden. Empty: Org Pulse (the only builtin
// this ever returned) was goal-alignment-centric end to end and was removed
// entirely alongside the org goals feature, not ported -- there was nothing
// left to salvage by rewriting it once goals were gone. Kept as a function
// (not deleted) since a future built-in schedule may need this registration
// point again.
func DefaultBuiltinSchedules() []WorkflowSchedule {
	return []WorkflowSchedule{}
}

func FindDefaultBuiltinSchedule(scheduleID string) (WorkflowSchedule, bool) {
	for _, sched := range DefaultBuiltinSchedules() {
		if sched.ID == scheduleID {
			return sched, true
		}
	}
	return WorkflowSchedule{}, false
}

func IsDefaultBuiltinSchedule(scheduleID string) bool {
	_, ok := FindDefaultBuiltinSchedule(scheduleID)
	return ok
}

func IsSlashManagedBuiltinSchedule(scheduleID string) bool {
	return scheduleID == builtinOrgPulseID
}

// CanDirectlyToggleBuiltinSchedule separates a safe operational pause/resume
// from editing a product-managed schedule. Org Pulse setup still owns its
// cadence and content, but the operator must be able to stop or resume it
// immediately from the Chief of Staff control.
func CanDirectlyToggleBuiltinSchedule(scheduleID string) bool {
	return scheduleID == builtinOrgPulseID
}

func SlashManagedBuiltinSetupCommand(scheduleID string) string {
	switch scheduleID {
	case builtinOrgPulseID:
		return "/pulse-setup"
	default:
		return ""
	}
}

func SlashManagedBuiltinScheduleLabel(scheduleID string) string {
	switch scheduleID {
	case builtinOrgPulseID:
		return "Daily Org Pulse"
	default:
		return "this managed schedule"
	}
}

func SlashManagedBuiltinError(scheduleID string, action string) string {
	cmd := SlashManagedBuiltinSetupCommand(scheduleID)
	label := SlashManagedBuiltinScheduleLabel(scheduleID)
	if cmd == "" {
		return "Use the setup command in Chief of Staff to " + action + " " + label + "."
	}
	return "Use " + cmd + " in Chief of Staff to " + action + " " + label + "."
}

// IsOrgPulseSchedule reports whether a multi-agent schedule IS the Org Pulse —
// either the canonical built-in (builtin-org-pulse) or a user-created duplicate.
// /pulse-setup is supposed to enable/override builtin-org-pulse, but an agent can
// instead materialize Org Pulse under a fresh id via create_multiagent_schedule
// (the on-disk shape we actually observe). Such a duplicate is still what the
// scheduler runs, so the effective "Org Pulse is on" state must account for it.
// We recognize duplicates by their highly specific Org Pulse signature.
func IsOrgPulseSchedule(sched WorkflowSchedule) bool {
	if sched.ID == builtinOrgPulseID {
		return true
	}
	if mode := strings.TrimSpace(strings.ToLower(sched.Mode)); mode != "" && mode != "multi-agent" {
		return false
	}
	name := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(sched.Name), "-", " "))
	name = strings.Join(strings.Fields(name), " ")
	return name == "org pulse" || name == "daily org pulse" || name == "daily org pulse scan"
}

// NormalizeOrgPulseSchedule keeps Org Pulse runtime behavior on the current
// product-managed prompt sequence while preserving the user's scheduling knobs.
// Older same-ID overrides and duplicate user-created Org Pulse schedules can
// otherwise keep stale Queries/Messages forever and miss new required audit
// steps.
func NormalizeOrgPulseSchedule(sched WorkflowSchedule) WorkflowSchedule {
	if !IsOrgPulseSchedule(sched) {
		return sched
	}

	builtin, ok := FindDefaultBuiltinSchedule(builtinOrgPulseID)
	if !ok {
		return sched
	}

	normalized := builtin
	normalized.ID = sched.ID
	normalized.Enabled = sched.Enabled
	normalized.Mode = "multi-agent"
	normalized.TriggerPayload = sched.TriggerPayload
	normalized.GroupNames = sched.GroupNames
	normalized.ResumePrevious = sched.ResumePrevious

	if strings.TrimSpace(sched.Name) != "" {
		normalized.Name = sched.Name
	}
	if strings.TrimSpace(sched.Description) != "" {
		normalized.Description = sched.Description
	}
	if strings.TrimSpace(sched.ScheduleType) != "" {
		normalized.ScheduleType = sched.ScheduleType
	}
	if strings.TrimSpace(sched.CronExpression) != "" {
		normalized.CronExpression = sched.CronExpression
	}
	if strings.TrimSpace(sched.Timezone) != "" {
		normalized.Timezone = sched.Timezone
	}
	if len(sched.CalendarItems) > 0 {
		normalized.CalendarItems = make([]CalendarScheduleItem, 0, len(sched.CalendarItems))
		for _, item := range sched.CalendarItems {
			item.Messages = nil
			normalized.CalendarItems = append(normalized.CalendarItems, item)
		}
	}

	return normalized
}

func NormalizeBuiltinSchedule(sched WorkflowSchedule) WorkflowSchedule {
	sched = NormalizeOrgPulseSchedule(sched)
	return sched
}

// MergeBuiltinSchedules appends built-in schedules that the user has not
// overridden. Matching is by ID — a user entry with the same ID supplies the
// scheduling knobs (enabled, cron/timezone, calendar items). Recognized Org
// Pulse overrides/duplicates are normalized to the current product-managed
// content so stale persisted messages do not shadow new built-in steps.
func MergeBuiltinSchedules(userSchedules []WorkflowSchedule) []WorkflowSchedule {
	existing := make(map[string]struct{}, len(userSchedules))
	out := make([]WorkflowSchedule, 0, len(userSchedules)+len(DefaultBuiltinSchedules()))
	for _, s := range userSchedules {
		if s.ID == deprecatedAutoEnrichMemoryID {
			continue
		}
		normalized := NormalizeBuiltinSchedule(s)
		existing[normalized.ID] = struct{}{}
		out = append(out, normalized)
	}
	for _, b := range DefaultBuiltinSchedules() {
		if _, overridden := existing[b.ID]; !overridden {
			out = append(out, b)
		}
	}
	return out
}

// PreFireCheck is a gating function called by the scheduler before a built-in
// schedule fires. Returning false skips this tick entirely — no LLM session.
type PreFireCheck func(userID string) bool

// PreFireChecks maps built-in schedule IDs to their gating functions.
var PreFireChecks = map[string]PreFireCheck{}
