package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mgate-agent/internal/outbox"
	"mgate-agent/internal/protocol"
)

func TestDispatcherDeletesAfterWebSocketSuccess(t *testing.T) {
	dispatcher, store := dispatcherWithStore(t)
	dispatcher.SetWebSocketSender(func(context.Context, outbox.Record) error {
		return nil
	})
	if err := dispatcher.Enqueue(context.Background(), resultEnvelopeForTransport(t, "cmd_ws_ok")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if got := store.PendingCount(); got != 0 {
		t.Fatalf("pending count = %d, want 0", got)
	}
}

func TestDispatcherKeepsRecordAfterWebSocketFailure(t *testing.T) {
	dispatcher, store := dispatcherWithStore(t)
	dispatcher.SetWebSocketSender(func(context.Context, outbox.Record) error {
		return errors.New("websocket closed")
	})
	if err := dispatcher.Enqueue(context.Background(), resultEnvelopeForTransport(t, "cmd_ws_fail")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	records, err := store.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(records) != 1 || records[0].Attempts != 1 || records[0].LastError == "" {
		t.Fatalf("record was not kept with failure state: %+v", records)
	}
}

func TestDispatcherDeletesAfterPullHTTPSuccess(t *testing.T) {
	var posted atomic.Bool
	dispatcher, store := dispatcherWithStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		verifyHTTPHMAC(t, r, testResultPath, body)
		posted.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	_ = mustPullClientWithDispatcher(t, server.URL, noopHandler{}, dispatcher)

	if err := dispatcher.Enqueue(context.Background(), resultEnvelopeForTransport(t, "cmd_pull_ok")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if !posted.Load() {
		t.Fatal("result was not posted")
	}
	if got := store.PendingCount(); got != 0 {
		t.Fatalf("pending count = %d, want 0", got)
	}
}

func TestDispatcherKeepsRecordAfterPullHTTPFailure(t *testing.T) {
	dispatcher, store := dispatcherWithStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_ = mustPullClientWithDispatcher(t, server.URL, noopHandler{}, dispatcher)

	if err := dispatcher.Enqueue(context.Background(), resultEnvelopeForTransport(t, "cmd_pull_fail")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	records, err := store.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(records) != 1 || records[0].Attempts != 1 {
		t.Fatalf("record was not kept after HTTP failure: %+v", records)
	}
}

func TestDispatcherSendsPendingRecordAfterStartup(t *testing.T) {
	dir := t.TempDir()
	initialStore, err := outbox.NewStore(outbox.Options{Dir: dir})
	if err != nil {
		t.Fatalf("NewStore(initial) error = %v", err)
	}
	if _, err := initialStore.Save(resultEnvelopeForTransport(t, "cmd_pending")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reloadedStore, err := outbox.NewStore(outbox.Options{Dir: dir})
	if err != nil {
		t.Fatalf("NewStore(reloaded) error = %v", err)
	}
	dispatcher, err := NewResultDispatcher(reloadedStore, nil, nil)
	if err != nil {
		t.Fatalf("NewResultDispatcher() error = %v", err)
	}
	dispatcher.SetWebSocketSender(func(context.Context, outbox.Record) error {
		return nil
	})
	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if got := reloadedStore.PendingCount(); got != 0 {
		t.Fatalf("pending count = %d, want 0", got)
	}
}

func TestDispatcherStopsOnContextCancel(t *testing.T) {
	dispatcher, _ := dispatcherWithStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := dispatcher.Run(ctx, time.Millisecond); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestDispatcherAvoidsConcurrentDuplicateSend(t *testing.T) {
	dispatcher, store := dispatcherWithStore(t)
	if _, err := store.Save(resultEnvelopeForTransport(t, "cmd_once")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var sends atomic.Int64
	dispatcher.SetWebSocketSender(func(context.Context, outbox.Record) error {
		sends.Add(1)
		closeOnce(started)
		<-release
		return nil
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = dispatcher.DispatchOnce(context.Background())
	}()
	<-started
	go func() {
		defer wg.Done()
		_ = dispatcher.DispatchOnce(context.Background())
	}()
	close(release)
	wg.Wait()
	if sends.Load() != 1 {
		t.Fatalf("sends = %d, want 1", sends.Load())
	}
}

func dispatcherWithStore(t *testing.T) (*ResultDispatcher, *outbox.Store) {
	t.Helper()
	store, err := outbox.NewStore(outbox.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	dispatcher, err := NewResultDispatcher(store, nil, nil)
	if err != nil {
		t.Fatalf("NewResultDispatcher() error = %v", err)
	}
	return dispatcher, store
}

func mustPullClientWithDispatcher(t *testing.T, baseURL string, handler CommandHandler, dispatcher *ResultDispatcher) *PullClient {
	t.Helper()
	client, err := NewPullClient(PullClientOptions{
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
		Dispatcher:        dispatcher,
	})
	if err != nil {
		t.Fatalf("NewPullClient() error = %v", err)
	}
	return client
}

func resultEnvelopeForTransport(t *testing.T, commandID string) protocol.Envelope {
	t.Helper()
	payload := protocol.ResultPayload{
		CommandID: commandID,
		Action:    "status.snapshot",
		State:     protocol.CommandSucceeded,
		ExitCode:  0,
		Stdout:    "ok",
		StartedAt: time.Now().UTC(),
		EndedAt:   time.Now().UTC(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(payload) error = %v", err)
	}
	return protocol.Envelope{
		Version:       envelopeVersion,
		Type:          protocol.MessageTypeResult,
		MessageID:     "msg_" + commandID,
		CorrelationID: commandID,
		DeviceID:      testDeviceID,
		Timestamp:     time.Now().UTC(),
		Payload:       data,
	}
}
