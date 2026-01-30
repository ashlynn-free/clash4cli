package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"rocket4cli/internal/app"
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
  c4c -v             Show version
  c4c -h             Show this help

Keyboard shortcuts (in TUI):
  Tab                  Switch tabs (Nodes/Subscriptions/Settings/Logs)
  j/k or ↑/↓          Navigate lists
  Enter               (Nodes) Cycle mode (Disconnect/Rule/Global); (Subscriptions) Set active subscription
  t                   Test all nodes speed
  a                   Add subscription (in Subscriptions tab)
  d                   Delete subscription (in Subscriptions tab)
  u                   Update subscription (in Subscriptions tab)
  q/Ctrl+C            Quit (proxy keeps running)

Data directory: ~/.clash4cli/
`)
}
