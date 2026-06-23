package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	cfg, err := DecodeYAML(f)
	if err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}
	return cfg, nil
}

func DecodeYAML(r io.Reader) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(r)
	// 主配置给人编辑，因此 unknown field 必须尽早拒绝；否则拼写错误会静默变成默认零值。
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config is empty")
		}
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if isZeroConfig(cfg) {
		return nil, errors.New("config is empty")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if err := validateCloud(c.Cloud); err != nil {
		return err
	}
	if err := validateAgent(c.Agent); err != nil {
		return err
	}
	if err := validateSecurity(c.Security); err != nil {
		return err
	}
	if err := validateLogging(c.Logging); err != nil {
		return err
	}
	return nil
}

func validateCloud(c CloudConfig) error {
	if err := validateURL("cloud.base_url", c.BaseURL); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"cloud.ws_path":     c.WSPath,
		"cloud.pull_path":   c.PullPath,
		"cloud.result_path": c.ResultPath,
		"cloud.status_path": c.StatusPath,
	} {
		if !strings.HasPrefix(value, "/") {
			return fmt.Errorf("%s must start with /", name)
		}
	}
	if err := validateRange("cloud.request_timeout_sec", c.RequestTimeoutSec, 1, 300); err != nil {
		return err
	}
	return validateRange("cloud.pull_interval_sec", c.PullIntervalSec, 1, 3600)
}

func validateAgent(c AgentConfig) error {
	if strings.TrimSpace(c.DeviceName) == "" {
		return errors.New("agent.device_name is required")
	}
	if !isAbsolutePath(c.MGatePath) {
		return errors.New("agent.mgate_path must be an absolute path")
	}
	if !isAbsolutePath(c.WorkDir) {
		return errors.New("agent.work_dir must be an absolute path")
	}
	if err := validateRange("agent.heartbeat_interval_sec", c.HeartbeatIntervalSec, 5, 3600); err != nil {
		return err
	}
	if err := validateRange("agent.status_interval_sec", c.StatusIntervalSec, 10, 86400); err != nil {
		return err
	}
	if err := validateRange("agent.default_command_timeout_sec", c.DefaultCommandTimeoutSec, 1, 600); err != nil {
		return err
	}
	if err := validateRange("agent.long_command_timeout_sec", c.LongCommandTimeoutSec, 1, 3600); err != nil {
		return err
	}
	if c.LongCommandTimeoutSec < c.DefaultCommandTimeoutSec {
		return errors.New("agent.long_command_timeout_sec must be >= agent.default_command_timeout_sec")
	}
	if err := validateRange("agent.max_parallel_jobs", c.MaxParallelJobs, 1, 16); err != nil {
		return err
	}
	return validateRange("agent.max_output_bytes", c.MaxOutputBytes, 1024, 1024*1024)
}

func validateSecurity(c SecurityConfig) error {
	if !isAbsolutePath(c.CredentialsFile) {
		return errors.New("security.credentials_file must be an absolute path")
	}
	if len(c.AllowActions) == 0 {
		return errors.New("security.allow_actions must not be empty")
	}
	seen := make(map[string]struct{}, len(c.AllowActions))
	for _, action := range c.AllowActions {
		if strings.TrimSpace(action) == "" {
			return errors.New("security.allow_actions contains empty action")
		}
		if _, ok := seen[action]; ok {
			return fmt.Errorf("security.allow_actions contains duplicate action %q", action)
		}
		seen[action] = struct{}{}
	}
	return validateRange("security.clock_skew_sec", c.ClockSkewSec, 0, 3600)
}

func validateLogging(c LoggingConfig) error {
	switch c.Level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("logging.level must be one of debug, info, warn, error")
	}
	if !isAbsolutePath(c.AuditFile) {
		return errors.New("logging.audit_file must be an absolute path")
	}
	return nil
}

func validateURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include host", name)
	}
	return nil
}

func validateRange(name string, value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return nil
}

func isAbsolutePath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	// 目标设备是 Debian，但开发和测试可能在 Windows 上运行；这里同时接受 POSIX
	// 绝对路径和宿主平台绝对路径，避免把平台差异误判为配置错误。
	return strings.HasPrefix(path, "/") || filepath.IsAbs(path)
}

func isZeroConfig(cfg Config) bool {
	return cfg.Cloud == (CloudConfig{}) &&
		cfg.Agent == (AgentConfig{}) &&
		cfg.Security.CredentialsFile == "" &&
		len(cfg.Security.AllowActions) == 0 &&
		cfg.Security.ClockSkewSec == 0 &&
		!cfg.Security.StrictWhitelist &&
		cfg.Logging == (LoggingConfig{})
}
