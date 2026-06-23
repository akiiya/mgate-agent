package protocol

import "time"

type HelloPayload struct {
	AgentVersion string              `json:"agent_version"`
	DeviceID     string              `json:"device_id"`
	TenantID     string              `json:"tenant_id"`
	DeviceName   string              `json:"device_name"`
	Capabilities HelloCapabilities   `json:"capabilities"`
	Config       HelloConfigSnapshot `json:"config"`
}

type HelloCapabilities struct {
	Actions      []string `json:"actions"`
	Async        bool     `json:"async"`
	WebSocket    bool     `json:"websocket"`
	PullFallback bool     `json:"pull_fallback"`
	Outbox       bool     `json:"outbox"`
}

type HelloConfigSnapshot struct {
	MaxParallelJobs int `json:"max_parallel_jobs"`
	MaxOutputBytes  int `json:"max_output_bytes"`
}

type HelloAckPayload struct {
	Accepted   bool      `json:"accepted"`
	ServerTime time.Time `json:"server_time"`
	Message    string    `json:"message"`
}

type HeartbeatPayload struct {
	AgentVersion     string `json:"agent_version"`
	DeviceID         string `json:"device_id"`
	UptimeSec        int64  `json:"uptime_sec"`
	ActiveJobs       int64  `json:"active_jobs"`
	LastCommandID    string `json:"last_command_id"`
	LastCommandState string `json:"last_command_state"`
	OutboxPending    int    `json:"outbox_pending"`
}

type ErrorPayload struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}
