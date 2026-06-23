package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
	"mgate-agent/internal/runner"
)

func TestHandlerProcessesAllowedAction(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	handler := newTestHandler(t, []string{"status.snapshot"}, auditPath, fakeSuccessRunner)

	result := handler.Handle(context.Background(), protocol.CommandPayload{
		CommandID: "cmd_ok",
		Action:    "status.snapshot",
	})

	if result.State != protocol.CommandSucceeded {
		t.Fatalf("state = %s, want succeeded: %+v", result.State, result)
	}
	content := readFile(t, auditPath)
	for _, event := range []string{
		audit.EventCommandReceived,
		audit.EventCommandStarted,
		audit.EventCommandFinished,
	} {
		if !strings.Contains(content, event) {
			t.Fatalf("audit missing %s: %s", event, content)
		}
	}
}

func TestHandlerRejectsUnknownAction(t *testing.T) {
	result := newTestHandler(t, []string{"status.snapshot"}, "", fakeSuccessRunner).Handle(context.Background(), protocol.CommandPayload{
		CommandID: "cmd_unknown",
		Action:    "unknown.action",
	})

	assertRejected(t, result, "unknown_action")
}

func TestHandlerRejectsActionNotAllowedByConfig(t *testing.T) {
	result := newTestHandler(t, []string{"status.snapshot"}, "", fakeSuccessRunner).Handle(context.Background(), protocol.CommandPayload{
		CommandID: "cmd_not_allowed",
		Action:    "gateway.status",
	})

	assertRejected(t, result, "action_not_allowed")
}

func TestHandlerRejectsInvalidCommandID(t *testing.T) {
	result := newTestHandler(t, []string{"status.snapshot"}, "", fakeSuccessRunner).Handle(context.Background(), protocol.CommandPayload{
		CommandID: "../bad",
		Action:    "status.snapshot",
	})

	assertRejected(t, result, "invalid_command_id")
}

func TestHandlerRejectsDuplicateCommandID(t *testing.T) {
	handler := newTestHandler(t, []string{"status.snapshot"}, "", fakeSuccessRunner)
	cmd := protocol.CommandPayload{CommandID: "cmd_duplicate", Action: "status.snapshot"}

	first := handler.Handle(context.Background(), cmd)
	if first.State != protocol.CommandSucceeded {
		t.Fatalf("first state = %s, want succeeded", first.State)
	}
	second := handler.Handle(context.Background(), cmd)
	assertRejected(t, second, "duplicate_command")
}

func TestHandlerRejectsInvalidArgs(t *testing.T) {
	result := newTestHandler(t, []string{"gateway.start"}, "", fakeSuccessRunner).Handle(context.Background(), protocol.CommandPayload{
		CommandID: "cmd_bad_args",
		Action:    "gateway.start",
		Args:      map[string]any{"country": "US; rm -rf /"},
	})

	assertRejected(t, result, "invalid_args")
}

func TestHandlerAuditRedactsSensitiveArgs(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	handler := newTestHandler(t, []string{"wlan.switch.safe"}, auditPath, fakeSuccessRunner)

	result := handler.Handle(context.Background(), protocol.CommandPayload{
		CommandID: "cmd_wifi",
		Action:    "wlan.switch.safe",
		Args: map[string]any{
			"ssid": "home",
			"psk":  "verysecret",
		},
	})
	if result.State != protocol.CommandSucceeded {
		t.Fatalf("state = %s, want succeeded: %+v", result.State, result)
	}

	content := readFile(t, auditPath)
	if strings.Contains(content, "verysecret") {
		t.Fatalf("audit leaked psk: %s", content)
	}
	if !strings.Contains(content, audit.RedactedValue) {
		t.Fatalf("audit missing redaction marker: %s", content)
	}
}

func TestHandlerRejectsTimeoutLongerThanActionLimit(t *testing.T) {
	timeoutSec := 11
	result := newTestHandler(t, []string{"status.snapshot"}, "", fakeSuccessRunner).Handle(context.Background(), protocol.CommandPayload{
		CommandID:  "cmd_timeout",
		Action:     "status.snapshot",
		TimeoutSec: &timeoutSec,
	})

	assertRejected(t, result, "timeout_exceeded")
}

func TestHandlerRejectsExplicitNonPositiveTimeout(t *testing.T) {
	timeoutSec := 0
	result := newTestHandler(t, []string{"status.snapshot"}, "", fakeSuccessRunner).Handle(context.Background(), protocol.CommandPayload{
		CommandID:  "cmd_timeout_zero",
		Action:     "status.snapshot",
		TimeoutSec: &timeoutSec,
	})

	assertRejected(t, result, "timeout_exceeded")
}

func fakeSuccessRunner(_ context.Context, req runner.Request) protocol.ResultPayload {
	started := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	ended := started.Add(10 * time.Millisecond)
	return protocol.ResultPayload{
		CommandID:  req.CommandID,
		Action:     req.Spec.Name,
		State:      protocol.CommandSucceeded,
		ExitCode:   0,
		StartedAt:  started,
		EndedAt:    ended,
		DurationMS: 10,
	}
}

func newTestHandler(t *testing.T, allow []string, auditPath string, run RunnerFunc) *Handler {
	t.Helper()
	registry, err := actions.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	var writer *audit.Writer
	if auditPath != "" {
		writer = audit.NewWriter(auditPath)
	}
	handler, err := NewHandler(Options{
		Registry:       registry,
		AllowActions:   allow,
		Audit:          writer,
		Dedupe:         NewDedupeStore(DefaultDedupeTTL, DefaultDedupeMaxEntries),
		Runner:         run,
		MGatePath:      "/usr/local/bin/mgate.sh",
		WorkDir:        "/var/lib/mgate-agent",
		MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func assertRejected(t *testing.T, result protocol.ResultPayload, code string) {
	t.Helper()
	if result.State != protocol.CommandRejected {
		t.Fatalf("state = %s, want rejected: %+v", result.State, result)
	}
	if result.ErrorCode != code {
		t.Fatalf("error_code = %q, want %q", result.ErrorCode, code)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
