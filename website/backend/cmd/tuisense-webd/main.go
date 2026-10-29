//go:build linux

// Command tuisense-webd is the TuiSense web demo backend. It serves the
// browser terminal's WebSocket API and runs one sandboxed `tuisense
// --web-demo` per visitor. Configuration comes from TUISENSE_WEB_*
// environment variables (see website/deployment/.env.example).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Phobby/TuiSense/internal/version"
	"github.com/Phobby/TuiSense/website/backend/server"
)

func main() {
	// The same binary is the sandbox helper that confines each session
	// process before executing tuisense (see server.SandboxMain).
	if len(os.Args) > 1 && os.Args[1] == server.SandboxArg {
		os.Exit(server.SandboxMain(os.Args[2:]))
	}
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("tuisense-webd %s (commit %s)\n", version.Display(), version.ShortCommit())
		return
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if os.Geteuid() == 0 && os.Getenv("TUISENSE_WEB_ALLOW_ROOT") != "1" {
		log.Error("refusing to run as root: start tuisense-webd as an unprivileged user")
		os.Exit(1)
	}
	cfg, err := server.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Error("cannot start", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
