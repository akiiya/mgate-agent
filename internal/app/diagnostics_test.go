package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgate-agent/internal/config"
	"mgate-agent/internal/identity"

	"gopkg.in/yaml.v3"
)

func TestCheckSuccess(t *testing.T) {
	cfgPath, _ := writeRuntimeFiles(t, runtimeFixtureOptions{})
	var out bytes.Buffer

	err := Check(t.Context(), Options{ConfigPath: cfgPath, Stdout: &out})
	if err != nil {
		t.Fatalf("Check() error = %v\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"MGate Agent 自检", "[OK] YAML 配置解析成功", "[OK] mgate.sh 可执行", "结果：通过"} {
		if !strings.Contains(text, want) {
			t.Fatalf("check output missing %q:\n%s", want, text)
		}
	}
}

func TestCheckMissingConfigFails(t *testing.T) {
	var out bytes.Buffer
	err := Check(t.Context(), Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), Stdout: &out})
	if !errors.Is(err, ErrCheckFailed) {
		t.Fatalf("Check() error = %v, want ErrCheckFailed", err)
	}
	if !strings.Contains(out.String(), "[FAIL] 配置文件不可读取") {
		t.Fatalf("missing config output unexpected:\n%s", out.String())
	}
}

func TestCheckMissingMGatePathFails(t *testing.T) {
	cfgPath, cfg := writeRuntimeFiles(t, runtimeFixtureOptions{})
	cfg.Agent.MGatePath = filepath.Join(t.TempDir(), "missing-mgate")
	writeConfigFile(t, cfgPath, cfg)

	var out bytes.Buffer
	err := Check(t.Context(), Options{ConfigPath: cfgPath, Stdout: &out})
	if !errors.Is(err, ErrCheckFailed) {
		t.Fatalf("Check() error = %v, want ErrCheckFailed", err)
	}
	if !strings.Contains(out.String(), "[FAIL] mgate.sh 不存在") {
		t.Fatalf("missing mgate output unexpected:\n%s", out.String())
	}
}

func TestCredentialPermissionStatusLinux(t *testing.T) {
	if _, ok := credentialPermissionStatus(0o644, "linux"); ok {
		t.Fatal("0644 should be rejected on linux")
	}
	if _, ok := credentialPermissionStatus(0o600, "linux"); !ok {
		t.Fatal("0600 should be accepted on linux")
	}
}

func TestDoctorOutputsRedactedSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	cfgPath, _ := writeRuntimeFiles(t, runtimeFixtureOptions{BaseURL: server.URL})
	var out bytes.Buffer

	err := Doctor(t.Context(), Options{ConfigPath: cfgPath, Stdout: &out})
	if err != nil {
		t.Fatalf("Doctor() error = %v\n%s", err, out.String())
	}
	text := out.String()
	if strings.Contains(text, "super-secret-value") {
		t.Fatalf("doctor leaked device_secret:\n%s", text)
	}
	for _, want := range []string{"配置摘要", "credentials.device_secret: ***REDACTED***", "outbox_pending:", "允许 action"} {
		if !strings.Contains(text, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, text)
		}
	}
}

type runtimeFixtureOptions struct {
	BaseURL string
}

func writeRuntimeFiles(t *testing.T, opts runtimeFixtureOptions) (string, config.Config) {
	t.Helper()
	root := t.TempDir()
	mgatePath := filepath.Join(root, "mgate.sh")
	if err := os.WriteFile(mgatePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(mgate) error = %v", err)
	}
	if err := os.Chmod(mgatePath, 0o755); err != nil {
		t.Fatalf("Chmod(mgate) error = %v", err)
	}

	credPath := filepath.Join(root, "credentials.json")
	cred := identity.Credentials{
		DeviceID:      "dev_test",
		TenantID:      "tenant_test",
		DeviceSecret:  "super-secret-value",
		SecretVersion: 1,
		EnrolledAt:    time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		CloudBaseURL:  "https://mgate.example.com",
	}
	credData, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(credentials) error = %v", err)
	}
	if err := os.WriteFile(credPath, credData, 0o600); err != nil {
		t.Fatalf("WriteFile(credentials) error = %v", err)
	}
	if err := os.Chmod(credPath, 0o600); err != nil {
		t.Fatalf("Chmod(credentials) error = %v", err)
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://mgate.example.com"
	}
	cfg := config.Config{
		Cloud: config.CloudConfig{
			BaseURL:           baseURL,
			WSPath:            "/api/agent/v1/ws",
			PullPath:          "/api/agent/v1/pull",
			ResultPath:        "/api/agent/v1/result",
			StatusPath:        "/api/agent/v1/status",
			RequestTimeoutSec: 15,
			PullIntervalSec:   10,
			WSEnabled:         true,
			PullEnabled:       true,
		},
		Agent: config.AgentConfig{
			DeviceName:               "ufi-test",
			MGatePath:                mgatePath,
			WorkDir:                  filepath.Join(root, "work"),
			HeartbeatIntervalSec:     30,
			StatusIntervalSec:        120,
			DefaultCommandTimeoutSec: 30,
			LongCommandTimeoutSec:    180,
			MaxParallelJobs:          1,
			MaxOutputBytes:           32768,
		},
		Security: config.SecurityConfig{
			CredentialsFile: credPath,
			AllowActions:    []string{"status.snapshot", "gateway.status"},
			ClockSkewSec:    300,
			StrictWhitelist: true,
		},
		Logging: config.LoggingConfig{
			Level:     "info",
			AuditFile: filepath.Join(root, "logs", "audit.jsonl"),
		},
	}
	cfgPath := filepath.Join(root, "agent.yaml")
	writeConfigFile(t, cfgPath, cfg)
	return cfgPath, cfg
}

func writeConfigFile(t *testing.T, path string, cfg config.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
}
