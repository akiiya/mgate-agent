package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/auth"
	"mgate-agent/internal/commands"
	"mgate-agent/internal/outbox"
	"mgate-agent/internal/protocol"
	"mgate-agent/internal/runner"

	"nhooyr.io/websocket"
)

const (
	testDeviceID = "dev_test"
	testTenantID = "tenant_test"
	testSecret   = "test-secret"
	testWSPath   = "/api/agent/v1/ws"
)

func TestBuildWebSocketURL(t *testing.T) {
	cases := []struct {
		name string
		base string
		path string
		want string
	}{
		{name: "https to wss", base: "https://mgate.example.com", path: testWSPath, want: "wss://mgate.example.com/api/agent/v1/ws"},
		{name: "http to ws", base: "http://127.0.0.1:8080", path: testWSPath, want: "ws://127.0.0.1:8080/api/agent/v1/ws"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildWebSocketURL(tc.base, tc.path)
			if err != nil {
				t.Fatalf("BuildWebSocketURL() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("BuildWebSocketURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandshakeHeaderVerifiesHMAC(t *testing.T) {
	client := mustClient(t, WSClientOptions{Handler: noopHandler{}})
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	header, err := client.HandshakeHeader(now)
	if err != nil {
		t.Fatalf("HandshakeHeader() error = %v", err)
	}
	if header.Get("X-MGate-Device-ID") != testDeviceID {
		t.Fatalf("device header = %q", header.Get("X-MGate-Device-ID"))
	}
	if header.Get("X-MGate-Signature") == "" {
		t.Fatal("signature header is empty")
	}
	if !auth.Verify([]byte(testSecret), auth.SignInput{
		Method:    http.MethodGet,
		Path:      testWSPath,
		Timestamp: header.Get("X-MGate-Timestamp"),
		Nonce:     header.Get("X-MGate-Nonce"),
		Body:      nil,
	}, header.Get("X-MGate-Signature")) {
		t.Fatal("handshake signature did not verify")
	}
}

func TestClientSendsHelloAndHandlesHelloAck(t *testing.T) {
	done := make(chan struct{})
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		env := readEnvelopeFromConn(t, conn)
		if env.Type != protocol.MessageTypeHello {
			t.Fatalf("first message type = %s, want hello", env.Type)
		}
		var hello protocol.HelloPayload
		unmarshalPayload(t, env.Payload, &hello)
		if hello.DeviceID != testDeviceID || hello.TenantID != testTenantID {
			t.Fatalf("unexpected hello payload: %+v", hello)
		}
		if len(hello.Capabilities.Actions) == 0 {
			t.Fatal("hello should include allowed actions")
		}
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{BaseURL: server.URL, Handler: noopHandler{}})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
}

func TestClientSendsHeartbeat(t *testing.T) {
	done := make(chan struct{})
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		env := readEnvelopeFromConn(t, conn)
		if env.Type != protocol.MessageTypeHeartbeat {
			t.Fatalf("message type = %s, want heartbeat", env.Type)
		}
		var hb protocol.HeartbeatPayload
		unmarshalPayload(t, env.Payload, &hb)
		if hb.DeviceID != testDeviceID {
			t.Fatalf("heartbeat device_id = %q", hb.DeviceID)
		}
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{
		BaseURL:           server.URL,
		HeartbeatInterval: 20 * time.Millisecond,
		Handler:           noopHandler{},
	})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
}

func TestClientProcessesCommandAndReturnsResult(t *testing.T) {
	done := make(chan struct{})
	handler := realCommandHandler(t, []string{"status.snapshot"})
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		writeEnvelopeToConn(t, conn, commandEnvelope(testDeviceID, "cmd_001", "status.snapshot"))

		seen := readUntilTypes(t, conn, protocol.MessageTypeAck, protocol.MessageTypeResult)
		var result protocol.ResultPayload
		unmarshalPayload(t, seen[protocol.MessageTypeResult].Payload, &result)
		if result.State != protocol.CommandSucceeded {
			t.Fatalf("result state = %s, want succeeded: %+v", result.State, result)
		}
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{BaseURL: server.URL, Handler: handler})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
}

func TestClientReturnsRejectedResultForUnknownAction(t *testing.T) {
	done := make(chan struct{})
	handler := realCommandHandler(t, []string{"status.snapshot"})
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		writeEnvelopeToConn(t, conn, commandEnvelope(testDeviceID, "cmd_002", "unknown.action"))

		seen := readUntilTypes(t, conn, protocol.MessageTypeAck, protocol.MessageTypeResult)
		var result protocol.ResultPayload
		unmarshalPayload(t, seen[protocol.MessageTypeResult].Payload, &result)
		if result.State != protocol.CommandRejected || result.ErrorCode != "unknown_action" {
			t.Fatalf("unexpected rejected result: %+v", result)
		}
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{BaseURL: server.URL, Handler: handler})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
}

func TestDeviceMismatchDoesNotCallHandler(t *testing.T) {
	done := make(chan struct{})
	handler := &countingHandler{}
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		writeEnvelopeToConn(t, conn, commandEnvelope("other_device", "cmd_003", "status.snapshot"))

		env := readEnvelopeFromConn(t, conn)
		if env.Type != protocol.MessageTypeResult {
			t.Fatalf("message type = %s, want result", env.Type)
		}
		var result protocol.ResultPayload
		unmarshalPayload(t, env.Payload, &result)
		if result.State != protocol.CommandRejected || result.ErrorCode != "device_mismatch" {
			t.Fatalf("unexpected result: %+v", result)
		}
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{BaseURL: server.URL, Handler: handler})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
	if handler.calls.Load() != 0 {
		t.Fatalf("handler was called %d times", handler.calls.Load())
	}
}

func TestLongCommandDoesNotBlockReadLoop(t *testing.T) {
	done := make(chan struct{})
	release := make(chan struct{})
	handler := blockingHandler{release: release}
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		writeEnvelopeToConn(t, conn, commandEnvelope(testDeviceID, "cmd_101", "status.snapshot"))
		firstAck := readEnvelopeFromConn(t, conn)
		if firstAck.Type != protocol.MessageTypeAck {
			t.Fatalf("first response = %s, want ack", firstAck.Type)
		}

		writeEnvelopeToConn(t, conn, commandEnvelope(testDeviceID, "cmd_102", "status.snapshot"))
		secondAck := readEnvelopeFromConn(t, conn)
		if secondAck.Type != protocol.MessageTypeAck {
			t.Fatalf("read loop blocked; second response = %s, want ack", secondAck.Type)
		}
		close(release)
		_ = readUntilTypes(t, conn, protocol.MessageTypeResult)
		_ = readUntilTypes(t, conn, protocol.MessageTypeResult)
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{
		BaseURL:          server.URL,
		Handler:          handler,
		MaxParallelJobs:  1,
		CommandQueueSize: 2,
	})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
}

func TestClientReconnectsAfterConnectionClose(t *testing.T) {
	done := make(chan struct{})
	var connections atomic.Int64
	server := newFakeServer(t, func(t *testing.T, conn *websocket.Conn, r *http.Request) {
		verifyHandshake(t, r)
		n := connections.Add(1)
		_ = readEnvelopeFromConn(t, conn)
		writeEnvelopeToConn(t, conn, helloAckEnvelope(true))
		if n == 1 {
			_ = conn.Close(websocket.StatusNormalClosure, "test reconnect")
			return
		}
		close(done)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := mustClient(t, WSClientOptions{
		BaseURL:           server.URL,
		Handler:           noopHandler{},
		ReconnectMinDelay: 10 * time.Millisecond,
		ReconnectMaxDelay: 20 * time.Millisecond,
	})
	errCh := runClient(t, ctx, client)
	waitClosed(t, done)
	cancel()
	waitClient(t, errCh)
	if connections.Load() < 2 {
		t.Fatalf("connections = %d, want at least 2", connections.Load())
	}
}

func TestClientStopsReconnectOnContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := mustClient(t, WSClientOptions{
		BaseURL:           server.URL,
		Handler:           noopHandler{},
		ReconnectMinDelay: 10 * time.Millisecond,
		ReconnectMaxDelay: 20 * time.Millisecond,
	})
	errCh := runClient(t, ctx, client)
	time.Sleep(30 * time.Millisecond)
	cancel()
	waitClient(t, errCh)
}

type noopHandler struct{}

func (noopHandler) Handle(_ context.Context, cmd protocol.CommandPayload) protocol.ResultPayload {
	now := time.Now().UTC()
	return protocol.ResultPayload{
		CommandID: cmd.CommandID,
		Action:    cmd.Action,
		State:     protocol.CommandSucceeded,
		ExitCode:  0,
		StartedAt: now,
		EndedAt:   now,
	}
}

type countingHandler struct {
	calls atomic.Int64
}

func (h *countingHandler) Handle(_ context.Context, cmd protocol.CommandPayload) protocol.ResultPayload {
	h.calls.Add(1)
	return noopHandler{}.Handle(context.Background(), cmd)
}

type blockingHandler struct {
	release <-chan struct{}
}

func (h blockingHandler) Handle(ctx context.Context, cmd protocol.CommandPayload) protocol.ResultPayload {
	select {
	case <-ctx.Done():
	case <-h.release:
	}
	return noopHandler{}.Handle(ctx, cmd)
}

func realCommandHandler(t *testing.T, allow []string) *commands.Handler {
	t.Helper()
	registry, err := actions.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	handler, err := commands.NewHandler(commands.Options{
		Registry:     registry,
		AllowActions: allow,
		Runner: func(_ context.Context, req runner.Request) protocol.ResultPayload {
			now := time.Now().UTC()
			return protocol.ResultPayload{
				CommandID: req.CommandID,
				Action:    req.Spec.Name,
				State:     protocol.CommandSucceeded,
				ExitCode:  0,
				Stdout:    "ok",
				StartedAt: now,
				EndedAt:   now,
			}
		},
		MGatePath:      "/usr/local/bin/mgate.sh",
		WorkDir:        "/var/lib/mgate-agent",
		MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func mustClient(t *testing.T, opts WSClientOptions) *WSClient {
	t.Helper()
	if opts.BaseURL == "" {
		opts.BaseURL = "http://127.0.0.1:1"
	}
	opts.WSPath = testWSPath
	opts.RequestTimeout = 500 * time.Millisecond
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Hour
	}
	opts.DeviceID = testDeviceID
	opts.TenantID = testTenantID
	opts.DeviceSecret = []byte(testSecret)
	opts.AgentVersion = "v0.1.0"
	opts.DeviceName = "ufi-test"
	opts.AllowedActions = []string{"status.snapshot"}
	opts.MaxParallelJobs = maxInt(opts.MaxParallelJobs, 1)
	opts.MaxOutputBytes = 1024
	if opts.Dispatcher == nil {
		opts.Dispatcher = testDispatcher(t)
	}
	client, err := NewWSClient(opts)
	if err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
	return client
}

func testDispatcher(t *testing.T) *ResultDispatcher {
	t.Helper()
	store, err := outbox.NewStore(outbox.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	dispatcher, err := NewResultDispatcher(store, nil, nil)
	if err != nil {
		t.Fatalf("NewResultDispatcher() error = %v", err)
	}
	return dispatcher
}

func newFakeServer(t *testing.T, handler func(*testing.T, *websocket.Conn, *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "test done")
		handler(t, conn, r)
	}))
}

func verifyHandshake(t *testing.T, r *http.Request) {
	t.Helper()
	for _, name := range []string{
		"X-MGate-Device-ID",
		"X-MGate-Tenant-ID",
		"X-MGate-Timestamp",
		"X-MGate-Nonce",
		"X-MGate-Signature",
		"X-MGate-Agent-Version",
	} {
		if r.Header.Get(name) == "" {
			t.Fatalf("missing header %s", name)
		}
	}
	if !auth.Verify([]byte(testSecret), auth.SignInput{
		Method:    http.MethodGet,
		Path:      testWSPath,
		Timestamp: r.Header.Get("X-MGate-Timestamp"),
		Nonce:     r.Header.Get("X-MGate-Nonce"),
		Body:      nil,
	}, r.Header.Get("X-MGate-Signature")) {
		t.Fatal("handshake HMAC verification failed")
	}
}

func runClient(t *testing.T, ctx context.Context, client *WSClient) <-chan error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx)
	}()
	return errCh
}

func waitClient(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("client.Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not stop")
	}
}

func waitClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for fake server")
	}
}

func readEnvelopeFromConn(t *testing.T, conn *websocket.Conn) protocol.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("message type = %v, want text", typ)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("Unmarshal(envelope) error = %v", err)
	}
	return env
}

func writeEnvelopeToConn(t *testing.T, conn *websocket.Conn, env protocol.Envelope) {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal(envelope) error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}
}

func readUntilTypes(t *testing.T, conn *websocket.Conn, types ...protocol.MessageType) map[protocol.MessageType]protocol.Envelope {
	t.Helper()
	want := make(map[protocol.MessageType]struct{}, len(types))
	for _, typ := range types {
		want[typ] = struct{}{}
	}
	seen := make(map[protocol.MessageType]protocol.Envelope, len(types))
	deadline := time.After(2 * time.Second)
	for len(seen) < len(types) {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for message types: %+v", types)
		default:
		}
		env := readEnvelopeFromConn(t, conn)
		if _, ok := want[env.Type]; ok {
			seen[env.Type] = env
		}
	}
	return seen
}

func helloAckEnvelope(accepted bool) protocol.Envelope {
	payload, _ := json.Marshal(protocol.HelloAckPayload{
		Accepted:   accepted,
		ServerTime: time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Message:    "ok",
	})
	return protocol.Envelope{
		Version:   envelopeVersion,
		Type:      protocol.MessageTypeHelloAck,
		MessageID: "msg_hello_ack",
		DeviceID:  testDeviceID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}

func commandEnvelope(deviceID, commandID, action string) protocol.Envelope {
	payload, _ := json.Marshal(protocol.CommandPayload{
		CommandID: commandID,
		Action:    action,
		Args:      map[string]any{},
		IssuedAt:  time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
	})
	return protocol.Envelope{
		Version:       envelopeVersion,
		Type:          protocol.MessageTypeCommand,
		MessageID:     "msg_" + commandID,
		CorrelationID: commandID,
		DeviceID:      deviceID,
		Timestamp:     time.Now().UTC(),
		Payload:       payload,
	}
}

func unmarshalPayload(t *testing.T, raw json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
