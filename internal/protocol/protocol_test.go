package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeCommandJSONRoundTrip(t *testing.T) {
	issuedAt := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	timeoutSec := 60
	payload, err := json.Marshal(CommandPayload{
		CommandID:   "cmd_xxx",
		Action:      "gateway.start",
		Args:        map[string]any{"country": "US"},
		TimeoutSec:  &timeoutSec,
		Async:       true,
		RequestedBy: "user_xxx",
		IssuedAt:    issuedAt,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	env := Envelope{
		Version:       "1",
		Type:          MessageTypeCommand,
		MessageID:     "msg_xxx",
		CorrelationID: "cmd_xxx",
		DeviceID:      "dev_xxx",
		Timestamp:     issuedAt,
		Payload:       payload,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Type != MessageTypeCommand {
		t.Fatalf("unexpected message type: %s", decoded.Type)
	}

	var cmd CommandPayload
	if err := json.Unmarshal(decoded.Payload, &cmd); err != nil {
		t.Fatalf("payload Unmarshal() error = %v", err)
	}
	if cmd.Action != "gateway.start" || cmd.Args["country"] != "US" {
		t.Fatalf("unexpected command payload: %+v", cmd)
	}
}

func TestResultPayloadJSONStable(t *testing.T) {
	started := time.Date(2026, 6, 23, 0, 0, 1, 0, time.UTC)
	ended := time.Date(2026, 6, 23, 0, 0, 8, 0, time.UTC)
	result := ResultPayload{
		CommandID:       "cmd_xxx",
		Action:          "gateway.start",
		State:           CommandSucceeded,
		ExitCode:        0,
		Stdout:          "...",
		Stderr:          "",
		OutputTruncated: false,
		StartedAt:       started,
		EndedAt:         ended,
		DurationMS:      7000,
		ErrorCode:       "",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"command_id":"cmd_xxx","action":"gateway.start","state":"succeeded","exit_code":0,"stdout":"...","stderr":"","output_truncated":false,"started_at":"2026-06-23T00:00:01Z","ended_at":"2026-06-23T00:00:08Z","duration_ms":7000,"error_code":""}`
	if string(data) != want {
		t.Fatalf("unexpected JSON:\nwant %s\ngot  %s", want, string(data))
	}
}
