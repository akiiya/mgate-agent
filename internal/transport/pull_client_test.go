package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mgate-agent/internal/auth"
	"mgate-agent/internal/protocol"
)

func TestBuildHTTPURL(t *testing.T) {
	got, err := BuildHTTPURL("https://mgate.example.com", "/api/agent/v1/pull")
	if err != nil {
		t.Fatalf("BuildHTTPURL() error = %v", err)
	}
	if got != "https://mgate.example.com/api/agent/v1/pull" {
		t.Fatalf("BuildHTTPURL() = %q", got)
	}
}

func TestPullEmptyCommandsDoesNotExecute(t *testing.T) {
	handler := &countingHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		verifyHTTPHMAC(t, r, testWSPathPull, body)
		writeJSON(t, w, PullResponsePayload{ServerTime: time.Now().UTC(), Commands: nil})
	}))
	defer server.Close()

	client := mustPullClient(t, server.URL, handler)
	if err := client.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls.Load())
	}
}

func TestPullCommandPostsResultAndSignsBothRequests(t *testing.T) {
	var pullSigned, resultSigned atomic.Bool
	handler := realCommandHandler(t, []string{"status.snapshot"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		switch r.URL.Path {
		case testWSPathPull:
			verifyHTTPHMAC(t, r, testWSPathPull, body)
			pullSigned.Store(true)
			writeJSON(t, w, PullResponsePayload{
				ServerTime: time.Now().UTC(),
				Commands:   []protocol.Envelope{commandEnvelope(testDeviceID, "cmd_p01", "status.snapshot")},
			})
		case testResultPath:
			verifyHTTPHMAC(t, r, testResultPath, body)
			resultSigned.Store(true)
			var env protocol.Envelope
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("decode result envelope: %v", err)
			}
			var result protocol.ResultPayload
			unmarshalPayload(t, env.Payload, &result)
			if result.State != protocol.CommandSucceeded {
				t.Fatalf("result state = %s, want succeeded", result.State)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := mustPullClient(t, server.URL, handler)
	if err := client.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if !pullSigned.Load() || !resultSigned.Load() {
		t.Fatalf("pullSigned=%v resultSigned=%v", pullSigned.Load(), resultSigned.Load())
	}
}

func TestPullUnknownActionPostsRejectedResult(t *testing.T) {
	handler := realCommandHandler(t, []string{"status.snapshot"})
	resultSeen := make(chan protocol.ResultPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		switch r.URL.Path {
		case testWSPathPull:
			verifyHTTPHMAC(t, r, testWSPathPull, body)
			writeJSON(t, w, PullResponsePayload{Commands: []protocol.Envelope{commandEnvelope(testDeviceID, "cmd_p02", "unknown.action")}})
		case testResultPath:
			verifyHTTPHMAC(t, r, testResultPath, body)
			var env protocol.Envelope
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("decode result envelope: %v", err)
			}
			var result protocol.ResultPayload
			unmarshalPayload(t, env.Payload, &result)
			resultSeen <- result
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := mustPullClient(t, server.URL, handler)
	if err := client.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	result := <-resultSeen
	if result.State != protocol.CommandRejected || result.ErrorCode != "unknown_action" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPullDeviceMismatchDoesNotCallHandler(t *testing.T) {
	handler := &countingHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		switch r.URL.Path {
		case testWSPathPull:
			verifyHTTPHMAC(t, r, testWSPathPull, body)
			writeJSON(t, w, PullResponsePayload{Commands: []protocol.Envelope{commandEnvelope("other_device", "cmd_p03", "status.snapshot")}})
		case testResultPath:
			verifyHTTPHMAC(t, r, testResultPath, body)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := mustPullClient(t, server.URL, handler)
	if err := client.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls.Load())
	}
}

func TestPullBadJSONAndOversizeReturnErrors(t *testing.T) {
	t.Run("bad json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()
		client := mustPullClient(t, server.URL, noopHandler{})
		if err := client.PollOnce(context.Background()); err == nil {
			t.Fatal("PollOnce() expected bad JSON error")
		}
	})

	t.Run("oversize", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"server_time":"2026-06-23T00:00:00Z","commands":[]}`))
		}))
		defer server.Close()
		client := mustPullClient(t, server.URL, noopHandler{})
		client.opts.MaxResponseBytes = 8
		if err := client.PollOnce(context.Background()); err == nil {
			t.Fatal("PollOnce() expected oversize error")
		}
	})
}

func TestPullHTTPNon2xxReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := mustPullClient(t, server.URL, noopHandler{})
	if err := client.PollOnce(context.Background()); err == nil {
		t.Fatal("PollOnce() expected non-2xx error")
	}
}

func TestPullDuplicateCommandIDUsesSharedHandlerDedupe(t *testing.T) {
	handler := realCommandHandler(t, []string{"status.snapshot"})
	results := make(chan protocol.ResultPayload, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		switch r.URL.Path {
		case testWSPathPull:
			verifyHTTPHMAC(t, r, testWSPathPull, body)
			writeJSON(t, w, PullResponsePayload{Commands: []protocol.Envelope{
				commandEnvelope(testDeviceID, "cmd_dup", "status.snapshot"),
				commandEnvelope(testDeviceID, "cmd_dup", "status.snapshot"),
			}})
		case testResultPath:
			verifyHTTPHMAC(t, r, testResultPath, body)
			var env protocol.Envelope
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("decode result envelope: %v", err)
			}
			var result protocol.ResultPayload
			unmarshalPayload(t, env.Payload, &result)
			results <- result
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := mustPullClient(t, server.URL, handler)
	if err := client.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	first := <-results
	second := <-results
	if first.State != protocol.CommandSucceeded || second.ErrorCode != "duplicate_command" {
		t.Fatalf("unexpected duplicate results: first=%+v second=%+v", first, second)
	}
}

const (
	testWSPathPull = "/api/agent/v1/pull"
	testResultPath = "/api/agent/v1/result"
)

func mustPullClient(t *testing.T, baseURL string, handler CommandHandler) *PullClient {
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
		Dispatcher:        testDispatcher(t),
	})
	if err != nil {
		t.Fatalf("NewPullClient() error = %v", err)
	}
	return client
}

func verifyHTTPHMAC(t *testing.T, r *http.Request, path string, body []byte) {
	t.Helper()
	if r.URL.RawQuery != "" {
		t.Fatalf("URL query must not contain auth material: %s", r.URL.RawQuery)
	}
	if !auth.Verify([]byte(testSecret), auth.SignInput{
		Method:    r.Method,
		Path:      path,
		Timestamp: r.Header.Get("X-MGate-Timestamp"),
		Nonce:     r.Header.Get("X-MGate-Nonce"),
		Body:      body,
	}, r.Header.Get("X-MGate-Signature")) {
		t.Fatalf("HMAC verification failed for %s", path)
	}
}

func readRequestBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ReadAll(body) error = %v", err)
	}
	return body
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("Encode(response) error = %v", err)
	}
}
