package outbox

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
)

type Record struct {
	RecordID    string            `json:"record_id"`
	CommandID   string            `json:"command_id"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Attempts    int               `json:"attempts"`
	LastError   string            `json:"last_error"`
	NextRetryAt time.Time         `json:"next_retry_at"`
	Envelope    protocol.Envelope `json:"envelope"`
}

var filenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func NewRecord(env protocol.Envelope, now time.Time) (Record, error) {
	if env.Type != protocol.MessageTypeResult {
		return Record{}, fmt.Errorf("outbox only accepts result envelope")
	}
	env = sanitizeEnvelope(env)
	commandID, err := commandIDFromEnvelope(env)
	if err != nil {
		return Record{}, err
	}
	recordID := sanitizeRecordID(commandID)
	return Record{
		RecordID:    recordID,
		CommandID:   commandID,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
		Attempts:    0,
		LastError:   "",
		NextRetryAt: now.UTC(),
		Envelope:    env,
	}, nil
}

func sanitizeEnvelope(env protocol.Envelope) protocol.Envelope {
	var payload map[string]any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return env
	}
	// outbox 只保存最终 result，但仍对 payload 里的敏感字段名和输出文本做基础脱敏，
	// 防止未来扩展误把 psk、token、secret 等内容持久化到磁盘。
	redacted, ok := audit.Redact(payload).(map[string]any)
	if !ok {
		return env
	}
	for _, key := range []string{"stdout", "stderr"} {
		if text, ok := redacted[key].(string); ok {
			redacted[key] = audit.RedactText(text)
		}
	}
	data, err := json.Marshal(redacted)
	if err != nil {
		return env
	}
	env.Payload = data
	return env
}

func commandIDFromEnvelope(env protocol.Envelope) (string, error) {
	if env.CorrelationID != "" {
		return env.CorrelationID, nil
	}
	var result protocol.ResultPayload
	if err := json.Unmarshal(env.Payload, &result); err != nil {
		return "", err
	}
	return result.CommandID, nil
}

func sanitizeRecordID(commandID string) string {
	id := filenameUnsafe.ReplaceAllString(commandID, "_")
	id = strings.Trim(id, ".-_")
	if id == "" {
		return "unknown"
	}
	return id
}
