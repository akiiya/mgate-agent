package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/audit"
	"mgate-agent/internal/commands"
	"mgate-agent/internal/config"
	"mgate-agent/internal/identity"
	"mgate-agent/internal/logx"
	"mgate-agent/internal/outbox"
	"mgate-agent/internal/transport"
)

type Options struct {
	ConfigPath string
	Stdout     io.Writer
	Stderr     io.Writer
}

func Run(ctx context.Context, opts Options) error {
	out := writerOrDefault(opts.Stdout, os.Stdout)

	cfg, cred, registry, err := loadRuntime(opts.ConfigPath)
	if err != nil {
		return err
	}
	if err := checkMGatePath(cfg.Agent.MGatePath); err != nil {
		return err
	}

	logger := logx.New(cfg.Logging.Level)
	logger.Info("agent started", "name", Name, "version", Version, "device_id", cred.DeviceID, "tenant_id", cred.TenantID)
	fmt.Fprintf(out, "%s %s started device_id=%s\n", Name, Version, cred.DeviceID)

	auditor := audit.NewWriter(cfg.Logging.AuditFile)
	outboxStore, err := outbox.NewStore(outbox.Options{
		Dir:   filepath.Join(cfg.Agent.WorkDir, "outbox"),
		Audit: auditor,
	})
	if err != nil {
		return err
	}
	dispatcher, err := transport.NewResultDispatcher(outboxStore, auditor, logger)
	if err != nil {
		return err
	}
	handler, err := commands.NewHandler(commands.Options{
		Registry:       registry,
		AllowActions:   cfg.Security.AllowActions,
		Audit:          auditor,
		MGatePath:      cfg.Agent.MGatePath,
		WorkDir:        cfg.Agent.WorkDir,
		MaxOutputBytes: cfg.Agent.MaxOutputBytes,
	})
	if err != nil {
		return err
	}

	if !cfg.Cloud.WSEnabled && !cfg.Cloud.PullEnabled {
		logger.Info("WebSocket 和 Pull 均未启用，agent 将保持运行等待退出信号")
		<-ctx.Done()
		logger.Info("agent stopped", "reason", ctx.Err())
		return nil
	}

	wsOpts := transport.WSClientOptions{
		BaseURL:           cfg.Cloud.BaseURL,
		WSPath:            cfg.Cloud.WSPath,
		RequestTimeout:    time.Duration(cfg.Cloud.RequestTimeoutSec) * time.Second,
		HeartbeatInterval: time.Duration(cfg.Agent.HeartbeatIntervalSec) * time.Second,
		MaxParallelJobs:   cfg.Agent.MaxParallelJobs,
		MaxOutputBytes:    cfg.Agent.MaxOutputBytes,
		DeviceID:          cred.DeviceID,
		TenantID:          cred.TenantID,
		DeviceSecret:      []byte(cred.DeviceSecret),
		AgentVersion:      Version,
		DeviceName:        cfg.Agent.DeviceName,
		AllowedActions:    cfg.Security.AllowActions,
		PullFallback:      cfg.Cloud.PullEnabled,
		Handler:           handler,
		Logger:            logger,
		Dispatcher:        dispatcher,
	}
	pullOpts := transport.PullClientOptions{
		BaseURL:        cfg.Cloud.BaseURL,
		PullPath:       cfg.Cloud.PullPath,
		ResultPath:     cfg.Cloud.ResultPath,
		RequestTimeout: time.Duration(cfg.Cloud.RequestTimeoutSec) * time.Second,
		PullInterval:   time.Duration(cfg.Cloud.PullIntervalSec) * time.Second,
		DeviceID:       cred.DeviceID,
		TenantID:       cred.TenantID,
		DeviceSecret:   []byte(cred.DeviceSecret),
		AgentVersion:   Version,
		DeviceName:     cfg.Agent.DeviceName,
		Handler:        handler,
		Logger:         logger,
		Dispatcher:     dispatcher,
	}
	manager, err := transport.NewManager(transport.ManagerOptions{
		WSEnabled:   cfg.Cloud.WSEnabled,
		PullEnabled: cfg.Cloud.PullEnabled,
		WS:          wsOpts,
		Pull:        pullOpts,
		Logger:      logger,
		Dispatcher:  dispatcher,
	})
	if err != nil {
		return err
	}
	if err := manager.Run(ctx); err != nil {
		return err
	}
	logger.Info("agent stopped", "reason", ctx.Err())
	return nil
}

func Check(ctx context.Context, opts Options) error {
	_ = ctx
	out := writerOrDefault(opts.Stdout, os.Stdout)

	result := runDiagnostics(opts.ConfigPath)
	writeReport(out, "MGate Agent 自检", result.report)
	if result.report.failed() {
		return ErrCheckFailed
	}
	return nil
}

func Doctor(ctx context.Context, opts Options) error {
	out := writerOrDefault(opts.Stdout, os.Stdout)
	return writeDoctor(ctx, out, opts.ConfigPath)
}

func loadRuntime(configPath string) (*config.Config, *identity.Credentials, *actions.Registry, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	cred, err := identity.Load(cfg.Security.CredentialsFile)
	if err != nil {
		return nil, nil, nil, err
	}
	registry, err := actions.NewDefaultRegistry()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := registry.ValidateAllowedActions(cfg.Security.AllowActions); err != nil {
		return nil, nil, nil, err
	}
	return cfg, cred, registry, nil
}

func checkMGatePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("mgate_path %q is not available: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("mgate_path %q is a directory", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("mgate_path %q is not executable", path)
	}
	return nil
}

func writerOrDefault(w io.Writer, fallback io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return fallback
}

var ErrUsage = errors.New("usage error")
