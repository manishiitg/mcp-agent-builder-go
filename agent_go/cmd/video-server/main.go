package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/agentsession"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/videoproduct"
)

func main() {
	// Video Studio follows AgentWorks' trusted native-shell model, not
	// SparkQuill child isolation. BuildSafeEnvironment keeps the real PATH,
	// HOME, and user-installed CLI configuration while stripping server
	// secrets. This lets the agent install and run local production runtimes
	// such as HyperFrames through execute_shell_command.
	os.Setenv("NATIVE_WORKSPACE", "true")

	// Give this product its own tmux namespace, distinct from
	// multi-llm-provider-go's shared "mlp-*" default that AgentWorks uses
	// unmodified and from family-server's "sq-*". Without this, the orphan sweep
	// below would match by prefix alone and could kill a live AgentWorks session
	// on the same machine. Set all four regardless of which provider is
	// selected, since that can change at any time.
	os.Setenv("CLAUDE_CODE_TMUX_SESSION_PREFIX", "video-claude-code")
	os.Setenv("CURSOR_CLI_INTERACTIVE_SESSION_PREFIX", "video-cursor-cli-int")
	os.Setenv("CODEX_CLI_INTERACTIVE_SESSION_PREFIX", "video-codex-cli-int")
	os.Setenv("PI_CLI_INTERACTIVE_SESSION_PREFIX", "video-pi-cli-int")

	// Clean up coding-agent tmux sessions a PAST process left behind — a crash
	// or a plain restart mid-session orphans whatever was warm at that moment —
	// then keep sweeping hourly. See tmux_sweep.go.
	startTmuxSweepLoop()

	config, err := videoproduct.DefaultConfig()
	if err != nil {
		log.Fatal(err)
	}
	app, err := videoproduct.NewServer(config)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	server := &http.Server{Addr: config.Addr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("Video Studio backend listening on http://%s (Claude Code only)", config.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	agentsession.CloseAllInteractiveSessions()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
