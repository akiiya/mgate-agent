package protocol

import "time"

type CommandState string

const (
	CommandQueued    CommandState = "queued"
	CommandRunning   CommandState = "running"
	CommandSucceeded CommandState = "succeeded"
	CommandFailed    CommandState = "failed"
	CommandTimedOut  CommandState = "timed_out"
	CommandRejected  CommandState = "rejected"
	CommandCancelled CommandState = "cancelled"
)

type CommandPayload struct {
	CommandID   string         `json:"command_id"`
	Action      string         `json:"action"`
	Args        map[string]any `json:"args"`
	TimeoutSec  *int           `json:"timeout_sec,omitempty"`
	Async       bool           `json:"async"`
	RequestedBy string         `json:"requested_by"`
	IssuedAt    time.Time      `json:"issued_at"`
}

type AckPayload struct {
	CommandID string       `json:"command_id"`
	Action    string       `json:"action"`
	State     CommandState `json:"state"`
	Accepted  bool         `json:"accepted"`
	ErrorCode string       `json:"error_code"`
	Message   string       `json:"message,omitempty"`
}

type ResultPayload struct {
	CommandID       string       `json:"command_id"`
	Action          string       `json:"action"`
	State           CommandState `json:"state"`
	ExitCode        int          `json:"exit_code"`
	Stdout          string       `json:"stdout"`
	Stderr          string       `json:"stderr"`
	OutputTruncated bool         `json:"output_truncated"`
	StartedAt       time.Time    `json:"started_at"`
	EndedAt         time.Time    `json:"ended_at"`
	DurationMS      int64        `json:"duration_ms"`
	ErrorCode       string       `json:"error_code"`
}

type StatusPayload struct {
	DeviceID     string         `json:"device_id"`
	AgentVersion string         `json:"agent_version"`
	State        string         `json:"state"`
	ObservedAt   time.Time      `json:"observed_at"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}
