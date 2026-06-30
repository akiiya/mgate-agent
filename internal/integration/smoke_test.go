package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"mgate-agent/internal/app"
	"mgate-agent/internal/auth"
	"mgate-agent/internal/config"
	"mgate-agent/internal/identity"
	"mgate-agent/internal/outbox"
	"mgate-agent/internal/protocol"
	"mgate-agent/internal/transport"

	"gopkg.in/yaml.v3"
	"nhooyr.io/websocket"
)

const (
	smokeDeviceID = "dev_smoke"
	smokeTenantID = "tenant_smoke"
	smokeSecret   = "smoke-secret"
)

func TestSmokeWebSocketCommandResult(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "counter.txt")
	t.Setenv("MGATE_COUNTER", counter)

	resultSeen := make(chan protocol.ResultPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/v1/ws" {
			http.NotFound(w, r)
			return
		}
		verifyWSHMAC(t, r)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("Accept() error = %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "smoke done")

		hello := readWSEnvelope(t, conn)
		if hello.Type != protocol.MessageTypeHello {
			t.Fatalf("first message = %s, want hello", hello.Type)
		}
		writeWSEnvelope(t, conn, helloAck())
		writeWSEnvelope(t, conn, commandEnvelope("cmd_ws_001"))

		for {
			env := readWSEnvelope(t, conn)
			if env.Type != protocol.MessageTypeResult {
				continue
			}
			var result protocol.ResultPayload
			unmarshalPayload(t, env.Payload, &result)
			resultSeen <- result
			return
		}
	}))
	defer server.Close()

	cfgPath := writeSmokeRuntime(t, server.URL, true, true, counter)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runAgent(t, ctx, cfgPath)
	result := waitResult(t, resultSeen)
	cancel()
	waitAgent(t, errCh)

	if result.State != protocol.CommandSucceeded || result.Action != "status.snapshot" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := readCounter(t, counter); got != 1 {
		t.Fatalf("mgate execution count = %d, want 1", got)
	}
}

func TestSmokePullFallbackResultAndDedupe(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "counter.txt")
	t.Setenv("MGATE_COUNTER", counter)
	results := make(chan protocol.ResultPayload, 2)
	var pulled atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		switch r.URL.Path {
		case "/api/agent/v1/pull":
			verifyHTTPHMAC(t, r, "/api/agent/v1/pull", body)
			commands := []protocol.Envelope{}
			if pulled.CompareAndSwap(false, true) {
				commands = []protocol.Envelope{
					commandEnvelope("cmd_pull_001"),
					commandEnvelope("cmd_pull_001"),
				}
			}
			writeJSON(t, w, transport.PullResponsePayload{ServerTime: time.Now().UTC(), Commands: commands})
		case "/api/agent/v1/result":
			verifyHTTPHMAC(t, r, "/api/agent/v1/result", body)
			var env protocol.Envelope
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("decode result envelope: %v", err)
			}
			var result protocol.ResultPayload
			unmarshalPayload(t, env.Payload, &result)
			results <- result
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfgPath := writeSmokeRuntime(t, server.URL, false, true, counter)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runAgent(t, ctx, cfgPath)
	first := waitResult(t, results)
	second := waitResult(t, results)
	cancel()
	waitAgent(t, errCh)

	if first.State != protocol.CommandSucceeded || second.ErrorCode != "duplicate_command" {
		t.Fatalf("unexpected pull results: first=%+v second=%+v", first, second)
	}
	if got := readCounter(t, counter); got != 1 {
		t.Fatalf("duplicate command executed %d times, want 1", got)
	}
}

func TestSmokeOutboxResendsAfterResultPostRecovery(t *testing.T) {
	var posts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		if r.URL.Path != "/api/agent/v1/result" {
			http.NotFound(w, r)
			return
		}
		verifyHTTPHMAC(t, r, "/api/agent/v1/result", body)
		if posts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	outboxDir := t.TempDir()
	store, err := outbox.NewStore(outbox.Options{Dir: outboxDir})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	dispatcher, err := transport.NewResultDispatcher(store, nil, nil)
	if err != nil {
		t.Fatalf("NewResultDispatcher() error = %v", err)
	}
	if _, err := transport.NewPullClient(transport.PullClientOptions{
		BaseURL:        server.URL,
		PullPath:       "/api/agent/v1/pull",
		ResultPath:     "/api/agent/v1/result",
		RequestTimeout: time.Second,
		PullInterval:   time.Second,
		DeviceID:       smokeDeviceID,
		TenantID:       smokeTenantID,
		DeviceSecret:   []byte(smokeSecret),
		AgentVersion:   app.Version,
		DeviceName:     "ufi-smoke",
		Handler:        staticHandler{},
		Dispatcher:     dispatcher,
	}); err != nil {
		t.Fatalf("NewPullClient() error = %v", err)
	}

	env := resultEnvelope("cmd_outbox_001", protocol.CommandSucceeded)
	if err := dispatcher.Enqueue(context.Background(), env); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if store.PendingCount() != 1 {
		t.Fatalf("pending count after failed POST = %d, want 1", store.PendingCount())
	}
	forceOutboxDue(t, outboxDir, store)
	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if store.PendingCount() != 0 {
		t.Fatalf("pending count after recovery = %d, want 0", store.PendingCount())
	}
	if posts.Load() != 2 {
		t.Fatalf("POST attempts = %d, want 2", posts.Load())
	}
}

type staticHandler struct{}

func (staticHandler) Handle(_ context.Context, cmd protocol.CommandPayload) protocol.ResultPayload {
	now := time.Now().UTC()
	return protocol.ResultPayload{
		CommandID: cmd.CommandID,
		Action:    cmd.Action,
		State:     protocol.CommandSucceeded,
		ExitCode:  0,
		Stdout:    "{}",
		StartedAt: now,
		EndedAt:   now,
	}
}

func runAgent(t *testing.T, ctx context.Context, cfgPath string) <-chan error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx, app.Options{ConfigPath: cfgPath})
	}()
	return errCh
}

func waitAgent(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("agent returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not stop")
	}
}

func waitResult(t *testing.T, ch <-chan protocol.ResultPayload) protocol.ResultPayload {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for result")
		return protocol.ResultPayload{}
	}
}

func writeSmokeRuntime(t *testing.T, baseURL string, wsEnabled, pullEnabled bool, counter string) string {
	t.Helper()
	root := t.TempDir()
	mgate := buildSmokeMGate(t)
	workDir := filepath.Join(root, "work")
	credPath := filepath.Join(root, "credentials.json")
	cred := identity.Credentials{
		DeviceID:      smokeDeviceID,
		TenantID:      smokeTenantID,
		DeviceSecret:  smokeSecret,
		SecretVersion: 1,
		EnrolledAt:    time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		CloudBaseURL:  baseURL,
	}
	credData, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(credentials) error = %v", err)
	}
	if err := os.WriteFile(credPath, credData, 0o600); err != nil {
		t.Fatalf("WriteFile(credentials) error = %v", err)
	}
	_ = os.Chmod(credPath, 0o600)

	cfg := config.Config{
		Cloud: config.CloudConfig{
			BaseURL:           baseURL,
			WSPath:            "/api/agent/v1/ws",
			PullPath:          "/api/agent/v1/pull",
			ResultPath:        "/api/agent/v1/result",
			StatusPath:        "/api/agent/v1/status",
			RequestTimeoutSec: 1,
			PullIntervalSec:   1,
			WSEnabled:         wsEnabled,
			PullEnabled:       pullEnabled,
		},
		Agent: config.AgentConfig{
			DeviceName:               "ufi-smoke",
			MGatePath:                mgate,
			WorkDir:                  workDir,
			HeartbeatIntervalSec:     5,
			StatusIntervalSec:        10,
			DefaultCommandTimeoutSec: 30,
			LongCommandTimeoutSec:    180,
			MaxParallelJobs:          1,
			MaxOutputBytes:           4096,
		},
		Security: config.SecurityConfig{
			CredentialsFile: credPath,
			AllowActions:    []string{"status.snapshot"},
			ClockSkewSec:    300,
			StrictWhitelist: true,
		},
		Logging: config.LoggingConfig{
			Level:     "error",
			AuditFile: filepath.Join(root, "logs", "audit.jsonl"),
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal(config) error = %v", err)
	}
	cfgPath := filepath.Join(root, "agent.yaml")
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if counter != "" {
		t.Setenv("MGATE_COUNTER", counter)
	}
	return cfgPath
}

func buildSmokeMGate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "fake-mgate")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	source := `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	args := strings.Join(os.Args[1:], " ")
	switch args {
	case "capabilities-json":
		fmt.Println(` + "`" + `{"schema_version":1,"component":"mgate","agent_contract":{"safe_poll_command":"agent-snapshot"}}` + "`" + `)
		return
	case "agent-snapshot":
		fmt.Println(` + "`" + `{"schema_version":1,"component":"mgate","mode":"nat","overall_health":"healthy","wifi":{"connected":true},"ap":{"enabled":true},"gateway":{"state":"ok"},"tproxy":{"enabled":false},"mihomo":{"running":true},"subscription":{"updated":true},"web":{"port":31888}}` + "`" + `)
		return
	}

	counter := os.Getenv("MGATE_COUNTER")
	if counter != "" {
		n := 0
		if data, err := os.ReadFile(counter); err == nil {
			n, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		_ = os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0600)
	}
	fmt.Println(` + "`" + `{"status":"ok"}` + "`" + `)
}
`
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile(fake source) error = %v", err)
	}
	goExe := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goExe += ".exe"
	}
	cmd := exec.Command(goExe, "build", "-o", bin, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake mgate error = %v\n%s", err, out)
	}
	_ = os.Chmod(bin, 0o755)
	return bin
}

func commandEnvelope(commandID string) protocol.Envelope {
	cmd := protocol.CommandPayload{
		CommandID: commandID,
		Action:    "status.snapshot",
		Args:      map[string]any{},
		IssuedAt:  time.Now().UTC(),
	}
	payload, _ := json.Marshal(cmd)
	return protocol.Envelope{
		Version:       "1",
		Type:          protocol.MessageTypeCommand,
		MessageID:     "msg_" + commandID,
		CorrelationID: commandID,
		DeviceID:      smokeDeviceID,
		Timestamp:     time.Now().UTC(),
		Payload:       payload,
	}
}

func helloAck() protocol.Envelope {
	payload, _ := json.Marshal(protocol.HelloAckPayload{
		Accepted:   true,
		ServerTime: time.Now().UTC(),
		Message:    "ok",
	})
	return protocol.Envelope{
		Version:   "1",
		Type:      protocol.MessageTypeHelloAck,
		MessageID: "msg_hello_ack",
		DeviceID:  smokeDeviceID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}

func resultEnvelope(commandID string, state protocol.CommandState) protocol.Envelope {
	payload, _ := json.Marshal(protocol.ResultPayload{
		CommandID: commandID,
		Action:    "status.snapshot",
		State:     state,
		ExitCode:  0,
		Stdout:    "{}",
		StartedAt: time.Now().UTC(),
		EndedAt:   time.Now().UTC(),
	})
	return protocol.Envelope{
		Version:       "1",
		Type:          protocol.MessageTypeResult,
		MessageID:     "msg_" + commandID,
		CorrelationID: commandID,
		DeviceID:      smokeDeviceID,
		Timestamp:     time.Now().UTC(),
		Payload:       payload,
	}
}

func readWSEnvelope(t *testing.T, conn *websocket.Conn) protocol.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("websocket read error: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("websocket message type = %v", typ)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

func writeWSEnvelope(t *testing.T, conn *websocket.Conn, env protocol.Envelope) {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("websocket write error: %v", err)
	}
}

func verifyWSHMAC(t *testing.T, r *http.Request) {
	t.Helper()
	if !auth.Verify([]byte(smokeSecret), auth.SignInput{
		Method:    http.MethodGet,
		Path:      "/api/agent/v1/ws",
		Timestamp: r.Header.Get("X-MGate-Timestamp"),
		Nonce:     r.Header.Get("X-MGate-Nonce"),
		Body:      nil,
	}, r.Header.Get("X-MGate-Signature")) {
		t.Fatal("websocket HMAC verification failed")
	}
}

func verifyHTTPHMAC(t *testing.T, r *http.Request, path string, body []byte) {
	t.Helper()
	if !auth.Verify([]byte(smokeSecret), auth.SignInput{
		Method:    r.Method,
		Path:      path,
		Timestamp: r.Header.Get("X-MGate-Timestamp"),
		Nonce:     r.Header.Get("X-MGate-Nonce"),
		Body:      body,
	}, r.Header.Get("X-MGate-Signature")) {
		t.Fatalf("HTTP HMAC verification failed for %s", path)
	}
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}

func unmarshalPayload(t *testing.T, payload []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(payload, v); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
}

func readCounter(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(counter) error = %v", err)
	}
	n, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("Atoi(counter) error = %v", err)
	}
	return n
}

func forceOutboxDue(t *testing.T, dir string, store *outbox.Store) {
	t.Helper()
	records, err := store.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	for _, record := range records {
		record.NextRetryAt = time.Now().Add(-time.Second)
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent(record) error = %v", err)
		}
		path := filepath.Join(dir, record.RecordID+".json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("WriteFile(record) error = %v", err)
		}
	}
}
