package cliupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var releaseVersion = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]*$`)

// Install into a fresh prefix; native "update" commands may target Homebrew,
// npm's shared global prefix, or ~/.local. Use an explicit npm prefix for the
// npm-distributed CLIs and an isolated HOME for Cursor's official installer.
func installRelease(ctx context.Context, p Provider, releases string, previous Result) (string, string, error) {
	dir, err := os.MkdirTemp(releases, p.Name+"-*")
	if err != nil {
		return "", "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		return "", "", err
	}
	env := installEnv(home)
	var binary, version string
	if p.Package != "" {
		out, err := run(ctx, dir, env, "npm", "view", p.Package+"@latest", "version", "--json", "--registry=https://registry.npmjs.org")
		if err != nil {
			return "", "", fmt.Errorf("check %s: %w", p.Name, err)
		}
		if err := json.Unmarshal(out, &version); err != nil || !releaseVersion.MatchString(version) {
			return "", "", fmt.Errorf("invalid registry version for %s", p.Name)
		}
		if version == previous.Version && strings.HasPrefix(previous.Executable, releases+string(os.PathSeparator)) {
			if observed, err := smoke(ctx, previous.Executable, env, p); err == nil && containsVersion(observed, version) {
				return previous.Executable, version, nil
			}
		}
		args := []string{"install", "--global", "--prefix", dir, "--no-audit", "--no-fund", "--registry=https://registry.npmjs.org", p.Package + "@" + version}
		// Pi's optional lifecycle scripts are not needed by its coding CLI.
		if p.Name == "pi" {
			args = append(args, "--ignore-scripts")
		}
		if _, err := run(ctx, dir, env, "npm", args...); err != nil {
			return "", "", fmt.Errorf("install %s: %w", p.Name, err)
		}
		binary = filepath.Join(dir, "bin", p.Name)
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cursor.com/install", nil)
		if err != nil {
			return "", "", err
		}
		client := &http.Client{Timeout: time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("fetch Cursor installer: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("Cursor installer HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
		if err != nil || len(data) > 1024*1024 {
			return "", "", fmt.Errorf("cannot read Cursor installer")
		}
		script := filepath.Join(dir, "install.sh")
		if err := os.WriteFile(script, data, 0600); err != nil {
			return "", "", err
		}
		if _, err := run(ctx, dir, env, "bash", script); err != nil {
			return "", "", fmt.Errorf("install cursor-agent: %w", err)
		}
		binary = filepath.Join(home, ".local", "bin", "cursor-agent")
	}
	// Resolve the candidate symlink once. A chat's pin points directly at this
	// release, never through a mutable "current" or ~/.local/bin link.
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return "", "", err
	}
	if !strings.HasPrefix(binary, dir+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("installer escaped private release directory")
	}
	observed, err := smoke(ctx, binary, env, p)
	if err != nil {
		return "", "", fmt.Errorf("verify %s: %w", p.Name, err)
	}
	if version == "" {
		version = observed
	}
	if p.Package != "" && !containsVersion(observed, version) {
		return "", "", fmt.Errorf("%s reported a different version than %s", p.Name, version)
	}
	if version == previous.Version {
		if oldObserved, err := smoke(ctx, previous.Executable, env, p); err == nil {
			if (p.Package == "" && oldObserved == version) || (p.Package != "" && containsVersion(oldObserved, version)) {
				return previous.Executable, version, nil
			}
		}
	}
	if p.Package != "" {
		_ = os.RemoveAll(home)
	} // discard install cache/logs
	keep = true
	return binary, version, nil
}

func containsVersion(output, version string) bool {
	for _, field := range strings.Fields(output) {
		if strings.Trim(field, "v()") == version {
			return true
		}
	}
	return false
}

func smoke(ctx context.Context, binary string, env []string, p Provider) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"--version"}
	if p.Name == "cursor-agent" {
		args = append([]string{"--disable-auto-update"}, args...)
	}
	out, err := run(ctx, filepath.Dir(binary), env, binary, args...)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(out))
	if version == "" || len(version) > 200 {
		return "", fmt.Errorf("invalid --version response")
	}
	return version, nil
}

// Installers receive no provider tokens, workflow secrets, or user's npmrc.
// Proxy/CA configuration is retained for servers using a corporate network.
func installEnv(home string) []string {
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"), "SHELL=/bin/bash", "CI=1", "DISABLE_AUTOUPDATER=1", "DISABLE_UPDATES=1", "npm_config_userconfig=/dev/null", "npm_config_cache=" + filepath.Join(home, ".npm"), "PI_TELEMETRY=0", "PI_OFFLINE=1"}
	for _, k := range []string{"PATH", "TMPDIR", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS"} {
		if value := os.Getenv(k); value != "" {
			env = append(env, k+"="+value)
		}
	}
	return env
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if left := 64*1024 - b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func run(ctx context.Context, dir string, env []string, executable string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var out limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Package-manager output may contain credentials embedded in proxy URLs.
		return nil, fmt.Errorf("%s: %w", filepath.Base(executable), err)
	}
	return out.Bytes(), nil
}
