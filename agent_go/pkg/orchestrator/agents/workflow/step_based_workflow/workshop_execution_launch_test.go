package step_based_workflow

import (
	"testing"
	"time"
)

type blockingWorkshopExecutionNotifier struct {
	started chan WorkshopExecutionStart
	release chan struct{}
}

func (n *blockingWorkshopExecutionNotifier) OnExecutionStart(start WorkshopExecutionStart) {
	n.started <- start
	<-n.release
}

func (*blockingWorkshopExecutionNotifier) OnExecutionComplete(string, string, string, map[string]string, error) {
}

func (*blockingWorkshopExecutionNotifier) OnExecutionTerminated(string, string) {}

func TestRegisterWorkshopExecutionBeforeLaunchIsSynchronous(t *testing.T) {
	notifier := &blockingWorkshopExecutionNotifier{
		started: make(chan WorkshopExecutionStart, 1),
		release: make(chan struct{}),
	}
	returned := make(chan struct{})
	go func() {
		registerWorkshopExecutionBeforeLaunch(notifier, WorkshopExecutionStart{ID: "exec-step-1"})
		close(returned)
	}()

	select {
	case start := <-notifier.started:
		if start.ID != "exec-step-1" {
			t.Fatalf("registered execution = %q, want exec-step-1", start.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("execution was not registered")
	}

	select {
	case <-returned:
		t.Fatal("launch boundary returned before execution registration completed")
	default:
	}

	close(notifier.release)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("launch boundary did not return after registration completed")
	}
}
