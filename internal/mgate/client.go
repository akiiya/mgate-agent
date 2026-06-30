package mgate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
)

const (
	CodeMGateMissing                  = "mgate_missing"
	CodeMGateNotExecutable            = "mgate_not_executable"
	CodeCapabilitiesTimeout           = "capabilities_timeout"
	CodeCapabilitiesInvalidJSON       = "capabilities_invalid_json"
	CodeCapabilitiesUnsupportedSchema = "capabilities_unsupported_schema"
	CodeSnapshotTimeout               = "snapshot_timeout"
	CodeSnapshotInvalidJSON           = "snapshot_invalid_json"
	CodeSnapshotUnsupportedSchema     = "snapshot_unsupported_schema"
	CodeMGateExecFailed               = "mgate_exec_failed"

	SupportedSchemaVersion = 1
)

const defaultCommandTimeout = 2 * time.Second

type Options struct {
	Path                string
	CapabilitiesTimeout time.Duration
	SnapshotTimeout     time.Duration
}

type Client struct {
	opts Options
}

type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return "mgate error"
	}
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewClient(opts Options) *Client {
	if opts.CapabilitiesTimeout <= 0 {
		opts.CapabilitiesTimeout = defaultCommandTimeout
	}
	if opts.SnapshotTimeout <= 0 {
		opts.SnapshotTimeout = defaultCommandTimeout
	}
	return &Client{opts: opts}
}

func ErrorCode(err error) string {
	var mgateErr *Error
	if errors.As(err, &mgateErr) && mgateErr.Code != "" {
		return mgateErr.Code
	}
	return CodeMGateExecFailed
}

func (c *Client) Capabilities(ctx context.Context) (map[string]any, error) {
	return c.runJSON(ctx, "capabilities-json", c.opts.CapabilitiesTimeout, CodeCapabilitiesTimeout, CodeCapabilitiesInvalidJSON, CodeCapabilitiesUnsupportedSchema)
}

func (c *Client) Snapshot(ctx context.Context) (map[string]any, error) {
	return c.runJSON(ctx, "agent-snapshot", c.opts.SnapshotTimeout, CodeSnapshotTimeout, CodeSnapshotInvalidJSON, CodeSnapshotUnsupportedSchema)
}

func (c *Client) Summary(ctx context.Context) protocol.MGateStatusSummary {
	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		return protocol.MGateStatusSummary{Available: false, ErrorCode: ErrorCode(err)}
	}
	return summaryFromSnapshot(snapshot)
}

func (c *Client) runJSON(ctx context.Context, command string, timeout time.Duration, timeoutCode, invalidJSONCode, schemaCode string) (map[string]any, error) {
	if err := ensureExecutable(c.opts.Path); err != nil {
		return nil, err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// mgate 采集只允许固定 argv，不经过 shell，也不拼接用户输入。
	// 这样即使未来 cloud 能看到摘要，也不能借采集入口执行任意命令。
	cmd := exec.CommandContext(cmdCtx, c.opts.Path, command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return nil, &Error{Code: timeoutCode, Err: errors.New("mgate command timed out")}
		}
		return nil, &Error{Code: CodeMGateExecFailed, Err: fmt.Errorf("mgate command failed: %w", err)}
	}

	data := stdout.Bytes()
	var payload map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil, &Error{Code: invalidJSONCode, Err: errors.New("mgate output is not valid JSON")}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, &Error{Code: invalidJSONCode, Err: errors.New("mgate output has trailing JSON data")}
	}
	if schemaVersion(payload) != SupportedSchemaVersion {
		return nil, &Error{Code: schemaCode, Err: errors.New("unsupported mgate schema_version")}
	}
	return payload, nil
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Error{Code: CodeMGateMissing, Err: errors.New("mgate executable is missing")}
		}
		return &Error{Code: CodeMGateMissing, Err: errors.New("mgate executable is not available")}
	}
	if info.IsDir() {
		return &Error{Code: CodeMGateNotExecutable, Err: errors.New("mgate path is a directory")}
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return &Error{Code: CodeMGateNotExecutable, Err: errors.New("mgate executable bit is not set")}
	}
	return nil
}

func schemaVersion(payload map[string]any) int {
	value, ok := payload["schema_version"]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func summaryFromSnapshot(snapshot map[string]any) protocol.MGateStatusSummary {
	return protocol.MGateStatusSummary{
		Available:     true,
		SchemaVersion: schemaVersion(snapshot),
		Version:       stringField(snapshot, "version"),
		Mode:          stringField(snapshot, "mode"),
		OverallHealth: stringField(snapshot, "overall_health"),
		WiFi:          objectField(snapshot, "wifi"),
		AP:            objectField(snapshot, "ap"),
		Gateway:       objectField(snapshot, "gateway"),
		TProxy:        objectField(snapshot, "tproxy"),
		Mihomo:        objectField(snapshot, "mihomo"),
		Subscription:  objectField(snapshot, "subscription"),
		Web:           objectField(snapshot, "web"),
	}
}

func stringField(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func objectField(payload map[string]any, key string) map[string]any {
	value, ok := payload[key].(map[string]any)
	if !ok {
		return nil
	}
	// mgate 输出会进入 heartbeat/status，上报前做递归脱敏，避免 WiFi 密码或 token 泄露。
	redacted, ok := audit.Redact(value).(map[string]any)
	if !ok {
		return nil
	}
	return redacted
}
