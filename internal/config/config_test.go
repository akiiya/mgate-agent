package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultPathUsesYAML(t *testing.T) {
	if DefaultPath != "/etc/mgate-agent/agent.yaml" {
		t.Fatalf("DefaultPath = %q", DefaultPath)
	}
}

func TestLoadYAMLSuccess(t *testing.T) {
	path := writeConfig(t, validConfig(t))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Agent.DeviceName != "ufi-test" {
		t.Fatalf("unexpected device name: %s", cfg.Agent.DeviceName)
	}
}

func TestLoadYAMLMissingRequiredField(t *testing.T) {
	cfg := validConfig(t)
	cfg.Cloud.BaseURL = ""
	path := writeConfig(t, cfg)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error")
	}
}

func TestLoadYAMLUnknownField(t *testing.T) {
	data := strings.Replace(string(mustYAML(t, validConfig(t))), "base_url:", "unknown_base_url:", 1)
	path := writeRawConfig(t, data)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error")
	}
	if !strings.Contains(err.Error(), "field unknown_base_url not found") {
		t.Fatalf("Load() error should mention unknown field, got %v", err)
	}
}

func TestLoadYAMLInvalidSyntax(t *testing.T) {
	path := writeRawConfig(t, "cloud:\n  base_url: [")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error")
	}
	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("Load() error should mention invalid YAML, got %v", err)
	}
}

func TestLoadYAMLEmptyFile(t *testing.T) {
	path := writeRawConfig(t, "")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error")
	}
	if !strings.Contains(err.Error(), "config is empty") {
		t.Fatalf("Load() error should mention empty config, got %v", err)
	}
}

func TestDefaultConfigYAMLValid(t *testing.T) {
	cfg, err := DecodeYAML(strings.NewReader(DefaultConfigYAML))
	if err != nil {
		t.Fatalf("DecodeYAML(DefaultConfigYAML) error = %v", err)
	}
	if cfg.Security.CredentialsFile != "/var/lib/mgate-agent/credentials.json" {
		t.Fatalf("credentials file should remain JSON, got %q", cfg.Security.CredentialsFile)
	}
}

func TestExampleYAMLValidAndMatchesTemplate(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "configs", "agent.example.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(example YAML) error = %v", err)
	}
	if normalizeNewlines(string(data)) != normalizeNewlines(DefaultConfigYAML) {
		t.Fatal("configs/agent.example.yaml differs from DefaultConfigYAML")
	}
	if _, err := DecodeYAML(strings.NewReader(string(data))); err != nil {
		t.Fatalf("DecodeYAML(example YAML) error = %v", err)
	}
}

func TestValidateRejectsEmptyAllowActions(t *testing.T) {
	cfg := validConfig(t)
	cfg.Security.AllowActions = nil

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error")
	}
}

func validConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		Cloud: CloudConfig{
			BaseURL:           "https://mgate.example.com",
			WSPath:            "/api/agent/v1/ws",
			PullPath:          "/api/agent/v1/pull",
			ResultPath:        "/api/agent/v1/result",
			StatusPath:        "/api/agent/v1/status",
			RequestTimeoutSec: 15,
			PullIntervalSec:   10,
			WSEnabled:         true,
			PullEnabled:       true,
		},
		Agent: AgentConfig{
			DeviceName:               "ufi-test",
			MGatePath:                filepath.Join(dir, "mgate.sh"),
			WorkDir:                  dir,
			HeartbeatIntervalSec:     30,
			StatusIntervalSec:        120,
			DefaultCommandTimeoutSec: 30,
			LongCommandTimeoutSec:    180,
			MaxParallelJobs:          1,
			MaxOutputBytes:           32768,
		},
		Security: SecurityConfig{
			CredentialsFile: filepath.Join(dir, "credentials.json"),
			AllowActions: []string{
				"status.snapshot",
				"gateway.status",
			},
			ClockSkewSec:    300,
			StrictWhitelist: true,
		},
		Logging: LoggingConfig{
			Level:     "info",
			AuditFile: filepath.Join(dir, "audit.jsonl"),
		},
	}
}

func writeConfig(t *testing.T, cfg Config) string {
	t.Helper()
	data := mustYAML(t, cfg)
	return writeRawConfig(t, string(data))
}

func writeRawConfig(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func mustYAML(t *testing.T, cfg Config) []byte {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	return data
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
