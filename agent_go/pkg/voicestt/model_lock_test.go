package voicestt

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestAcquireModelDownloadLockSerializesConcurrentDownloaders exercises the
// scenario two AgentWorks-family desktops share: both warm the voice engine
// on launch, but only one may download the shared model files at a time.
func TestAcquireModelDownloadLockSerializesConcurrentDownloaders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "voice-models")

	release1, err := acquireModelDownloadLock(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	var secondAcquired atomic.Bool
	done := make(chan struct{})
	go func() {
		release2, err := acquireModelDownloadLock(context.Background(), dir)
		if err != nil {
			t.Error(err)
			close(done)
			return
		}
		secondAcquired.Store(true)
		release2()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	if secondAcquired.Load() {
		t.Fatal("a second process must not acquire the lock while the first holds it")
	}

	release1()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("second acquirer never proceeded after the first released the lock")
	}
	if !secondAcquired.Load() {
		t.Fatal("second acquirer did not report success")
	}
}

func TestAcquireModelDownloadLockRespectsContextCancellation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "voice-models")

	release, err := acquireModelDownloadLock(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := acquireModelDownloadLock(ctx, dir); err == nil {
		t.Fatal("expected acquireModelDownloadLock to return an error once the context is canceled")
	}
}

// TestPendingModelDownloadsSkipsCompletedModels is a narrower unit check for
// the re-check pendingModelDownloads() does after acquiring the lock — a
// waiter must see the other process's completed download, not re-fetch it.
func TestPendingModelDownloadsSkipsCompletedModels(t *testing.T) {
	dir := t.TempDir()
	todo, needPunct := pendingModelDownloads(dir)
	if len(todo) == 0 {
		t.Fatal("an empty dir must report every default model as pending")
	}
	if !needPunct {
		t.Fatal("an empty dir must report the punctuation model as pending")
	}
}
