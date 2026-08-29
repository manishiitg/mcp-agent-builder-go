package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestListCDPTabsRetriesOnceAfterTimeout pins the PLAT-235 fix: the internal
// tab-listing helper used to make one attempt with no retry at all, even
// though it always issues the bare, side-effect-free `tab` list form. Five
// independent Pulse findings converged on the exact same shape -- an
// endpoint proven reachable moments earlier by a passed Stage-0 connection
// test, then a bare tab listing timing out with zero retries.
func TestListCDPTabsRetriesOnceAfterTimeout(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			// Outlast the client's own short timeout so ExecuteCommand sees
			// context.DeadlineExceeded, exactly like a real slow CDP listing.
			time.Sleep(200 * time.Millisecond)
			return
		}
		_ = json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Data: ShellExecuteResponse{
				Stdout:   `[{"id":"t1","label":"work","url":"https://x.com","active":true}]`,
				ExitCode: 0,
			},
		})
	}))
	defer server.Close()

	executor := NewExecutor(NewClient(server.URL))
	output, err := executor.listCDPTabs(context.Background(), "shared-cdp-9222", "http://127.0.0.1:9222", &ExecuteOptions{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("listCDPTabs() error = %v, want the retry to recover", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("server saw %d attempts, want exactly 2 (initial timeout + one retry)", got)
	}
	if output == "" {
		t.Fatal("listCDPTabs() returned empty output on the recovered retry")
	}
}

// TestListCDPTabsDoesNotRetryASecondTime pins that the retry is bounded: a
// listing call that keeps timing out must surface the error, not loop.
func TestListCDPTabsDoesNotRetryASecondTime(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	executor := NewExecutor(NewClient(server.URL))
	_, err := executor.listCDPTabs(context.Background(), "shared-cdp-9222", "http://127.0.0.1:9222", &ExecuteOptions{Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("listCDPTabs() error = nil, want a surfaced timeout after the bounded retry is exhausted")
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("server saw %d attempts, want exactly 2 (no unbounded retry loop)", got)
	}
}

// TestListCDPTabsDoesNotRetryOnCallerCancellation is an independent-review
// follow-up on PLAT-235: isCommandTimeoutError also matches "context
// canceled", which fires when the *caller's* context was canceled rather
// than the CDP endpoint being genuinely slow. Retrying that after a 500ms
// sleep is pure waste -- the reissued command fails immediately for the
// same reason -- so listCDPTabs must not retry it.
func TestListCDPTabsDoesNotRetryOnCallerCancellation(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		_ = json.NewEncoder(w).Encode(APIResponse{Success: true, Data: ShellExecuteResponse{Stdout: "[]", ExitCode: 0}})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before the call, simulating an aborted parent request

	executor := NewExecutor(NewClient(server.URL))
	start := time.Now()
	_, err := executor.listCDPTabs(ctx, "shared-cdp-9222", "http://127.0.0.1:9222", &ExecuteOptions{Timeout: 5 * time.Second})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("listCDPTabs() error = nil, want the cancellation to surface as an error")
	}
	if got := atomic.LoadInt32(&attempts); got > 1 {
		t.Fatalf("server saw %d attempts, want at most 1 -- a canceled context must not trigger the timeout retry", got)
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("listCDPTabs() took %v, want it to fail immediately without the 500ms retry sleep", elapsed)
	}
}
