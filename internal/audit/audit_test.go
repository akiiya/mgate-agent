package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditWriterRedactsSensitiveArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "audit.jsonl")
	writer := NewWriter(path)

	err := writer.Write(Event{
		Time:      time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Event:     EventCommandFinished,
		CommandID: "cmd_xxx",
		Action:    "wlan.switch.safe",
		Args: map[string]any{
			"ssid": "home",
			"psk":  "supersecret",
			"nested": map[string]any{
				"api_token": "token-value",
			},
		},
		State:      "succeeded",
		ExitCode:   0,
		DurationMS: 100,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	line := string(data)
	for _, secret := range []string{"supersecret", "token-value"} {
		if strings.Contains(line, secret) {
			t.Fatalf("audit leaked secret %q: %s", secret, line)
		}
	}
	if !strings.Contains(line, RedactedValue) {
		t.Fatalf("audit did not contain redacted marker: %s", line)
	}
	if strings.Contains(line, "stdout") || strings.Contains(line, "stderr") {
		t.Fatalf("audit should not contain stdout/stderr: %s", line)
	}
}

func TestRedactTextRedactsSensitiveLines(t *testing.T) {
	input := "normal line\napi token=abc\npassword: secret\nstill ok"
	got := RedactText(input)
	for _, secret := range []string{"abc", "password: secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("RedactText leaked %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "normal line") || !strings.Contains(got, "still ok") {
		t.Fatalf("RedactText removed non-sensitive lines: %q", got)
	}
}
