package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/config"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui"
)

// App represents the main application
type App struct {
	cfg        *config.Config
	subManager *subscription.Manager
	nodeMgr    *node.Manager
	proxyCtrl  *proxy.Controller
	lock       *instanceLock
}

// New creates a new application instance
func New() (*App, error) {
	// Ensure all directories exist
	if err := config.EnsureAllDirs(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	lockPath, err := config.GetInstanceLockFilePath()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve instance lock path: %w", err)
	}
	lock, err := ensureSingleInstanceInteractive(lockPath)
	if err != nil {
		return nil, err
	}
	releaseLockOnError := true
	defer func() {
		if releaseLockOnError {
			lock.release()
		}
	}()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize i18n
	if err := i18n.Init(cfg.Language); err != nil {
		return nil, fmt.Errorf("failed to initialize i18n: %w", err)
	}

	// Initialize logger
	if err := logger.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	logger.Info("clash4cli started")

	// Create managers
	subManager := subscription.NewManager()
	if err := subManager.Load(); err != nil {
		return nil, fmt.Errorf("failed to load subscriptions: %w", err)
	}

	nodeMgr := node.NewManager(subManager, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	proxyCtrl := proxy.NewController(cfg)

	a := &App{
		cfg:        cfg,
		subManager: subManager,
		nodeMgr:    nodeMgr,
		proxyCtrl:  proxyCtrl,
		lock:       lock,
	}
	releaseLockOnError = false
	return a, nil
}

// Run starts the TUI application
func (a *App) Run() error {
	model := tui.New(a.cfg, a.subManager, a.nodeMgr, a.proxyCtrl)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := p.Run()
	return err
}

// Cleanup performs cleanup on shutdown
func (a *App) Cleanup() {
	logger.Info("clash4cli shutting down")
	if a.proxyCtrl != nil {
		_ = a.proxyCtrl.Stop()
	}
	logger.Close()
	if a.lock != nil {
		a.lock.release()
	}
}
