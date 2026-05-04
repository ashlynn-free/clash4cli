package daemon

import (
	"time"

	"rocket4cli/internal/proxy"
)

const projectionMaxAge = 30 * time.Second

type ProjectionContext struct {
	Revision      uint64
	DaemonPID     int
	DaemonStarted string
	APIVersion    string
}

func proxyWriteProjection(mlf *proxy.MultiLockFile, ctx ProjectionContext) error {
	return proxy.WriteProjectedMultiLockFile(mlf, proxy.ProjectionMeta{
		Source:           "c4cd",
		DaemonPID:        ctx.DaemonPID,
		DaemonAPIVersion: ctx.APIVersion,
		DaemonStartedAt:  ctx.DaemonStarted,
		Revision:         ctx.Revision,
		GeneratedAt:      time.Now().UTC(),
		MaxAge:           projectionMaxAge,
	})
}

func (s *Server) writeCompatibilityProjection() error {
	if s.slotMgr == nil || s.info == nil {
		return nil
	}
	mlf := &proxy.MultiLockFile{Version: 2, Instances: make(map[int]proxy.LockFile)}
	for _, state := range s.slotMgr.ActiveSlots() {
		mlf.Instances[state.SlotID] = proxy.LockFile{
			Port:             state.Port,
			APIPort:          state.APIPort,
			PID:              state.PID,
			SubID:            state.SubID,
			Node:             state.NodeName,
			ProxyHost:        "127.0.0.1",
			APIHost:          "127.0.0.1",
			EffectiveProxy:   state.NodeName,
			UpstreamEnabled:  state.UpstreamEnabled,
			UpstreamProtocol: state.UpstreamProtocol,
			UpstreamServer:   state.UpstreamServer,
			UpstreamPort:     state.UpstreamPort,
			Detached:         false,
		}
	}
	return proxyWriteProjection(mlf, ProjectionContext{
		Revision:      s.revision.Load(),
		DaemonPID:     s.info.PID,
		DaemonStarted: s.info.StartedAt,
		APIVersion:    APIVersion,
	})
}
