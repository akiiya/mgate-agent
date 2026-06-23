package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCredentialsSuccess(t *testing.T) {
	path := writeCredentials(t, validCredentials())

	cred, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cred.DeviceID != "dev_test" {
		t.Fatalf("unexpected device_id: %s", cred.DeviceID)
	}
}

func TestValidateFileModeRejectsNon0600OnLinux(t *testing.T) {
	if err := validateFileMode(0o644, "linux"); err == nil {
		t.Fatal("validateFileMode() expected error")
	}
}

func TestLoadErrorDoesNotLeakDeviceSecret(t *testing.T) {
	cred := validCredentials()
	cred.DeviceID = ""
	path := writeCredentials(t, cred)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error")
	}
	if strings.Contains(err.Error(), cred.DeviceSecret) {
		t.Fatalf("error leaked device_secret: %v", err)
	}
}

func validCredentials() Credentials {
	return Credentials{
		DeviceID:      "dev_test",
		TenantID:      "tenant_test",
		DeviceSecret:  "super-secret-value",
		SecretVersion: 1,
		EnrolledAt:    time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		CloudBaseURL:  "https://mgate.example.com",
	}
}

func writeCredentials(t *testing.T, cred Credentials) string {
	t.Helper()
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	return path
}
