package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rocket4cli/internal/app"
	"rocket4cli/internal/daemon"
)

// Version information (set via ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Handle version flag
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("clash4cli %s\n", Version)
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

	// Create application
	application, err := app.New(Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		application.Cleanup()
		os.Exit(0)
	}()

	// Run the TUI
	if err := application.Run(); err != nil {
		application.Cleanup()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	application.Cleanup()
}

func printHelp() {
	fmt.Printf(`clash4cli - Terminal proxy manager

Usage:
  c4c                Start the TUI interface
  c4c --stop         Stop the running c4cd daemon (SIGTERM, then SIGKILL after 1s)
  c4c --restart      Restart c4cd in the background
  c4c -v             Show version
  c4c -h             Show this help

Run any command through the proxy (c4cgo):
  c4cgo <command> [args...]          Run a command through the active proxy slot
  c4cgo -s N <command>               Use proxy slot N (0-9)
  c4cgo --port PORT <command>        Use an explicit mixed proxy port
  c4cgo --mode MODE <command>        auto|proxychains|env (default: auto)
  c4cgo --strict <command>           Fail closed (default on macOS)
  c4cgo --no-strict <command>        Disable the fail-closed sandbox
  c4cgo -v <command>                 Show proxy details
  c4cgo --help                       Show full c4cgo usage

Examples:
  c4cgo curl https://ip.sb
  c4cgo git clone https://github.com/user/repo.git
  c4cgo --no-strict npm install

Keyboard shortcuts (in TUI):
  Tab                  Switch tabs (Slots/Subscriptions/Settings)
  j/k or ↑/↓          Navigate lists
  Enter               Open the selected node/subscription browser
  0-9                 (Slots) Assign a node to proxy slot 0-9
  t                   Test the selected node/slot
  T                   Test all nodes/slots
  a                   Add subscription (in Subscriptions tab)
  d                   Delete subscription (in Subscriptions tab)
  u                   Update subscription (in Subscriptions tab)
  C                   (Settings) Open the live connections view
  q/Ctrl+C            Quit the TUI (proxy slots keep running in c4cd)

Data directory: ~/.clash4cli/
`)
}
