package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	EventCommandReceived = "command_received"
	EventCommandRejected = "command_rejected"
	EventCommandStarted  = "command_started"
	EventCommandFinished = "command_finished"

	EventOutboxRecordSaved         = "outbox_record_saved"
	EventOutboxRecordSendStarted   = "outbox_record_send_started"
	EventOutboxRecordSendSucceeded = "outbox_record_send_succeeded"
	EventOutboxRecordSendFailed    = "outbox_record_send_failed"
	EventOutboxRecordDeleted       = "outbox_record_deleted"
	EventOutboxRecordDropped       = "outbox_record_dropped"
	EventOutboxRecordCorrupted     = "outbox_record_corrupted"
)

type Event struct {
	Time       time.Time      `json:"time"`
	Event      string         `json:"event"`
	RecordID   string         `json:"record_id,omitempty"`
	CommandID  string         `json:"command_id,omitempty"`
	Action     string         `json:"action,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
	State      string         `json:"state,omitempty"`
	ExitCode   int            `json:"exit_code,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	ErrorCode  string         `json:"error_code,omitempty"`
	Attempts   int            `json:"attempts,omitempty"`
	Transport  string         `json:"transport,omitempty"`
}

type Writer struct {
	path string
	mu   sync.Mutex
}

func NewWriter(path string) *Writer {
	return &Writer{path: path}
}

func (w *Writer) Write(event Event) error {
	if w == nil || w.path == "" {
		return nil
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.Args != nil {
		// audit 是长期留存材料，必须在写文件前做深拷贝脱敏，避免调用方误传 psk、
		// token 等字段后把敏感信息固化到磁盘。
		if redacted, ok := Redact(event.Args).(map[string]any); ok {
			event.Args = redacted
		}
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
