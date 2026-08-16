package server

import "testing"

// PLAT-113. Auto-notifications are supposed to queue while a turn is running and
// be delivered afterwards in one batch. The gate that decides this read
// sessionBusy — a user-facing display flag that is deliberately never set for
// workflow turns — so during a scheduled workflow run every background-agent
// completion skipped the queue, called executeSyntheticTurn, registered itself
// as a running execution, and then blocked on the input lane the workflow turn
// was holding. 25 of them piled up behind one 5-hour turn until the idle-wait
// watchdog killed a run that was working correctly.
//
// The lane is the authority: a turn occupies the session exactly when it holds
// that mutex.

func TestWorkflowTurnQueuesAutoNotificationsEvenWithoutTheBusyFlag(t *testing.T) {
	api := &StreamingAPI{}
	const sessionID = "schedule-cron--5227790a_workflow"

	// A workflow turn never sets sessionBusy — this is the exact state the old
	// gate misread as "idle".
	if api.isSessionBusy(sessionID) {
		t.Fatal("precondition: a workflow turn must not set the display busy flag")
	}

	release := api.lockSessionInputLane(sessionID)
	defer release()

	if !api.sessionTurnInProgress(sessionID) {
		t.Fatal("a turn holding the lane must report the session as occupied")
	}
	if !api.isSessionBusyForAutoNotification(sessionID) {
		t.Fatal("auto-notification must queue while a workflow turn holds the lane; " +
			"executing instead is what blocked 25 synthetic turns behind one turn")
	}
}

// The old gate is still the reason this used to fail: sessionBusy alone reports
// idle for the same session and state.
func TestDisplayBusyFlagAloneMisreadsAWorkflowTurn(t *testing.T) {
	api := &StreamingAPI{}
	const sessionID = "schedule-cron--5227790a_workflow"

	release := api.lockSessionInputLane(sessionID)
	defer release()

	if api.isSessionBusy(sessionID) {
		t.Fatal("sessionBusy is expected to be false here; if this changes, the " +
			"lane check is no longer the thing carrying this case")
	}
	if !api.sessionTurnInProgress(sessionID) {
		t.Fatal("the lane must still report occupancy when the display flag does not")
	}
}

func TestSessionIsFreeOnceTheTurnReleasesTheLane(t *testing.T) {
	api := &StreamingAPI{}
	const sessionID = "session-released"

	release := api.lockSessionInputLane(sessionID)
	if !api.sessionTurnInProgress(sessionID) {
		t.Fatal("occupied while held")
	}
	release()

	if api.sessionTurnInProgress(sessionID) {
		t.Fatal("still occupied after release; queued notifications would never drain")
	}
	if api.isSessionBusyForAutoNotification(sessionID) {
		t.Fatal("an idle session must let an auto-notification execute rather than queue forever")
	}

	// Releasing twice must stay safe — the release closure is once-guarded.
	release()
	if api.sessionTurnInProgress(sessionID) {
		t.Fatal("double release corrupted the lane refcount")
	}
}

func TestUnknownSessionIsNotOccupied(t *testing.T) {
	api := &StreamingAPI{}
	if api.sessionTurnInProgress("never-seen") {
		t.Fatal("a session with no lane must not report occupancy")
	}
	if api.sessionTurnInProgress("") {
		t.Fatal("an empty session id must not report occupancy")
	}
	var nilAPI *StreamingAPI
	if nilAPI.sessionTurnInProgress("x") {
		t.Fatal("nil receiver must be safe")
	}
}
