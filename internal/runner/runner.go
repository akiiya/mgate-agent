package runner

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/audit"
	"mgate-agent/internal/protocol"
)

type Request struct {
	CommandID      string
	Spec           actions.Spec
	Args           actions.ValidatedArgs
	MGatePath      string
	WorkDir        string
	Timeout        time.Duration
	MaxOutputBytes int
}

func Run(ctx context.Context, req Request) protocol.ResultPayload {
	started := time.Now().UTC()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = req.Spec.Timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	stdout, stderr, limiter := newOutputStreams(req.MaxOutputBytes)
	result := protocol.ResultPayload{
		CommandID: req.CommandID,
		Action:    req.Spec.Name,
		State:     protocol.CommandRunning,
		ExitCode:  -1,
		StartedAt: started,
	}

	argv, err := req.Spec.Argv(req.Args)
	if err != nil {
		return finish(result, protocol.CommandRejected, -1, "", "", false, "invalid_action_spec", started)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 这里是 agent 最关键的安全边界：mgatePath 来自本地配置，argv 来自硬编码
	// action spec 和已校验参数。不能通过 shell 中转，也不能把远端输入拼成命令字符串。
	cmd := exec.CommandContext(runCtx, req.MGatePath, argv...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Run()
	out := stdout.String()
	errOut := stderr.String()
	truncated := limiter.Truncated()

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return finish(result, protocol.CommandTimedOut, -1, out, errOut, truncated, "timeout", started)
	}
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return finish(result, protocol.CommandFailed, exitCode, out, errOut, truncated, "process_failed", started)
	}
	return finish(result, protocol.CommandSucceeded, 0, out, errOut, truncated, "", started)
}

func finish(result protocol.ResultPayload, state protocol.CommandState, exitCode int, stdout, stderr string, truncated bool, errorCode string, started time.Time) protocol.ResultPayload {
	ended := time.Now().UTC()
	result.State = state
	result.ExitCode = exitCode
	// result 需要保留必要输出给 cloud 排障，但不能把明显的密钥行原样写入 result/outbox。
	result.Stdout = audit.RedactText(stdout)
	result.Stderr = audit.RedactText(stderr)
	result.OutputTruncated = truncated
	result.EndedAt = ended
	result.DurationMS = ended.Sub(started).Milliseconds()
	result.ErrorCode = errorCode
	return result
}
