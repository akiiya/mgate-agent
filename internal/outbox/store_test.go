package outbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
)

func TestStoreSaveAndLoadRecord(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir, Options{})
	env := resultEnvelope(t, "cmd_save", protocol.ResultPayload{
		CommandID: "cmd_save",
		Action:    "status.snapshot",
		State:     protocol.CommandSucceeded,
		ExitCode:  0,
		Stdout:    "ok",
	})

	record, err := store.Save(env)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(store.path(record.RecordID))
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	var decoded Record
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("record is not complete JSON: %v", err)
	}
	if decoded.CommandID != "cmd_save" || decoded.Envelope.Type != protocol.MessageTypeResult {
		t.Fatalf("unexpected record: %+v", decoded)
	}

	reloaded := mustStore(t, dir, Options{})
	records, err := reloaded.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(records) != 1 || records[0].CommandID != "cmd_save" {
		t.Fatalf("loaded records = %+v", records)
	}
}

func TestStoreCleansTempAndRenamesCorruptedRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale.json.tmp"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")

	store := mustStore(t, dir, Options{Audit: audit.NewWriter(auditPath)})
	if _, err := os.Stat(filepath.Join(dir, "stale.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("tmp file was not cleaned, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.json.bad")); err != nil {
		t.Fatalf("corrupted file was not renamed to .bad: %v", err)
	}
	if store.PendingCount() != 0 {
		t.Fatalf("pending count = %d, want 0", store.PendingCount())
	}
	auditData, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(auditData), audit.EventOutboxRecordCorrupted) {
		t.Fatalf("audit does not contain corrupted event: %s", auditData)
	}
}

func TestStoreEnforcesMaxRecordsAndBytes(t *testing.T) {
	t.Run("max records", func(t *testing.T) {
		store := mustStore(t, t.TempDir(), Options{MaxRecords: 2})
		for _, id := range []string{"cmd_old", "cmd_mid", "cmd_new"} {
			if _, err := store.Save(resultEnvelope(t, id, protocol.ResultPayload{CommandID: id, State: protocol.CommandSucceeded})); err != nil {
				t.Fatalf("Save(%s) error = %v", id, err)
			}
			time.Sleep(time.Millisecond)
		}
		records, err := store.All()
		if err != nil {
			t.Fatalf("All() error = %v", err)
		}
		if len(records) != 2 || records[0].CommandID != "cmd_mid" || records[1].CommandID != "cmd_new" {
			t.Fatalf("records after max record cleanup = %+v", records)
		}
	})

	t.Run("max bytes", func(t *testing.T) {
		store := mustStore(t, t.TempDir(), Options{MaxBytes: 1})
		if _, err := store.Save(resultEnvelope(t, "cmd_big", protocol.ResultPayload{CommandID: "cmd_big", Stdout: strings.Repeat("x", 128)})); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if got := store.PendingCount(); got != 0 {
			t.Fatalf("pending count after max bytes cleanup = %d, want 0", got)
		}
	})
}

func TestStoreMarkFailureUpdatesRetryState(t *testing.T) {
	store := mustStore(t, t.TempDir(), Options{})
	record, err := store.Save(resultEnvelope(t, "cmd_retry", protocol.ResultPayload{CommandID: "cmd_retry"}))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	updated, err := store.MarkFailure(record.RecordID, os.ErrPermission)
	if err != nil {
		t.Fatalf("MarkFailure() error = %v", err)
	}
	if updated.Attempts != 1 || updated.LastError == "" {
		t.Fatalf("unexpected failure state: %+v", updated)
	}
	minRetry := updated.UpdatedAt.Add(4 * time.Second)
	maxRetry := updated.UpdatedAt.Add(7 * time.Second)
	if updated.NextRetryAt.Before(minRetry) || updated.NextRetryAt.After(maxRetry) {
		t.Fatalf("next retry = %s, want around first retry window", updated.NextRetryAt)
	}
}

func TestStoreRedactsSensitivePayloadFields(t *testing.T) {
	store := mustStore(t, t.TempDir(), Options{})
	payload := map[string]any{
		"command_id":    "cmd_secret",
		"state":         "succeeded",
		"stdout":        "safe line\napi token=abc",
		"stderr":        "password=123",
		"psk":           "wifi-password",
		"device_secret": "device-secret-value",
		"nested": map[string]any{
			"token": "token-value",
		},
	}
	env := resultEnvelope(t, "cmd_secret", payload)
	record, err := store.Save(env)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(store.path(record.RecordID))
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	content := string(data)
	for _, secret := range []string{"wifi-password", "device-secret-value", "token-value", "api token=abc", "password=123"} {
		if strings.Contains(content, secret) {
			t.Fatalf("outbox record contains sensitive value %q: %s", secret, content)
		}
	}
}

func mustStore(t *testing.T, dir string, opts Options) *Store {
	t.Helper()
	opts.Dir = dir
	store, err := NewStore(opts)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store
}

func resultEnvelope(t *testing.T, commandID string, payload any) protocol.Envelope {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(payload) error = %v", err)
	}
	return protocol.Envelope{
		Version:       "1",
		Type:          protocol.MessageTypeResult,
		MessageID:     "msg_" + commandID,
		CorrelationID: commandID,
		DeviceID:      "dev_test",
		Timestamp:     time.Now().UTC(),
		Payload:       data,
	}
}
