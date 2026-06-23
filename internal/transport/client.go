package transport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"mgate-agent/internal/protocol"
)

const (
	DefaultMaxMessageBytes  = 64 * 1024
	DefaultCommandQueueSize = 16
	DefaultOutboundSize     = 32
	DefaultMaxPullCommands  = 16
)

type CommandHandler interface {
	Handle(context.Context, protocol.CommandPayload) protocol.ResultPayload
}

type HealthState string

const (
	HealthConnected    HealthState = "connected"
	HealthDisconnected HealthState = "disconnected"
)

type HealthEvent struct {
	State HealthState
}

type WSClientOptions struct {
	BaseURL           string
	WSPath            string
	RequestTimeout    time.Duration
	HeartbeatInterval time.Duration
	MaxMessageBytes   int64
	CommandQueueSize  int
	OutboundSize      int
	MaxParallelJobs   int
	MaxOutputBytes    int
	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration

	DeviceID       string
	TenantID       string
	DeviceSecret   []byte
	AgentVersion   string
	DeviceName     string
	AllowedActions []string
	PullFallback   bool

	Handler      CommandHandler
	Logger       *slog.Logger
	HealthEvents chan<- HealthEvent
	Dispatcher   *ResultDispatcher
}

type PullClientOptions struct {
	BaseURL           string
	PullPath          string
	ResultPath        string
	RequestTimeout    time.Duration
	PullInterval      time.Duration
	MaxResponseBytes  int64
	MaxCommands       int
	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration

	DeviceID     string
	TenantID     string
	DeviceSecret []byte
	AgentVersion string
	DeviceName   string

	Handler    CommandHandler
	Logger     *slog.Logger
	Client     *http.Client
	Dispatcher *ResultDispatcher
}

type ManagerOptions struct {
	WSEnabled   bool
	PullEnabled bool
	WS          WSClientOptions
	Pull        PullClientOptions
	Logger      *slog.Logger
	Dispatcher  *ResultDispatcher
}

type Client interface {
	Run(context.Context) error
}
