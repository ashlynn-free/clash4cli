package app

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	cfg         *config.Config
	subManager  *subscription.Manager
	nodeMgr     *node.Manager
	slotMgr     *proxy.SlotManager
	lock        *instanceLock
	version     string
	startupWarn string
}

// New creates a new application instance
func New(version string) (*App, error) {
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
	slotMgr := proxy.NewSlotManager(cfg)
	nodeMgr.SetProbeLockProvider(func(node *subscription.Node) *proxy.LockFile {
		if node == nil || slotMgr == nil {
			return nil
		}
		slotID, ok := slotMgr.NodeSlot(node.Name)
		if !ok {
			return nil
		}
		mlf, err := proxy.ReadMultiLockFile()
		if err != nil || mlf == nil {
			return nil
		}
		lock, ok := mlf.Instances[slotID]
		if !ok {
			return nil
		}
		lockCopy := lock
		return &lockCopy
	})
	subManager.SetUpdateViaProxy(cfg.Subscription.UpdateViaProxy)
	subManager.SetProxyProvider(func() (*url.URL, error) {
		// Use slot 0 (or first active slot) for subscription updates via proxy,
		// but never borrow a slot that is already chained to an upstream proxy.
		state := slotMgr.GetSlotState(0)
		if state != nil && state.UpstreamEnabled {
			state = nil
		}
		if state == nil {
			for _, candidate := range slotMgr.ActiveSlots() {
				if candidate != nil && !candidate.UpstreamEnabled {
					state = candidate
					break
				}
			}
		}
		if state == nil || state.Port <= 0 || state.Port >= 65536 {
			return nil, nil
		}
		return &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Port)),
		}, nil
	})

	startupWarn := ""
	if runtime.GOOS == "linux" && os.Geteuid() != 0 && cfg != nil && (cfg.Proxy.TunEnabled || cfg.Proxy.DockerRedirectTCP) {
		var features []string
		if cfg.Proxy.TunEnabled {
			features = append(features, "TUN")
		}
		if cfg.Proxy.DockerRedirectTCP {
			features = append(features, "Docker 劫持")
		}
		feat := strings.Join(features, "/")
		startupWarn = fmt.Sprintf("已启用需要 root 的功能（%s）。\n当前不是 root 运行，这些功能不会生效。\n请使用 sudo 启动 c4c（或授予 CAP_NET_ADMIN）。\n提示：sudo 启动时默认仍使用 SUDO_USER 的 ~/.clash4cli（可通过 CLASH4CLI_HOME 覆盖）。", feat)
	}

	a := &App{
		cfg:         cfg,
		subManager:  subManager,
		nodeMgr:     nodeMgr,
		slotMgr:     slotMgr,
		lock:        lock,
		version:     version,
		startupWarn: startupWarn,
	}
	releaseLockOnError = false
	return a, nil
}

// Run starts the TUI application
func (a *App) Run() error {
	// Reset any leaked terminal attributes (e.g. underline/reverse) before
	// entering the alternate screen. tmux can preserve these attributes across
	// programs, causing "garbled" UI where spaces render as underscores.
	_, _ = fmt.Fprint(os.Stdout, "\x1b[0m")

	// Work around tmux setups where TERM is set to "screen" (no "color" suffix),
	// which can cause termenv to report Ascii and disable all styling.
	//
	// Recommended fix for users is still: set tmux default-terminal to
	// "tmux-256color" or "screen-256color".
	if os.Getenv("TMUX") != "" {
		r := lipgloss.NewRenderer(os.Stdout)
		if r.ColorProfile() == termenv.Ascii {
			r.SetColorProfile(termenv.ANSI256)
			lipgloss.SetDefaultRenderer(r)
			logger.Warn("tmux detected but terminal reports no color (TERM=%q); forcing ANSI256 colors", os.Getenv("TERM"))
		}
	}

	model := tui.New(a.cfg, a.subManager, a.nodeMgr, a.slotMgr, a.version, a.startupWarn)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}

// Cleanup performs cleanup on shutdown
func (a *App) Cleanup() {
	logger.Info("clash4cli shutting down")
	logger.Close()
	if a.lock != nil {
		a.lock.release()
	}
}
