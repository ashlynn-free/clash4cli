package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/daemon"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("c4cd %s\n", Version)
			fmt.Printf("Build time: %s\n", BuildTime)
			return
		case "-h", "--help", "help":
			printHelp()
			return
		}
	}

	if err := config.EnsureAllDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := logger.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	subManager := subscription.NewManager()
	if err := subManager.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	_ = subManager

	proxy.SetDetachSubprocesses(false)
	defer proxy.SetDetachSubprocesses(true)

	slotMgr := proxy.NewSlotManager(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := daemon.NewServer(cfg, slotMgr)
	if err := server.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`c4cd - clash4cli daemon

Usage:
  c4cd                Start local runtime daemon
  c4cd -v             Show version
  c4cd -h             Show this help

The daemon exposes a localhost-only control API and owns runtime slot state.
`)
}
