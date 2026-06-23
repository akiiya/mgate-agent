package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type Manager struct {
	opts      ManagerOptions
	logger    *slog.Logger
	wsHealthy atomic.Bool
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if !opts.WSEnabled && !opts.PullEnabled {
		return nil, errors.New("at least one transport must be enabled")
	}
	if opts.Dispatcher == nil {
		return nil, errors.New("transport manager requires result dispatcher")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Manager{opts: opts, logger: logger}, nil
}

func (m *Manager) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 4)
	healthCh := make(chan HealthEvent, 8)

	// Manager 只协调 transport 生命周期；命令处理仍然统一交给 commands.Handler，
	// result 可靠性则统一交给 outbox dispatcher，避免 WS/Pull 各自长出业务逻辑。
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := m.opts.Dispatcher.Run(ctx, time.Second); err != nil && ctx.Err() == nil {
			reportManagerErr(errCh, err)
		}
	}()

	if m.opts.WSEnabled {
		wsOpts := m.opts.WS
		wsOpts.HealthEvents = healthCh
		if wsOpts.Dispatcher == nil {
			wsOpts.Dispatcher = m.opts.Dispatcher
		}
		ws, err := NewWSClient(wsOpts)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ws.Run(ctx); err != nil && ctx.Err() == nil {
				reportManagerErr(errCh, err)
			}
		}()
	}

	if m.opts.PullEnabled {
		pullOpts := m.opts.Pull
		if pullOpts.Dispatcher == nil {
			pullOpts.Dispatcher = m.opts.Dispatcher
		}
		pull, err := NewPullClient(pullOpts)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pull.Run(ctx, func() bool {
				return !m.opts.WSEnabled || !m.wsHealthy.Load()
			}); err != nil && ctx.Err() == nil {
				reportManagerErr(errCh, err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-healthCh:
				healthy := event.State == HealthConnected
				m.wsHealthy.Store(healthy)
				if healthy {
					m.logger.Info("WebSocket 健康，暂停 Pull 高频轮询")
				} else {
					m.logger.Warn("WebSocket 不健康，Pull 兜底可启用")
				}
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		<-done
		return nil
	case err := <-errCh:
		return err
	}
}

func reportManagerErr(errCh chan<- error, err error) {
	select {
	case errCh <- err:
	default:
	}
}
