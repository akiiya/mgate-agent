package commands

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
	"mgate-agent/internal/runner"
)

var commandIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{6,128}$`)

type RunnerFunc func(context.Context, runner.Request) protocol.ResultPayload

type Options struct {
	Registry       *actions.Registry
	AllowActions   []string
	Audit          *audit.Writer
	Dedupe         *DedupeStore
	Runner         RunnerFunc
	MGatePath      string
	WorkDir        string
	MaxOutputBytes int
}

type Handler struct {
	registry       *actions.Registry
	allowActions   map[string]struct{}
	audit          *audit.Writer
	dedupe         *DedupeStore
	runner         RunnerFunc
	mgatePath      string
	workDir        string
	maxOutputBytes int
}

func NewHandler(opts Options) (*Handler, error) {
	if opts.Registry == nil {
		return nil, errors.New("commands handler requires action registry")
	}
	if len(opts.AllowActions) == 0 {
		return nil, errors.New("commands handler requires allow_actions")
	}
	if err := opts.Registry.ValidateAllowedActions(opts.AllowActions); err != nil {
		return nil, err
	}
	runnerFunc := opts.Runner
	if runnerFunc == nil {
		runnerFunc = runner.Run
	}
	dedupe := opts.Dedupe
	if dedupe == nil {
		dedupe = NewDedupeStore(DefaultDedupeTTL, DefaultDedupeMaxEntries)
	}

	allowed := make(map[string]struct{}, len(opts.AllowActions))
	for _, action := range opts.AllowActions {
		allowed[action] = struct{}{}
	}
	return &Handler{
		registry:       opts.Registry,
		allowActions:   allowed,
		audit:          opts.Audit,
		dedupe:         dedupe,
		runner:         runnerFunc,
		mgatePath:      opts.MGatePath,
		workDir:        opts.WorkDir,
		maxOutputBytes: opts.MaxOutputBytes,
	}, nil
}

func (h *Handler) Handle(ctx context.Context, cmd protocol.CommandPayload) protocol.ResultPayload {
	// command handler 是 Phase 2 transport 的唯一入口。WebSocket/Pull 只负责收发，
	// 不能在 transport 层绕过这里的白名单、去重、参数校验和 audit 生命周期。
	h.writeAudit(audit.Event{
		Event:     audit.EventCommandReceived,
		CommandID: cmd.CommandID,
		Action:    cmd.Action,
		Args:      cmd.Args,
	})

	if err := validateCommandID(cmd.CommandID); err != nil {
		return h.reject(cmd, "invalid_command_id", err)
	}
	if !h.dedupe.Reserve(cmd.CommandID) {
		return h.reject(cmd, "duplicate_command", errors.New("duplicate command_id"))
	}

	spec, ok := h.registry.Get(cmd.Action)
	if !ok {
		return h.reject(cmd, "unknown_action", fmt.Errorf("unknown action %q", cmd.Action))
	}
	if _, ok := h.allowActions[cmd.Action]; !ok {
		// 双层白名单：代码内 registry 定义“系统知道什么”，本地配置 allow_actions
		// 再定义“这台设备允许什么”。这样云端和通道都不能扩大本机授权面。
		return h.reject(cmd, "action_not_allowed", fmt.Errorf("action %q is not allowed by local config", cmd.Action))
	}

	validatedArgs, err := actions.ValidateArgs(spec, cmd.Args)
	if err != nil {
		return h.reject(cmd, "invalid_args", err)
	}
	timeout, err := resolveTimeout(spec, cmd.TimeoutSec)
	if err != nil {
		return h.reject(cmd, "timeout_exceeded", err)
	}

	h.writeAudit(audit.Event{
		Event:     audit.EventCommandStarted,
		CommandID: cmd.CommandID,
		Action:    cmd.Action,
		Args:      spec.RedactedArgs(validatedArgs),
		State:     string(protocol.CommandRunning),
	})

	result := h.runner(ctx, runner.Request{
		CommandID:      cmd.CommandID,
		Spec:           spec,
		Args:           validatedArgs,
		MGatePath:      h.mgatePath,
		WorkDir:        h.workDir,
		Timeout:        timeout,
		MaxOutputBytes: h.maxOutputBytes,
	})

	h.writeAudit(audit.Event{
		Event:      audit.EventCommandFinished,
		CommandID:  result.CommandID,
		Action:     result.Action,
		State:      string(result.State),
		ExitCode:   result.ExitCode,
		DurationMS: result.DurationMS,
		ErrorCode:  result.ErrorCode,
	})
	return result
}

func validateCommandID(id string) error {
	if !commandIDPattern.MatchString(id) {
		return errors.New("command_id must be 6..128 chars and contain only letters, numbers, dot, underscore, colon or dash")
	}
	return nil
}

func resolveTimeout(spec actions.Spec, timeoutSec *int) (time.Duration, error) {
	if timeoutSec == nil {
		return spec.Timeout, nil
	}
	if *timeoutSec <= 0 {
		return 0, errors.New("timeout_sec must be positive when specified")
	}
	requested := time.Duration(*timeoutSec) * time.Second
	if requested > spec.Timeout {
		return 0, fmt.Errorf("timeout_sec must not exceed action timeout %d", int(spec.Timeout.Seconds()))
	}
	return requested, nil
}

func (h *Handler) reject(cmd protocol.CommandPayload, code string, err error) protocol.ResultPayload {
	now := time.Now().UTC()
	h.writeAudit(audit.Event{
		Event:     audit.EventCommandRejected,
		CommandID: cmd.CommandID,
		Action:    cmd.Action,
		Args:      cmd.Args,
		State:     string(protocol.CommandRejected),
		ExitCode:  -1,
		ErrorCode: code,
	})
	return protocol.ResultPayload{
		CommandID:  cmd.CommandID,
		Action:     cmd.Action,
		State:      protocol.CommandRejected,
		ExitCode:   -1,
		StartedAt:  now,
		EndedAt:    now,
		DurationMS: 0,
		ErrorCode:  code,
		Stderr:     err.Error(),
	}
}

func (h *Handler) writeAudit(event audit.Event) {
	if h.audit == nil {
		return
	}
	// audit 失败不能影响命令处理结果。设备侧磁盘或权限问题应由运维观察日志处理，
	// 不能让审计写入失败变成新的远程拒绝或 panic 路径。
	_ = h.audit.Write(event)
}
