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
		case "--stop", "stop":
			result, err := daemon.StopDaemon()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			if result.WasRunning {
				fmt.Printf("c4cd stopped (pid %d)\n", result.PID)
			} else {
				fmt.Println("c4cd is not running")
			}
			return
		case "--restart", "restart":
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, info, err := daemon.RestartDaemon(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("c4cd restarted (pid %d, port %d)\n", info.PID, info.Port)
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	server := daemon.NewServer(cfg, slotMgr, Version)
	if err := server.Start(ctx); err != nil {
		if errors.Is(err, daemon.ErrDaemonAlreadyRunningHealthy) {
			fmt.Println(err.Error())
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	<-ctx.Done()
	logger.Info("c4cd received shutdown signal; stopping slots")
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
  c4cd --stop         Stop the running daemon (SIGTERM, then SIGKILL after 1s)
  c4cd --restart      Restart the daemon in the background
  c4cd -v             Show version
  c4cd -h             Show this help

If a healthy daemon is already running, c4cd prints its PID and exits without
replacing it. Use c4cd --restart to replace it explicitly.

The daemon exposes a localhost-only control API and owns runtime slot state.
c4cd --stop verifies the daemon identity before terminating it, stops all
slots, and removes stale authority files. SIGINT/SIGTERM/SIGHUP trigger the
same graceful shutdown path.
`)
}
