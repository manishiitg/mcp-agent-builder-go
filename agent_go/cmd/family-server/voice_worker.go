package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

//go:embed voice_worker.py
var voiceWorkerScript embed.FS

// voiceWorkerIdleTimeout is how long the warm worker stays loaded after its
// last request before shutting itself down. Mirrors this app's own
// download-on-activate / delete-on-deactivate philosophy, applied to RUNTIME
// memory instead of disk: a family mid-conversation keeps the model warm
// (each call ~0.8s instead of ~3.5s), but an idle process isn't left holding
// onto RAM/GPU memory for a family who stepped away.
const voiceWorkerIdleTimeout = 15 * time.Minute

// voiceWorker manages ONE persistent Python process that keeps the Parakeet
// (STT) model loaded in memory, reused across every request rather than
// reloaded from scratch each time. A fresh
// `python -c` process pays ~1.9s just re-importing mlx_audio before any real
// work happens — measured directly, not assumed — which is why this exists.
//
// Requests are strictly serialized (one in flight at a time): MLX/Metal has
// documented issues under concurrent access (mlx-audio's own GitHub reports
// crashes under concurrent requests), and a family's voice usage is
// inherently serial anyway — one recording or one reply spoken at a time.
type voiceWorker struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	idle   *time.Timer
	// warm tracks which models are ACTUALLY loaded in the running process's
	// memory right now — distinct from "installed on disk". A model can be
	// fully installed but still cold (worker never started, or idle-shut-down
	// 15 minutes ago) — this is what lets the UI say "Warming up..." instead
	// of falsely claiming instant readiness.
	warm map[string]bool

	// How to start the process. Everything else here — the readiness
	// handshake, call timeout, idle unload, crash reaping — is transport
	// agnostic, so the native Swift helper (voice_native.go) reuses all of it
	// by supplying a different launcher rather than duplicating the
	// supervision logic. name only labels log lines.
	name   string
	launch func() (*exec.Cmd, error)
	// Per-worker override for voiceWorkerCallTimeout. The native helper needs
	// a much longer one: its very first `load` downloads CoreML weights
	// (~96s measured), which would otherwise trip the 30s default and get the
	// process killed mid-download, forever.
	callTimeout time.Duration
	// Per-worker override for voiceWorkerIdleTimeout.
	idleTimeout time.Duration
}

func (w *voiceWorker) idleAfter() time.Duration {
	if w.idleTimeout > 0 {
		return w.idleTimeout
	}
	return voiceWorkerIdleTimeout
}

func (w *voiceWorker) timeout() time.Duration {
	if w.callTimeout > 0 {
		return w.callTimeout
	}
	return voiceWorkerCallTimeout
}

var sharedVoiceWorker = &voiceWorker{name: "voice", launch: pythonVoiceWorkerCmd}

// pythonVoiceWorkerCmd starts the MLX/Python worker.
func pythonVoiceWorkerCmd() (*exec.Cmd, error) {
	scriptBytes, err := voiceWorkerScript.ReadFile("voice_worker.py")
	if err != nil {
		return nil, err
	}
	// Written into the SAME directory the shared MLX environment already
	// lives in, rather than embedded-and-run-from-memory, so it's a normal
	// file `python` can execute and a developer can open to debug.
	scriptPath := filepath.Join(mlxVoiceDir(), "voice_worker.py")
	if err := os.WriteFile(scriptPath, scriptBytes, 0o600); err != nil {
		return nil, err
	}
	return exec.Command(mlxVoicePython(), scriptPath), nil
}

// voiceWorkerCallTimeout bounds how long ANY single request may block waiting
// on the worker's response. Without this, a genuinely stuck worker (observed
// live: the Python process sitting at ~0% CPU, never responding, after a
// transcribe request) held w.mu FOREVER — every later call, even a totally
// unrelated one from a different conversation minutes later, piled up behind
// the same dead read with no error, no log line, nothing. Generous enough for
// a real cold start (measured worst case ~7s) plus margin, short enough that
// a genuinely wedged worker gets NOTICED and replaced within one request
// instead of taking the whole voice feature down silently for good.
const voiceWorkerCallTimeout = 30 * time.Second

// call sends one request and waits for its matching response, bounded by
// ctx AND voiceWorkerCallTimeout (whichever is shorter). Starts the worker if
// it isn't already running (first use, previously idle-shut-down, or
// crashed), and restarts+retries ONCE transparently if the pipe is
// unexpectedly broken — a stale/crashed process should be invisible to the
// caller, not a hard failure on their very next click. A timeout counts as
// broken too: the worker is killed so its stuck goroutine unblocks and the
// NEXT call (this retry, or a future one) starts a fresh process rather than
// queuing behind the same dead one forever.
func (w *voiceWorker) call(ctx context.Context, req map[string]any) (map[string]any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, w.timeout())
	defer cancel()
	resp, err := w.callLocked(callCtx, req)
	if err != nil {
		// One retry against a freshly (re)started process — covers the
		// common case of the worker having crashed, been idle-killed, or
		// timed out between calls, without making every transient hiccup
		// visible.
		w.stopLocked()
		retryCtx, retryCancel := context.WithTimeout(ctx, w.timeout())
		defer retryCancel()
		resp, err = w.callLocked(retryCtx, req)
	}
	return resp, err
}

func (w *voiceWorker) callLocked(ctx context.Context, req map[string]any) (map[string]any, error) {
	if err := w.ensureStartedLocked(); err != nil {
		return nil, err
	}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := w.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("voice worker pipe broken: %w", err)
	}
	// ReadString blocks on the pipe with no deadline of its own — run it in a
	// goroutine and race it against ctx so a wedged worker can't hold this
	// call (and w.mu, and every call after it) forever. The goroutine itself
	// may still be blocked in ReadString after we give up on it; killing the
	// process below (via the caller's stopLocked on error) is what actually
	// unblocks it, not this select.
	type readResult struct {
		line string
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		respLine, err := w.reader.ReadString('\n')
		resultCh <- readResult{respLine, err}
	}()
	var respLine string
	select {
	case res := <-resultCh:
		if res.err != nil {
			return nil, fmt.Errorf("voice worker did not respond: %w", res.err)
		}
		respLine = res.line
	case <-ctx.Done():
		return nil, fmt.Errorf("voice worker timed out: %w", ctx.Err())
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(respLine), &resp); err != nil {
		return nil, fmt.Errorf("voice worker sent an unreadable response: %w", err)
	}
	if errMsg, ok := resp["error"]; ok {
		return nil, fmt.Errorf("%v", errMsg)
	}

	// Every successful call pushes the idle-shutdown clock back out — the
	// worker only unloads after a real gap in use, not a fixed wall-clock
	// window from when it started.
	if w.idle != nil {
		w.idle.Reset(w.idleAfter())
	}
	if model, ok := req["model"].(string); ok {
		if w.warm == nil {
			w.warm = map[string]bool{}
		}
		w.warm[model] = true
	}
	return resp, nil
}

// IsWarm reports whether a model is ACTUALLY loaded in the worker's memory
// right now — not just installed on disk. False whenever the worker isn't
// running at all (never started, idle-shut-down, or crashed) or hasn't yet
// been asked to use this specific model since it last started.
func (w *voiceWorker) IsWarm(model string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cmd != nil && w.warm[model]
}

func (w *voiceWorker) ensureStartedLocked() error {
	if w.cmd != nil {
		return nil
	}
	cmd, err := w.launch()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the voice engine: %w", err)
	}

	// Wait for the explicit readiness line on stderr rather than a fixed
	// sleep — import time varies by machine, and stdout is reserved for JSON
	// responses only, so readiness can't be signaled there.
	ready := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if sc.Text() == "WORKER_READY" {
				ready <- nil
				go io.Copy(io.Discard, stderr) // drain remaining stderr so the pipe never fills and blocks the process
				return
			}
		}
		ready <- fmt.Errorf("voice engine exited before it was ready")
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			return err
		}
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		return fmt.Errorf("voice engine took too long to start")
	}

	w.cmd = cmd
	w.stdin = stdin
	w.reader = bufio.NewReader(stdout)
	w.idle = time.AfterFunc(w.idleAfter(), func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		log.Printf("[%s] worker idle for %s, unloading", w.name, w.idleAfter())
		w.stopLocked()
	})

	// Clear state the MOMENT the process actually exits — crash, killed
	// externally, anything — rather than leaving IsWarm()/callLocked() to
	// find out lazily on the next attempted use. Without this, a dead
	// process could be reported "warm" for an arbitrarily long stretch:
	// stale bookkeeping, not the process's real, current state.
	deadCmd := cmd
	go func() {
		// The ONE place that calls Wait() on this process — see stopLocked's
		// doc comment for why that has to be exclusive.
		_ = deadCmd.Wait()
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.cmd == deadCmd { // only clear if nothing else already replaced it
			log.Printf("[voice] worker process exited")
			w.stopLocked()
		}
	}()

	log.Printf("[%s] worker started (models load lazily on first use of each)", w.name)
	return nil
}

// stopLocked asks a still-running process to terminate and clears bookkeeping
// immediately, but does NOT call Wait() itself — the reaper goroutine spawned
// in ensureStartedLocked is already watching this exact process and will Wait
// on it once it actually exits. Calling Wait() from two places on the same
// *exec.Cmd is not safe in Go (a second call errors with "Wait was already
// called"), so exactly one place — the reaper — owns that. Safe to call when
// nothing is running.
func (w *voiceWorker) stopLocked() {
	if w.idle != nil {
		w.idle.Stop()
		w.idle = nil
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
	w.cmd = nil
	w.stdin = nil
	w.reader = nil
	w.warm = nil // the new process starts cold — nothing is loaded yet
}
