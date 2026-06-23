package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

type Credentials struct {
	DeviceID      string    `json:"device_id"`
	TenantID      string    `json:"tenant_id"`
	DeviceSecret  string    `json:"device_secret"`
	SecretVersion int       `json:"secret_version"`
	EnrolledAt    time.Time `json:"enrolled_at"`
	CloudBaseURL  string    `json:"cloud_base_url"`
}

func Load(path string) (*Credentials, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat credentials %q: %w", path, err)
	}
	if err := validateFileMode(info.Mode().Perm(), runtime.GOOS); err != nil {
		return nil, fmt.Errorf("invalid credentials permissions %q: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credentials %q: %w", path, err)
	}

	var cred Credentials
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cred); err != nil {
		return nil, fmt.Errorf("decode credentials %q: %w", path, err)
	}
	if err := cred.Validate(); err != nil {
		return nil, fmt.Errorf("invalid credentials %q: %w", path, err)
	}
	return &cred, nil
}

func (c *Credentials) Validate() error {
	if c == nil {
		return errors.New("credentials is nil")
	}
	if strings.TrimSpace(c.DeviceID) == "" {
		return errors.New("device_id is required")
	}
	if strings.TrimSpace(c.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(c.DeviceSecret) == "" {
		// 这里只说明字段缺失，不回显任何 secret 值，避免错误链路泄露敏感信息。
		return errors.New("device_secret is required")
	}
	if c.SecretVersion <= 0 {
		return errors.New("secret_version must be positive")
	}
	if c.EnrolledAt.IsZero() {
		return errors.New("enrolled_at is required")
	}
	if c.CloudBaseURL != "" {
		u, err := url.Parse(c.CloudBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("cloud_base_url must be a valid http or https URL")
		}
	}
	return nil
}

func validateFileMode(mode fs.FileMode, goos string) error {
	if goos != "linux" {
		return nil
	}
	// Linux 设备上的凭证文件必须只允许 owner 读写。device_secret 是设备身份根密钥，
	// 不能依赖上层日志脱敏来补救文件权限过宽的问题。
	if mode != 0o600 {
		return fmt.Errorf("must be 0600 on Linux, got %04o", mode)
	}
	return nil
}
