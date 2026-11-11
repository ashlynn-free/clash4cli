package app

import (
	"context"
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
	"rocket4cli/internal/daemon"
	"rocket4cli/internal/i18n"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/netctl"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
	"rocket4cli/internal/tui"
)

// App represents the main application
type App struct {
	cfg           *config.Config
	subManager    *subscription.Manager
	nodeMgr       *node.Manager
	slotMgr       proxy.SlotService
	version       string
	startupWarn   string
	daemonClient  *daemon.Client
	daemonInfo    *daemon.AuthorityInfo
	daemonLogCh   <-chan logger.Entry
	stopDaemonLog func()
}

// New creates a new application instance
func New(version string) (*App, error) {
	if err := config.EnsureAllDirs(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := i18n.Init(cfg.Language); err != nil {
		return nil, fmt.Errorf("failed to initialize i18n: %w", err)
	}

	if err := logger.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	logger.Info("clash4cli started")

	subManager := subscription.NewManager()
	if err := subManager.Load(); err != nil {
		return nil, fmt.Errorf("failed to load subscriptions: %w", err)
	}

	nodeMgr := node.NewManager(subManager, cfg.SpeedTest.Concurrent, cfg.SpeedTest.Timeout)
	client, info, err := daemon.EnsureDaemon(context.Background(), version)
	if err != nil {
		return nil, fmt.Errorf("failed to connect or start c4cd: %w", err)
	}
	slotMgr := proxy.SlotService(newDaemonSlotService(client))

	var daemonLogCh <-chan logger.Entry
	var stopDaemonLog func()
	if client != nil {
		daemonLogCh, stopDaemonLog, _ = client.StreamLogs(context.Background())
	}

	nodeMgr.SetProbeLockProvider(func(node *subscription.Node) *proxy.LockFile {
		if node == nil || client == nil {
			return nil
		}
		snapshot, err := client.Runtime(context.Background())
		if err != nil || snapshot == nil {
			return nil
		}
		for _, state := range snapshot.Active {
			if state == nil || state.NodeName != node.Name {
				continue
			}
			return &proxy.LockFile{
				Port:             state.Port,
				APIPort:          state.APIPort,
				PID:              state.PID,
				SubID:            state.SubID,
				Node:             state.NodeName,
				ProxyHost:        "127.0.0.1",
				APIHost:          "127.0.0.1",
				UpstreamEnabled:  state.UpstreamEnabled,
				UpstreamProtocol: state.UpstreamProtocol,
				UpstreamServer:   state.UpstreamServer,
				UpstreamPort:     state.UpstreamPort,
			}
		}
		return nil
	})

	subManager.SetUpdateViaProxy(cfg.Subscription.UpdateViaProxy)
	subManager.SetProxyProvider(func() (*url.URL, error) {
		snapshot, err := client.Runtime(context.Background())
		if err != nil || snapshot == nil {
			return nil, nil
		}
		var state *proxy.SlotState
		for _, candidate := range snapshot.Active {
			if candidate != nil && candidate.SlotID == 0 && !candidate.UpstreamEnabled {
				state = candidate
				break
			}
		}
		if state == nil {
			for _, candidate := range snapshot.Active {
				if candidate != nil && !candidate.UpstreamEnabled {
					state = candidate
					break
				}
			}
		}
		if state == nil || state.Port <= 0 || state.Port >= 65536 {
			return nil, nil
		}
		return &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Port))}, nil
	})

	startupWarn := ""
	if os.Geteuid() != 0 && cfg != nil && ((runtime.GOOS == "darwin" && cfg.Proxy.TunEnabled) || (runtime.GOOS == "linux" && (cfg.Proxy.TunEnabled || cfg.Proxy.DockerRedirectTCP))) {
		var features []string
		if cfg.Proxy.TunEnabled {
			features = append(features, "TUN")
		}
		if cfg.Proxy.DockerRedirectTCP {
			features = append(features, "Docker 劫持")
		}
		feat := strings.Join(features, "/")
		startupWarn = fmt.Sprintf("已启用需要 root 的功能（%s）。\n当前连接的 daemon 不是 root 模式，这些功能不会生效。\n请使用 sudo 启动 c4cd。\n提示：sudo 启动时默认仍使用 SUDO_USER 的 ~/.clash4cli（可通过 CLASH4CLI_HOME 覆盖）。", feat)
	}

	return &App{
		cfg:           cfg,
		subManager:    subManager,
		nodeMgr:       nodeMgr,
		slotMgr:       slotMgr,
		version:       version,
		startupWarn:   startupWarn,
		daemonClient:  client,
		daemonInfo:    info,
		daemonLogCh:   daemonLogCh,
		stopDaemonLog: stopDaemonLog,
	}, nil
}

// Run starts the TUI application
func (a *App) Run() error {
	_, _ = fmt.Fprint(os.Stdout, "\x1b[0m")

	if os.Getenv("TMUX") != "" {
		r := lipgloss.NewRenderer(os.Stdout)
		if r.ColorProfile() == termenv.Ascii {
			r.SetColorProfile(termenv.ANSI256)
			lipgloss.SetDefaultRenderer(r)
			logger.Warn("tmux detected but terminal reports no color (TERM=%q); forcing ANSI256 colors", os.Getenv("TERM"))
		}
	}

	model := tui.New(a.cfg, a.subManager, a.nodeMgr, a.slotMgr, a.daemonLogCh, a.version, a.startupWarn)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// Cleanup performs cleanup on shutdown
func (a *App) Cleanup() {
	logger.Info("clash4cli shutting down")
	if runtime.GOOS == "darwin" {
		snapshot, err := netctl.LoadSystemProxySnapshot()
		if err == nil && snapshot != nil {
			if err := netctl.RestoreSystemProxy(snapshot); err != nil {
				logger.Warn("failed to restore system proxy: %v", err)
			} else {
				_ = netctl.ClearSystemProxySnapshot()
			}
		}
	}
	if a.stopDaemonLog != nil {
		a.stopDaemonLog()
	}
	logger.Close()
}
