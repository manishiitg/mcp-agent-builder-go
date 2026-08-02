//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestDiffPatchCreationWithControllingTTY is the regression for a production
// workspace-server hang. A /dev/null creation diff is applied through the real
// HTTP handler while the server owns a controlling terminal, matching the
// desktop launch environment. BSD patch opens /dev/tty for its reverse-patch
// question in this situation; a server without a TTY cannot reproduce it.
func TestDiffPatchCreationWithControllingTTY(t *testing.T) {
	docsDir := t.TempDir()
	serverURL := startTTYWorkspaceServer(t, docsDir)
	if !waitForServer(t, serverURL, 10*time.Second) {
		t.Fatal("workspace server did not become ready")
	}

	diff := "--- /dev/null\n" +
		"+++ b/created.json\n" +
		"@@ -0,0 +1,1 @@\n" +
		"+{\"created\": true}\n" +
		"\\ No newline at end of file\n"
	body, err := json.Marshal(map[string]string{"diff": diff})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		serverURL+"/api/documents/_diff_patch_tty_e2e/created.json/diff",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("creation diff did not return within 2s (possible interactive patch hang): %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status=%d body=%s", resp.StatusCode, responseBody)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("creation diff took %s; expected a bounded response", elapsed)
	}

	got, err := os.ReadFile(filepath.Join(docsDir, "_diff_patch_tty_e2e", "created.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"created\": true}"
	if string(got) != want {
		t.Fatalf("created file=%q want=%q", got, want)
	}
}

func startTTYWorkspaceServer(t *testing.T, docsDir string) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	binaryPath := filepath.Join(t.TempDir(), "workspace-test-server")
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build workspace server: %v\n%s", err, output)
	}

	cmd := exec.Command(
		binaryPath,
		"server",
		"--debug",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--docs-dir", docsDir,
	)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start workspace server with PTY: %v", err)
	}
	go func() {
		_, _ = io.Copy(io.Discard, ptmx)
	}()

	t.Cleanup(func() {
		// pty.Start creates a new session whose process-group id is the server
		// pid. Kill the whole group so a failing pre-fix test cannot leave the
		// blocked patch child behind.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = ptmx.Close()
		_ = cmd.Wait()
	})

	return fmt.Sprintf("http://127.0.0.1:%d", port)
}
