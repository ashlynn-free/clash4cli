package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"rocket4cli/internal/config"
	"rocket4cli/internal/logger"
	"rocket4cli/internal/proxy"
)

const APIVersion = "v1alpha1"

const (
	defaultHost         = "127.0.0.1"
	defaultPort         = 47189
	requestReadTimeout  = 5 * time.Second
	requestWriteTimeout = 15 * time.Second
	requestIdleTimeout  = 30 * time.Second
	maxHeaderBytes      = 8 << 10
	maxRequestBodyBytes = 1 << 20
)

type Server struct {
	slotMgr    *proxy.SlotManager
	cfg        *config.Config
	configMu   sync.RWMutex
	listener   net.Listener
	httpSrv    *http.Server
	port       int
	revision   atomic.Uint64
	privileged bool
	info       *AuthorityInfo
	lock       *authorityLock
	logHub     *logStreamHub
}


func NewServer(cfg *config.Config, slotMgr *proxy.SlotManager) *Server {
	s := &Server{
		cfg:        cfg,
		slotMgr:    slotMgr,
		port:       defaultPort,
		privileged: os.Geteuid() == 0,
		logHub:     newLogStreamHub(),
	}
	s.revision.Store(1)
	return s
}

func (s *Server) Start(ctx context.Context) error {
	if s.slotMgr == nil {
		return errors.New("slot manager is nil")
	}
	if err := CleanupStaleAuthorityFiles(); err != nil {
		return err
	}
	paths, err := GetAuthorityPaths()
	if err != nil {
		return err
	}
	lock, err := ensureSingleAuthority(paths)
	if err != nil {
		return err
	}
	s.lock = lock

	listener, err := net.Listen("tcp", net.JoinHostPort(defaultHost, fmt.Sprintf("%d", s.port)))
	if err != nil {
		lock.release()
		return err
	}
	s.listener = listener
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok && tcpAddr.Port > 0 {
		s.port = tcpAddr.Port
	}

	info, err := NewAuthorityInfo(s.port, APIVersion, s.privileged)
	if err != nil {
		_ = listener.Close()
		lock.release()
		return err
	}
	s.info = info
	if err := WriteDaemonPID(info.PID); err != nil {
		_ = listener.Close()
		lock.release()
		return err
	}
	if err := WriteAuthorityInfo(info); err != nil {
		_ = listener.Close()
		lock.release()
		return err
	}

	if s.slotMgr != nil {
		s.slotMgr.SetLockProjectionWriter(func(mlf *proxy.MultiLockFile) error {
			return proxyWriteProjection(mlf, ProjectionContext{
				Revision:      s.revision.Load(),
				DaemonPID:     s.info.PID,
				DaemonStarted: s.info.StartedAt,
				APIVersion:    APIVersion,
			})
		})
	}
	if err := s.writeCompatibilityProjection(); err != nil {
		_ = listener.Close()
		lock.release()
		return err
	}
	s.initLogPublisher()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/runtime", s.handleRuntime)
	mux.HandleFunc("/slots/", s.handleSlotRoutes)
	mux.HandleFunc("/logs/stream", s.handleLogStream)
	mux.HandleFunc("/state/config", s.handleConfigState)

	s.httpSrv = &http.Server{
		Handler:           s.limitBody(mux),
		ReadHeaderTimeout: requestReadTimeout,
		ReadTimeout:       requestReadTimeout,
		WriteTimeout:      requestWriteTimeout,
		IdleTimeout:       requestIdleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(shutdownCtx)
	}()

	go func() {
		_ = s.httpSrv.Serve(listener)
	}()
	return nil
}

func (s *Server) initLogPublisher() {
	if s.logHub == nil {
		return
	}
	logger.SetPublisher(func(entry logger.Entry) {
		s.logHub.publish(entry)
	})
}

func (s *Server) Stop(ctx context.Context) error {
	var stopErr error
	if s.slotMgr != nil {
		s.slotMgr.StopAll()
	}
	if s.logHub != nil {
		// Active SSE log streams would otherwise keep Shutdown blocked until
		// the context deadline, making the daemon slow to exit (and causing
		// --stop/--restart to escalate to SIGKILL).
		s.logHub.closeAll()
	}
	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			stopErr = err
		}
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if paths, err := GetAuthorityPaths(); err == nil {
		_ = os.Remove(paths.InfoPath)
		_ = os.Remove(paths.PIDPath)
	}
	if s.lock != nil {
		s.lock.release()
		s.lock = nil
	}
	logger.ClearPublisher()
	return stopErr
}

func (s *Server) bumpRevision() uint64 {
	return s.revision.Add(1)
}

func (s *Server) onRuntimeMutation() {
	s.bumpRevision()
	_ = s.writeCompatibilityProjection()
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"api_version": APIVersion,
		"daemon":      s.info,
		"revision":    s.revision.Load(),
		"healthy":     true,
		"privileged":  s.privileged,
	})
}

func (s *Server) handleRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	active := s.slotMgr.ActiveSlots()
	s.writeJSON(w, http.StatusOK, RuntimeSnapshot{
		APIVersion: APIVersion,
		Revision:   s.revision.Load(),
		Daemon:     s.info,
		Active:     active,
		Count:      len(active),
	})
}
