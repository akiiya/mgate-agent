package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/config"
	"mgate-agent/internal/identity"
	"mgate-agent/internal/outbox"
	"mgate-agent/internal/transport"
)

var ErrCheckFailed = errors.New("diagnostics failed")

type checkStatus string

const (
	statusOK   checkStatus = "OK"
	statusWarn checkStatus = "WARN"
	statusFail checkStatus = "FAIL"
)

type checkItem struct {
	Status  checkStatus
	Message string
}

type checkReport struct {
	items []checkItem
}

func (r *checkReport) ok(format string, args ...any) {
	r.items = append(r.items, checkItem{Status: statusOK, Message: fmt.Sprintf(format, args...)})
}

func (r *checkReport) warn(format string, args ...any) {
	r.items = append(r.items, checkItem{Status: statusWarn, Message: fmt.Sprintf(format, args...)})
}

func (r *checkReport) fail(format string, args ...any) {
	r.items = append(r.items, checkItem{Status: statusFail, Message: fmt.Sprintf(format, args...)})
}

func (r *checkReport) failed() bool {
	for _, item := range r.items {
		if item.Status == statusFail {
			return true
		}
	}
	return false
}

type diagnosticsResult struct {
	report   checkReport
	cfg      *config.Config
	cred     *identity.Credentials
	registry *actions.Registry
}

func runDiagnostics(configPath string) diagnosticsResult {
	var result diagnosticsResult
	report := &result.report
	if strings.TrimSpace(configPath) == "" {
		configPath = config.DefaultPath
	}

	if info, err := os.Stat(configPath); err != nil {
		report.fail("配置文件不可读取: %s (%v)", configPath, err)
		return result
	} else if info.IsDir() {
		report.fail("配置文件不是普通文件: %s", configPath)
		return result
	} else {
		report.ok("配置文件可读取: %s", configPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		report.fail("YAML 配置解析或校验失败: %v", err)
		return result
	}
	result.cfg = cfg
	report.ok("YAML 配置解析成功")
	report.ok("配置值合法")

	if cfg.Cloud.WSEnabled {
		if _, err := transport.BuildWebSocketURL(cfg.Cloud.BaseURL, cfg.Cloud.WSPath); err != nil {
			report.fail("WebSocket URL 非法: %v", err)
		} else {
			report.ok("WebSocket URL 合法: %s + %s", cfg.Cloud.BaseURL, cfg.Cloud.WSPath)
		}
	} else {
		report.warn("WebSocket 已禁用")
	}
	if _, err := transport.BuildHTTPURL(cfg.Cloud.BaseURL, cfg.Cloud.PullPath); err != nil {
		report.fail("Pull URL 非法: %v", err)
	} else {
		report.ok("Pull URL 合法: %s + %s", cfg.Cloud.BaseURL, cfg.Cloud.PullPath)
	}
	if _, err := transport.BuildHTTPURL(cfg.Cloud.BaseURL, cfg.Cloud.ResultPath); err != nil {
		report.fail("Result URL 非法: %v", err)
	} else {
		report.ok("Result URL 合法: %s + %s", cfg.Cloud.BaseURL, cfg.Cloud.ResultPath)
	}
	if !cfg.Cloud.WSEnabled && !cfg.Cloud.PullEnabled {
		report.fail("WebSocket 和 HTTPS Pull 不能同时禁用")
	} else if !cfg.Cloud.PullEnabled {
		report.warn("HTTPS Pull 已禁用，将仅使用 WebSocket")
	} else if !cfg.Cloud.WSEnabled {
		report.warn("WebSocket 已禁用，将仅使用 HTTPS Pull")
	} else {
		report.ok("transport 配置可用: WebSocket + HTTPS Pull")
	}

	registry, err := actions.NewDefaultRegistry()
	if err != nil {
		report.fail("action registry 初始化失败: %v", err)
	} else if err := registry.ValidateAllowedActions(cfg.Security.AllowActions); err != nil {
		report.fail("allow_actions 校验失败: %v", err)
	} else {
		result.registry = registry
		report.ok("allow_actions 全部存在于本地 action registry")
	}

	checkCredentials(report, cfg.Security.CredentialsFile, &result)
	checkMGate(report, cfg.Agent.MGatePath)
	checkWritableDir(report, "work_dir 目录可写", cfg.Agent.WorkDir)
	checkWritableDir(report, "audit 日志目录可写", filepath.Dir(cfg.Logging.AuditFile))
	checkWritableDir(report, "outbox 目录可写", filepath.Join(cfg.Agent.WorkDir, "outbox"))

	return result
}

func checkCredentials(report *checkReport, path string, result *diagnosticsResult) {
	info, err := os.Stat(path)
	if err != nil {
		report.fail("credentials 文件不可读取: %s (%v)", path, err)
		return
	}
	if info.IsDir() {
		report.fail("credentials 不是普通文件: %s", path)
		return
	}
	report.ok("credentials 文件可读取: %s", path)

	if msg, ok := credentialPermissionStatus(info.Mode().Perm(), runtime.GOOS); !ok {
		report.fail("credentials 文件权限不安全: %s", msg)
	} else {
		report.ok("credentials 文件权限安全: %s", msg)
	}

	cred, err := identity.Load(path)
	if err != nil {
		report.fail("credentials JSON 解析或校验失败: %v", err)
		return
	}
	result.cred = cred
	report.ok("credentials JSON 解析成功: device_id=%s tenant_id=%s", cred.DeviceID, cred.TenantID)
}

func credentialPermissionStatus(mode fs.FileMode, goos string) (string, bool) {
	if goos != "linux" {
		return fmt.Sprintf("%04o (当前平台不强制 0600)", mode), true
	}
	if mode != 0o600 {
		return fmt.Sprintf("期望 0600，实际 %04o", mode), false
	}
	return "0600", true
}

func checkMGate(report *checkReport, path string) {
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
		report.fail("mgate_path 不是绝对路径: %s", path)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		report.fail("mgate.sh 不存在: %s (%v)", path, err)
		return
	}
	if info.IsDir() {
		report.fail("mgate_path 指向目录: %s", path)
		return
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		report.fail("mgate.sh 不可执行: %s", path)
		return
	}
	report.ok("mgate.sh 可执行: %s", path)
}

func checkWritableDir(report *checkReport, label, dir string) {
	if err := ensureWritableDir(dir); err != nil {
		report.fail("%s: %s (%v)", label, dir, err)
		return
	}
	report.ok("%s: %s", label, dir)
}

func ensureWritableDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("empty path")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".mgate-agent-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}

func writeReport(out interface {
	Write([]byte) (int, error)
}, title string, report checkReport) {
	fmt.Fprintf(out, "%s\n\n", title)
	for _, item := range report.items {
		fmt.Fprintf(out, "[%s] %s\n", item.Status, item.Message)
	}
	if report.failed() {
		fmt.Fprintln(out, "\n结果：失败")
		return
	}
	fmt.Fprintln(out, "\n结果：通过")
}

func writeDoctor(ctx context.Context, out interface {
	Write([]byte) (int, error)
}, configPath string) error {
	result := runDiagnostics(configPath)
	writeReport(out, "MGate Agent Doctor", result.report)

	fmt.Fprintf(out, "\n版本：%s %s\n", Name, Version)
	if result.cfg != nil {
		// doctor 只输出安全摘要，方便用户复制给开发者排查，同时避免泄露 device_secret。
		writeSafeConfigSummary(out, result)
		doNetworkProbe(ctx, out, result.cfg.Cloud.BaseURL)
	}
	if result.report.failed() {
		return ErrCheckFailed
	}
	return nil
}

func writeSafeConfigSummary(out interface {
	Write([]byte) (int, error)
}, result diagnosticsResult) {
	cfg := result.cfg
	fmt.Fprintln(out, "\n配置摘要：")
	fmt.Fprintf(out, "- device_name: %s\n", cfg.Agent.DeviceName)
	if result.cred != nil {
		fmt.Fprintf(out, "- device_id: %s\n", result.cred.DeviceID)
		fmt.Fprintf(out, "- tenant_id: %s\n", result.cred.TenantID)
	}
	fmt.Fprintf(out, "- cloud.base_url: %s\n", cfg.Cloud.BaseURL)
	fmt.Fprintf(out, "- websocket: %s (%s)\n", enabledText(cfg.Cloud.WSEnabled), cfg.Cloud.WSPath)
	fmt.Fprintf(out, "- pull: %s (%s)\n", enabledText(cfg.Cloud.PullEnabled), cfg.Cloud.PullPath)
	fmt.Fprintf(out, "- result_path: %s\n", cfg.Cloud.ResultPath)
	fmt.Fprintf(out, "- work_dir: %s\n", cfg.Agent.WorkDir)
	fmt.Fprintf(out, "- audit_file: %s\n", cfg.Logging.AuditFile)
	fmt.Fprintf(out, "- outbox_dir: %s\n", filepath.Join(cfg.Agent.WorkDir, "outbox"))
	fmt.Fprintf(out, "- credentials_file: %s\n", cfg.Security.CredentialsFile)
	fmt.Fprintf(out, "- credentials.device_secret: %s\n", "***REDACTED***")
	fmt.Fprintf(out, "- outbox_pending: %d\n", outboxPendingCount(cfg.Agent.WorkDir))

	allowed := append([]string(nil), cfg.Security.AllowActions...)
	sort.Strings(allowed)
	fmt.Fprintln(out, "\n允许 action：")
	for _, action := range allowed {
		fmt.Fprintf(out, "- %s\n", action)
	}
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func outboxPendingCount(workDir string) int {
	store, err := outbox.NewStore(outbox.Options{Dir: filepath.Join(workDir, "outbox")})
	if err != nil {
		return 0
	}
	return store.PendingCount()
}

func doNetworkProbe(ctx context.Context, out interface {
	Write([]byte) (int, error)
}, baseURL string) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, baseURL, nil)
	if err != nil {
		fmt.Fprintf(out, "\n[WARN] cloud base_url 网络探测跳过: %v\n", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(out, "\n[WARN] cloud base_url 暂不可达: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Fprintf(out, "\n[OK] cloud base_url 可达: HTTP %d\n", resp.StatusCode)
}
