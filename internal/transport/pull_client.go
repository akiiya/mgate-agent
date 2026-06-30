package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"mgate-agent/internal/outbox"
	"mgate-agent/internal/protocol"
)

type PullClient struct {
	opts   PullClientOptions
	logger *slog.Logger

	lastCommandID    string
	lastCommandState string
}

func NewPullClient(opts PullClientOptions) (*PullClient, error) {
	if opts.Handler == nil {
		return nil, errors.New("pull client requires command handler")
	}
	if opts.Dispatcher == nil {
		return nil, errors.New("pull client requires result dispatcher")
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 15 * time.Second
	}
	if opts.PullInterval <= 0 {
		opts.PullInterval = 10 * time.Second
	}
	if opts.MaxResponseBytes <= 0 {
		opts.MaxResponseBytes = DefaultMaxMessageBytes
	}
	if opts.MaxCommands <= 0 {
		opts.MaxCommands = DefaultMaxPullCommands
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: opts.RequestTimeout}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	client := &PullClient{opts: opts, logger: logger}
	opts.Dispatcher.SetPullSender(client.SendRecord)
	return client, nil
}

func (c *PullClient) Run(ctx context.Context, shouldPoll func() bool) error {
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
		if ctx.Err() != nil {
			return nil
		}
		if shouldPoll != nil && !shouldPoll() {
			// WebSocket 健康时暂停 Pull，避免主通道正常时仍高频轮询。
			if !sleepContext(ctx, 200*time.Millisecond) {
				return nil
			}
			continue
		}

		if err := c.PollOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			delay := backoff.Next()
			c.logger.Warn("Pull 请求失败，将退避重试", "error", err, "delay", delay.String())
			if !sleepContext(ctx, delay) {
				return nil
			}
			continue
		}
		backoff.Reset()
		if !sleepContext(ctx, c.opts.PullInterval) {
			return nil
		}
	}
}

func (c *PullClient) PollOnce(ctx context.Context) error {
	pullURL, err := BuildHTTPURL(c.opts.BaseURL, c.opts.PullPath)
	if err != nil {
		return err
	}
	body, err := json.Marshal(PullRequestPayload{
		AgentVersion:     c.opts.AgentVersion,
		DeviceID:         c.opts.DeviceID,
		TenantID:         c.opts.TenantID,
		DeviceName:       c.opts.DeviceName,
		LastCommandID:    c.lastCommandID,
		LastCommandState: c.lastCommandState,
		ActiveJobs:       0,
		Transport:        "pull",
		MGate:            c.mgateSummary(ctx),
	})
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	req, err := newSignedJSONRequest(reqCtx, http.MethodPost, pullURL, c.opts.PullPath, body, c.opts)
	if err != nil {
		return err
	}
	resp, err := c.opts.Client.Do(req)
	if err != nil {
		return fmt.Errorf("pull request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pull returned status %d", resp.StatusCode)
	}
	// 限制 Pull response 大小，避免异常响应放大内存占用。
	data, err := readLimited(resp.Body, c.opts.MaxResponseBytes)
	if err != nil {
		return err
	}
	var payload PullResponsePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode pull response: %w", err)
	}
	if len(payload.Commands) > c.opts.MaxCommands {
		return fmt.Errorf("pull response has too many commands: %d", len(payload.Commands))
	}
	for _, env := range payload.Commands {
		result := c.handleEnvelope(ctx, env)
		if result.CommandID == "" {
			continue
		}
		resultEnv, err := makeEnvelope(protocol.MessageTypeResult, c.opts.DeviceID, result.CommandID, result)
		if err != nil {
			return err
		}
		// Pull 只是兜底 transport，final result 仍先进入 outbox。
		if err := c.opts.Dispatcher.Enqueue(ctx, resultEnv); err != nil {
			return err
		}
		c.lastCommandID = result.CommandID
		c.lastCommandState = string(result.State)
	}
	return nil
}

func (c *PullClient) mgateSummary(ctx context.Context) *protocol.MGateStatusSummary {
	if c.opts.MGateStatus == nil {
		return nil
	}
	summary := c.opts.MGateStatus.Summary(ctx)
	return &summary
}
func (c *PullClient) handleEnvelope(ctx context.Context, env protocol.Envelope) protocol.ResultPayload {
	var cmd protocol.CommandPayload
	if err := json.Unmarshal(env.Payload, &cmd); err != nil {
		return rejectedResult("", "", "invalid_payload", "invalid command payload")
	}
	if env.Version != envelopeVersion {
		return rejectedResult(cmd.CommandID, cmd.Action, "invalid_version", "unsupported envelope version")
	}
	if env.Type != protocol.MessageTypeCommand {
		return rejectedResult(cmd.CommandID, cmd.Action, "invalid_type", "pull response item is not command")
	}
	if env.DeviceID != c.opts.DeviceID {
		return rejectedResult(cmd.CommandID, cmd.Action, "device_mismatch", "command device_id does not match this device")
	}
	// Pull 不能直接调用 runner 或 action registry，所有命令必须进入统一 Handler。
	return c.opts.Handler.Handle(ctx, cmd)
}

func (c *PullClient) SendRecord(ctx context.Context, record outbox.Record) error {
	return c.postEnvelope(ctx, record.Envelope)
}

func (c *PullClient) postEnvelope(ctx context.Context, env protocol.Envelope) error {
	resultURL, err := BuildHTTPURL(c.opts.BaseURL, c.opts.ResultPath)
	if err != nil {
		return err
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	req, err := newSignedJSONRequest(reqCtx, http.MethodPost, resultURL, c.opts.ResultPath, body, c.opts)
	if err != nil {
		return err
	}
	resp, err := c.opts.Client.Do(req)
	if err != nil {
		return fmt.Errorf("result request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("result returned status %d", resp.StatusCode)
	}
	return nil
}

func rejectedResult(commandID, action, code, message string) protocol.ResultPayload {
	now := time.Now().UTC()
	return protocol.ResultPayload{
		CommandID:  commandID,
		Action:     action,
		State:      protocol.CommandRejected,
		ExitCode:   -1,
		Stderr:     message,
		StartedAt:  now,
		EndedAt:    now,
		DurationMS: 0,
		ErrorCode:  code,
	}
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("http response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
