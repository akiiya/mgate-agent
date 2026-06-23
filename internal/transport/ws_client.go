package transport

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mgate-agent/internal/auth"
	"mgate-agent/internal/outbox"
	"mgate-agent/internal/protocol"

	"nhooyr.io/websocket"
)

type WSClient struct {
	opts   WSClientOptions
	logger *slog.Logger

	startedAt  time.Time
	activeJobs atomic.Int64

	lastMu           sync.Mutex
	lastCommandID    string
	lastCommandState string
}

type commandJob struct {
	command protocol.CommandPayload
}

func NewWSClient(opts WSClientOptions) (*WSClient, error) {
	if opts.Handler == nil {
		return nil, errors.New("websocket client requires command handler")
	}
	if opts.Dispatcher == nil {
		return nil, errors.New("websocket client requires result dispatcher")
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 15 * time.Second
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 30 * time.Second
	}
	if opts.MaxMessageBytes <= 0 {
		opts.MaxMessageBytes = DefaultMaxMessageBytes
	}
	if opts.CommandQueueSize <= 0 {
		opts.CommandQueueSize = DefaultCommandQueueSize
	}
	if opts.OutboundSize <= 0 {
		opts.OutboundSize = DefaultOutboundSize
	}
	if opts.MaxParallelJobs <= 0 {
		opts.MaxParallelJobs = 1
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &WSClient{
		opts:      opts,
		logger:    logger,
		startedAt: time.Now().UTC(),
	}, nil
}

func (c *WSClient) Run(ctx context.Context) error {
	minDelay := c.opts.ReconnectMinDelay
	if minDelay <= 0 {
		minDelay = time.Second
	}
	maxDelay := c.opts.ReconnectMaxDelay
	if maxDelay <= 0 {
		maxDelay = time.Minute
	}
	backoff := NewBackoff(minDelay, maxDelay, 0.2)
	for {
		connected, err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if connected {
			backoff.Reset()
		}
		if err != nil {
			c.logger.Warn("WebSocket 连接断开，将重连", "error", err)
		}
		delay := backoff.Next()
		c.logger.Info("WebSocket 准备重连", "delay", delay.String())
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (c *WSClient) runOnce(ctx context.Context) (bool, error) {
	wsURL, err := BuildWebSocketURL(c.opts.BaseURL, c.opts.WSPath)
	if err != nil {
		return false, err
	}
	header, err := c.HandshakeHeader(time.Now().UTC())
	if err != nil {
		return false, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return false, fmt.Errorf("dial websocket: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "client reconnect")

	// 远端消息必须限制大小，避免单条 JSON 把设备内存打爆。
	conn.SetReadLimit(c.opts.MaxMessageBytes)
	c.logger.Info("WebSocket 已连接", "url", wsURL)

	connCtx, stop := context.WithCancel(ctx)
	defer stop()
	outbound := make(chan outboundMessage, c.opts.OutboundSize)
	jobs := make(chan commandJob, c.opts.CommandQueueSize)
	errCh := make(chan error, 4)

	go c.writeLoop(connCtx, conn, outbound, errCh)

	if err := c.sendHello(connCtx, outbound); err != nil {
		return true, err
	}
	if err := c.waitHelloAck(connCtx, conn); err != nil {
		return true, err
	}
	c.logger.Info("WebSocket hello 已接受")
	c.reportHealth(HealthConnected)
	defer c.reportHealth(HealthDisconnected)

	c.opts.Dispatcher.SetWebSocketSender(func(_ context.Context, record outbox.Record) error {
		return sendOutboundWait(connCtx, outbound, record.Envelope)
	})
	defer c.opts.Dispatcher.SetWebSocketSender(nil)
	go func() { _ = c.opts.Dispatcher.DispatchOnce(connCtx) }()

	for i := 0; i < c.opts.MaxParallelJobs; i++ {
		go c.worker(connCtx, jobs)
	}
	go c.heartbeatLoop(connCtx, outbound, errCh)
	go c.readLoop(connCtx, conn, jobs, outbound, errCh)

	select {
	case <-ctx.Done():
		stop()
		_ = conn.Close(websocket.StatusNormalClosure, "context canceled")
		return true, nil
	case err := <-errCh:
		stop()
		_ = conn.Close(websocket.StatusGoingAway, "reconnect")
		return true, err
	}
}

func BuildWebSocketURL(baseURL, wsPath string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse cloud.base_url: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("cloud.base_url must use http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("cloud.base_url must include host")
	}
	pathURL, err := url.Parse(wsPath)
	if err != nil {
		return "", fmt.Errorf("parse cloud.ws_path: %w", err)
	}
	if pathURL.IsAbs() || !strings.HasPrefix(wsPath, "/") {
		return "", fmt.Errorf("cloud.ws_path must be an absolute path")
	}
	u.Path = wsPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (c *WSClient) HandshakeHeader(now time.Time) (http.Header, error) {
	nonce, err := randomNonce()
	if err != nil {
		return nil, err
	}
	timestamp := now.UTC().Format(time.RFC3339)
	signature := auth.Sign(c.opts.DeviceSecret, auth.SignInput{
		Method:    http.MethodGet,
		Path:      c.opts.WSPath,
		Timestamp: timestamp,
		Nonce:     nonce,
		Body:      nil,
	})
	header := http.Header{}
	// 认证材料放在 header，避免 secret 派生值进入 URL、代理访问日志或浏览器历史。
	header.Set("X-MGate-Device-ID", c.opts.DeviceID)
	header.Set("X-MGate-Tenant-ID", c.opts.TenantID)
	header.Set("X-MGate-Timestamp", timestamp)
	header.Set("X-MGate-Nonce", nonce)
	header.Set("X-MGate-Signature", signature)
	header.Set("X-MGate-Agent-Version", c.opts.AgentVersion)
	return header, nil
}

func randomNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(b[:]), nil
}

type outboundMessage struct {
	env  protocol.Envelope
	done chan error
}

func (c *WSClient) sendHello(ctx context.Context, outbound chan<- outboundMessage) error {
	env, err := makeEnvelope(protocol.MessageTypeHello, c.opts.DeviceID, "", protocol.HelloPayload{
		AgentVersion: c.opts.AgentVersion,
		DeviceID:     c.opts.DeviceID,
		TenantID:     c.opts.TenantID,
		DeviceName:   c.opts.DeviceName,
		Capabilities: protocol.HelloCapabilities{
			Actions:      append([]string(nil), c.opts.AllowedActions...),
			Async:        true,
			WebSocket:    true,
			PullFallback: c.opts.PullFallback,
			Outbox:       true,
		},
		Config: protocol.HelloConfigSnapshot{
			MaxParallelJobs: c.opts.MaxParallelJobs,
			MaxOutputBytes:  c.opts.MaxOutputBytes,
		},
	})
	if err != nil {
		return err
	}
	return sendOutbound(ctx, outbound, env)
}

func (c *WSClient) waitHelloAck(ctx context.Context, conn *websocket.Conn) error {
	readCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	env, err := readEnvelope(readCtx, conn)
	if err != nil {
		return fmt.Errorf("read hello_ack: %w", err)
	}
	if env.Version != envelopeVersion || env.Type != protocol.MessageTypeHelloAck {
		return fmt.Errorf("expected hello_ack, got %s", env.Type)
	}
	var ack protocol.HelloAckPayload
	if err := json.Unmarshal(env.Payload, &ack); err != nil {
		return fmt.Errorf("decode hello_ack: %w", err)
	}
	if !ack.Accepted {
		return fmt.Errorf("hello rejected: %s", ack.Message)
	}
	return nil
}

func (c *WSClient) writeLoop(ctx context.Context, conn *websocket.Conn, outbound <-chan outboundMessage, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-outbound:
			data, err := json.Marshal(msg.env)
			if err != nil {
				completeOutbound(msg, err)
				reportErr(ctx, errCh, fmt.Errorf("marshal envelope: %w", err))
				continue
			}
			// nhooyr 连接不允许多个 goroutine 并发写；hello、heartbeat、ack、result
			// 都经过这个 loop 串行发送，避免写帧交错破坏连接。
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				completeOutbound(msg, err)
				reportErr(ctx, errCh, fmt.Errorf("write websocket: %w", err))
				return
			}
			completeOutbound(msg, nil)
		}
	}
}

func (c *WSClient) readLoop(ctx context.Context, conn *websocket.Conn, jobs chan<- commandJob, outbound chan<- outboundMessage, errCh chan<- error) {
	for {
		env, err := readEnvelope(ctx, conn)
		if err != nil {
			reportErr(ctx, errCh, fmt.Errorf("read websocket: %w", err))
			return
		}
		switch env.Type {
		case protocol.MessageTypeCommand:
			if err := c.handleCommandEnvelope(ctx, env, jobs, outbound); err != nil {
				reportErr(ctx, errCh, err)
				return
			}
		default:
			c.logger.Warn("忽略未知 WebSocket 消息", "type", env.Type)
		}
	}
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (protocol.Envelope, error) {
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		return protocol.Envelope{}, err
	}
	if messageType != websocket.MessageText {
		return protocol.Envelope{}, fmt.Errorf("websocket message must be text JSON")
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return protocol.Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	return env, nil
}

func (c *WSClient) handleCommandEnvelope(ctx context.Context, env protocol.Envelope, jobs chan<- commandJob, outbound chan<- outboundMessage) error {
	if env.Version != envelopeVersion {
		return c.sendError(ctx, outbound, env.CorrelationID, "invalid_version", "unsupported envelope version")
	}
	var cmd protocol.CommandPayload
	if err := json.Unmarshal(env.Payload, &cmd); err != nil {
		return c.sendError(ctx, outbound, env.CorrelationID, "invalid_payload", "invalid command payload")
	}
	if env.DeviceID != c.opts.DeviceID {
		return c.sendRejectedResult(ctx, cmd, "device_mismatch", "command device_id does not match this device")
	}

	// WebSocket 只是 transport：这里只做 envelope 级校验和排队。
	// action 是否存在、是否允许、参数是否合法，必须统一交给 commands.Handler。
	select {
	case jobs <- commandJob{command: cmd}:
		c.logger.Info("收到 command", "command_id", cmd.CommandID, "action", cmd.Action)
	default:
		return c.sendRejectedResult(ctx, cmd, "queue_full", "command queue is full")
	}

	ack, err := makeEnvelope(protocol.MessageTypeAck, c.opts.DeviceID, cmd.CommandID, protocol.AckPayload{
		CommandID: cmd.CommandID,
		Action:    cmd.Action,
		State:     protocol.CommandQueued,
		Accepted:  true,
		ErrorCode: "",
	})
	if err != nil {
		return err
	}
	return sendOutbound(ctx, outbound, ack)
}

func (c *WSClient) worker(ctx context.Context, jobs <-chan commandJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			c.activeJobs.Add(1)
			result := c.opts.Handler.Handle(ctx, job.command)
			c.activeJobs.Add(-1)
			c.setLast(result.CommandID, string(result.State))
			c.logger.Info("command 执行完成", "command_id", result.CommandID, "action", result.Action, "state", result.State, "duration_ms", result.DurationMS)
			env, err := makeEnvelope(protocol.MessageTypeResult, c.opts.DeviceID, result.CommandID, result)
			if err != nil {
				c.logger.Warn("result envelope 构造失败", "command_id", result.CommandID, "error", err)
				continue
			}
			// final result 必须先落入 outbox；发送失败只会补发 result，不会重放 command。
			if err := c.opts.Dispatcher.Enqueue(ctx, env); err != nil {
				c.logger.Warn("result 写入 outbox 失败", "command_id", result.CommandID, "error", err)
			}
		}
	}
}

func (c *WSClient) heartbeatLoop(ctx context.Context, outbound chan<- outboundMessage, errCh chan<- error) {
	ticker := time.NewTicker(c.opts.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastID, lastState := c.last()
			env, err := makeEnvelope(protocol.MessageTypeHeartbeat, c.opts.DeviceID, "", protocol.HeartbeatPayload{
				AgentVersion:     c.opts.AgentVersion,
				DeviceID:         c.opts.DeviceID,
				UptimeSec:        int64(time.Since(c.startedAt).Seconds()),
				ActiveJobs:       c.activeJobs.Load(),
				LastCommandID:    lastID,
				LastCommandState: lastState,
				OutboxPending:    c.opts.Dispatcher.PendingCount(),
			})
			if err != nil {
				reportErr(ctx, errCh, err)
				return
			}
			if err := sendOutbound(ctx, outbound, env); err != nil {
				reportErr(ctx, errCh, err)
				return
			}
		}
	}
}

func (c *WSClient) sendRejectedResult(ctx context.Context, cmd protocol.CommandPayload, code, message string) error {
	now := time.Now().UTC()
	result := protocol.ResultPayload{
		CommandID:  cmd.CommandID,
		Action:     cmd.Action,
		State:      protocol.CommandRejected,
		ExitCode:   -1,
		Stderr:     message,
		StartedAt:  now,
		EndedAt:    now,
		DurationMS: 0,
		ErrorCode:  code,
	}
	env, err := makeEnvelope(protocol.MessageTypeResult, c.opts.DeviceID, cmd.CommandID, result)
	if err != nil {
		return err
	}
	return c.opts.Dispatcher.Enqueue(ctx, env)
}

func (c *WSClient) sendError(ctx context.Context, outbound chan<- outboundMessage, correlationID, code, message string) error {
	env, err := makeEnvelope(protocol.MessageTypeError, c.opts.DeviceID, correlationID, protocol.ErrorPayload{
		ErrorCode: code,
		Message:   message,
	})
	if err != nil {
		return err
	}
	return sendOutbound(ctx, outbound, env)
}

func sendOutbound(ctx context.Context, outbound chan<- outboundMessage, env protocol.Envelope) error {
	select {
	case outbound <- outboundMessage{env: env}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sendOutboundWait(ctx context.Context, outbound chan<- outboundMessage, env protocol.Envelope) error {
	done := make(chan error, 1)
	msg := outboundMessage{env: env, done: done}
	select {
	case outbound <- msg:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func completeOutbound(msg outboundMessage, err error) {
	if msg.done == nil {
		return
	}
	select {
	case msg.done <- err:
	default:
	}
}

func reportErr(ctx context.Context, errCh chan<- error, err error) {
	if ctx.Err() != nil {
		return
	}
	select {
	case errCh <- err:
	default:
	}
}

func (c *WSClient) setLast(commandID, state string) {
	c.lastMu.Lock()
	defer c.lastMu.Unlock()
	c.lastCommandID = commandID
	c.lastCommandState = state
}

func (c *WSClient) last() (string, string) {
	c.lastMu.Lock()
	defer c.lastMu.Unlock()
	return c.lastCommandID, c.lastCommandState
}

func (c *WSClient) reportHealth(state HealthState) {
	if c.opts.HealthEvents == nil {
		return
	}
	select {
	case c.opts.HealthEvents <- HealthEvent{State: state}:
	default:
	}
}
