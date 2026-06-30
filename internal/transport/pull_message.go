package transport

import (
	"time"

	"mgate-agent/internal/protocol"
)

type PullRequestPayload struct {
	AgentVersion     string                       `json:"agent_version"`
	DeviceID         string                       `json:"device_id"`
	TenantID         string                       `json:"tenant_id"`
	DeviceName       string                       `json:"device_name"`
	LastCommandID    string                       `json:"last_command_id"`
	LastCommandState string                       `json:"last_command_state"`
	ActiveJobs       int64                        `json:"active_jobs"`
	Transport        string                       `json:"transport"`
	MGate            *protocol.MGateStatusSummary `json:"mgate,omitempty"`
}

type PullResponsePayload struct {
	ServerTime time.Time           `json:"server_time"`
	Commands   []protocol.Envelope `json:"commands"`
}
