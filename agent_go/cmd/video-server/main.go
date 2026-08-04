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
