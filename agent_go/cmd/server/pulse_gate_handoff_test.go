package server

import "testing"

// The Gate handoff proves "this handoff belongs to this run". Confida's 11:00
// run wrote the correct pulse_run_id under data-pulse-run — the attribute the
// .pulse-fixer-recovery ledger nested in this same element uses — instead of
// data-pulse-run-id. The contract rejected it twice, the Pulse turn fell into a
// recovery session, and the run finished "partial", over a one-word alias while
// the id itself was right.
func TestPulseGateHandoffAcceptsRunIDUnderEitherAttribute(t *testing.T) {
	const runID = "schedule-cron--d25999f9_1785303047299675000"

	for name, html := range map[string]string{
		"canonical data-pulse-run-id": `<div id="pulse-agent-handoff" data-pulse-run-id="` + runID + `" hidden></div>`,
		// The shape confida actually produced.
		"ledger-style data-pulse-run": `<div id="pulse-agent-handoff" hidden data-contract-version="1.0.15" ` +
			`data-execution-state="gate_recorded" data-pulse-run="` + runID + `"></div>`,
		"id carried in element text": `<div id="pulse-agent-handoff" hidden><span>run ` + runID + `</span></div>`,
	} {
		t.Run(name, func(t *testing.T) {
			if !pulseGateHandoffContainsRunID(html, runID) {
				t.Fatalf("handoff carries the current run id but was rejected:\n%s", html)
			}
		})
	}
}

// The check still has to mean something: a handoff left over from an earlier
// run carries no attribute with the current id, and must not pass.
func TestPulseGateHandoffRejectsStaleAndMissingRunIDs(t *testing.T) {
	const runID = "schedule-cron--d25999f9_1785303047299675000"

	for name, html := range map[string]string{
		"stale run id": `<div id="pulse-agent-handoff" data-pulse-run-id="schedule-cron--OLD_1700000000000000000" hidden></div>`,
		"no run id at all": `<div id="pulse-agent-handoff" hidden data-contract-version="1.0.15" ` +
			`data-execution-state="gate_recorded"></div>`,
		"right id but wrong element": `<div id="something-else" data-pulse-run-id="` + runID + `"></div>`,
		"empty document":             ``,
	} {
		t.Run(name, func(t *testing.T) {
			if pulseGateHandoffContainsRunID(html, runID) {
				t.Fatalf("handoff without the current run id was accepted:\n%s", html)
			}
		})
	}
}

// An empty pulse_run_id must never validate, or every handoff would pass.
func TestPulseGateHandoffRejectsEmptyRunID(t *testing.T) {
	if pulseGateHandoffContainsRunID(`<div id="pulse-agent-handoff" data-pulse-run-id=""></div>`, "") {
		t.Fatal("an empty pulse_run_id must not satisfy the contract")
	}
}
