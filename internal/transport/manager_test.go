package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mgate-agent/internal/protocol"

	"nhooyr.io/websocket"
)

func TestManagerRunsPullWhenWebSocketDisabled(t *testing.T) {
	var pulls atomic.Int64
	pulled := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pulls.Add(1)
		closeOnce(pulled)
		writeJSON(t, w, PullResponsePayload{Commands: nil})
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	manager := mustManager(t, ManagerOptions{
		WSEnabled:   false,
		PullEnabled: true,
		Pull:        pullOptionsForTest(httpServer.URL, noopHandler{}),
	})
	errCh := runManager(ctx, manager)
	waitClosed(t, pulled)
	cancel()
	waitManager(t, errCh)
	if pulls.Load() == 0 {
		t.Fatal("Pull did not run when WebSocket disabled")
	}
}

func TestManagerPausesPullWhileWebSocketHealthy(t *testing.T) {
	helloAccepted := make(chan struct{})
	wsServer := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		closeOnce(helloAccepted)
		<-r.Context().Done()
	})
	defer wsServer.Close()

	var pulls atomic.Int64
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pulls.Add(1)
		writeJSON(t, w, PullResponsePayload{Commands: nil})
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	manager := mustManager(t, ManagerOptions{
		WSEnabled:   true,
		PullEnabled: true,
		WS:          wsOptionsForTest(wsServer.URL, noopHandler{}),
		Pull:        pullOptionsForTest(httpServer.URL, noopHandler{}),
	})
	errCh := runManager(ctx, manager)
	waitClosed(t, helloAccepted)
	before := pulls.Load()
	time.Sleep(80 * time.Millisecond)
	after := pulls.Load()
	cancel()
	waitManager(t, errCh)
	if after != before {
		t.Fatalf("Pull ran while WebSocket healthy: before=%d after=%d", before, after)
	}
}

func TestManagerEnablesPullAfterWebSocketDisconnect(t *testing.T) {
	pulled := make(chan struct{})
	var wsConnections atomic.Int64
	wsServer := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		if wsConnections.Add(1) > 1 {
			_ = conn.Close(websocket.StatusNormalClosure, "test done")
			return
		}
		verifyHandshake(t, r)
		_ = conn.Close(websocket.StatusNormalClosure, "test disconnect")
	})
	defer wsServer.Close()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeOnce(pulled)
		writeJSON(t, w, PullResponsePayload{Commands: nil})
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	manager := mustManager(t, ManagerOptions{
		WSEnabled:   true,
		PullEnabled: true,
		WS:          wsOptionsForTest(wsServer.URL, noopHandler{}),
		Pull:        pullOptionsForTest(httpServer.URL, noopHandler{}),
	})
	errCh := runManager(ctx, manager)
	waitClosed(t, pulled)
	cancel()
	waitManager(t, errCh)
}

func TestManagerDoesNotRunPullWhenPullDisabled(t *testing.T) {
	helloAccepted := make(chan struct{})
	wsServer := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		closeOnce(helloAccepted)
		<-r.Context().Done()
	})
	defer wsServer.Close()

	var pulls atomic.Int64
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pulls.Add(1)
		writeJSON(t, w, PullResponsePayload{Commands: nil})
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	manager := mustManager(t, ManagerOptions{
		WSEnabled:   true,
		PullEnabled: false,
		WS:          wsOptionsForTest(wsServer.URL, noopHandler{}),
		Pull:        pullOptionsForTest(httpServer.URL, noopHandler{}),
	})
	errCh := runManager(ctx, manager)
	waitClosed(t, helloAccepted)
	time.Sleep(50 * time.Millisecond)
	cancel()
	waitManager(t, errCh)
	if pulls.Load() != 0 {
		t.Fatalf("Pull ran while disabled: %d", pulls.Load())
	}
}

func pullOptionsForTest(baseURL string, handler CommandHandler) PullClientOptions {
	opts := mustPullOptions(baseURL, handler)
	opts.PullInterval = 20 * time.Millisecond
	opts.ReconnectMinDelay = 10 * time.Millisecond
	opts.ReconnectMaxDelay = 20 * time.Millisecond
	return opts
}

func wsOptionsForTest(baseURL string, handler CommandHandler) WSClientOptions {
	return WSClientOptions{
		BaseURL:           baseURL,
		WSPath:            testWSPath,
		RequestTimeout:    500 * time.Millisecond,
		HeartbeatInterval: time.Hour,
		MaxParallelJobs:   1,
		MaxOutputBytes:    1024,
		DeviceID:          testDeviceID,
		TenantID:          testTenantID,
		DeviceSecret:      []byte(testSecret),
		AgentVersion:      "v0.1.0",
		DeviceName:        "ufi-test",
		AllowedActions:    []string{"status.snapshot"},
		Handler:           handler,
		ReconnectMinDelay: 10 * time.Millisecond,
		ReconnectMaxDelay: 20 * time.Millisecond,
	}
}

func mustPullOptions(baseURL string, handler CommandHandler) PullClientOptions {
	return PullClientOptions{
		BaseURL:           baseURL,
		PullPath:          testWSPathPull,
		ResultPath:        testResultPath,
		RequestTimeout:    500 * time.Millisecond,
		PullInterval:      20 * time.Millisecond,
		MaxResponseBytes:  DefaultMaxMessageBytes,
		DeviceID:          testDeviceID,
		TenantID:          testTenantID,
		DeviceSecret:      []byte(testSecret),
		AgentVersion:      "v0.1.0",
		DeviceName:        "ufi-test",
		Handler:           handler,
		ReconnectMinDelay: 10 * time.Millisecond,
		ReconnectMaxDelay: 20 * time.Millisecond,
	}
}

func mustManager(t *testing.T, opts ManagerOptions) *Manager {
	t.Helper()
	if opts.Dispatcher == nil {
		opts.Dispatcher = testDispatcher(t)
	}
	manager, err := NewManager(opts)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func runManager(ctx context.Context, manager *Manager) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.Run(ctx)
	}()
	return errCh
}

func waitManager(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("manager.Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop")
	}
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

var _ = protocol.MessageTypeResult
