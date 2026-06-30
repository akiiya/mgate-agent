package mgate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"mgate-agent/internal/audit"
)

func TestMain(m *testing.M) {
	if os.Getenv("MGATE_TEST_HELPER") == "1" {
		fakeMGateMain()
		return
	}
	os.Exit(m.Run())
}

func TestCapabilitiesSuccess(t *testing.T) {
	client := helperClient(t, "ok")
	payload, err := client.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if schemaVersion(payload) != SupportedSchemaVersion {
		t.Fatalf("schema_version = %d", schemaVersion(payload))
	}
	if payload["agent_contract"] == nil {
		t.Fatal("capabilities should include agent_contract")
	}
}

func TestSnapshotSuccessAndSummaryRedactsSensitiveFields(t *testing.T) {
	client := helperClient(t, "ok")
	payload, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if schemaVersion(payload) != SupportedSchemaVersion {
		t.Fatalf("schema_version = %d", schemaVersion(payload))
	}
	summary := client.Summary(context.Background())
	if !summary.Available {
		t.Fatalf("summary should be available: %+v", summary)
	}
	if summary.Mode != "tproxy" || summary.OverallHealth != "healthy" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.WiFi["password"] != audit.RedactedValue {
		t.Fatalf("wifi password was not redacted: %+v", summary.WiFi)
	}
	if summary.Web["token"] != audit.RedactedValue {
		t.Fatalf("web token was not redacted: %+v", summary.Web)
	}
}

func TestMGateMissing(t *testing.T) {
	client := NewClient(Options{Path: t.TempDir() + string(os.PathSeparator) + "missing-mgate"})
	_, err := client.Snapshot(context.Background())
	assertCode(t, err, CodeMGateMissing)
}

func TestMGateNotExecutable(t *testing.T) {
	client := NewClient(Options{Path: t.TempDir()})
	_, err := client.Snapshot(context.Background())
	assertCode(t, err, CodeMGateNotExecutable)
}

func TestCapabilitiesTimeout(t *testing.T) {
	client := helperClient(t, "sleep")
	client.opts.CapabilitiesTimeout = 10 * time.Millisecond
	_, err := client.Capabilities(context.Background())
	assertCode(t, err, CodeCapabilitiesTimeout)
}

func TestSnapshotTimeout(t *testing.T) {
	client := helperClient(t, "sleep")
	client.opts.SnapshotTimeout = 10 * time.Millisecond
	_, err := client.Snapshot(context.Background())
	assertCode(t, err, CodeSnapshotTimeout)
}

func TestInvalidJSON(t *testing.T) {
	client := helperClient(t, "invalid-json")
	_, err := client.Capabilities(context.Background())
	assertCode(t, err, CodeCapabilitiesInvalidJSON)
	_, err = client.Snapshot(context.Background())
	assertCode(t, err, CodeSnapshotInvalidJSON)
}

func TestUnsupportedSchemaVersion(t *testing.T) {
	client := helperClient(t, "unsupported-schema")
	_, err := client.Capabilities(context.Background())
	assertCode(t, err, CodeCapabilitiesUnsupportedSchema)
	_, err = client.Snapshot(context.Background())
	assertCode(t, err, CodeSnapshotUnsupportedSchema)
}

func TestExecFailed(t *testing.T) {
	client := helperClient(t, "fail")
	_, err := client.Snapshot(context.Background())
	assertCode(t, err, CodeMGateExecFailed)
	if strings.Contains(err.Error(), "secret stderr") {
		t.Fatalf("error leaked stderr content: %v", err)
	}
}

func TestSummaryFailureDoesNotPanic(t *testing.T) {
	client := helperClient(t, "invalid-json")
	summary := client.Summary(context.Background())
	if summary.Available {
		t.Fatalf("summary should be unavailable: %+v", summary)
	}
	if summary.ErrorCode != CodeSnapshotInvalidJSON {
		t.Fatalf("error_code = %q", summary.ErrorCode)
	}
}

func helperClient(t *testing.T, mode string) *Client {
	t.Helper()
	t.Setenv("MGATE_TEST_HELPER", "1")
	t.Setenv("MGATE_TEST_MODE", mode)
	return NewClient(Options{
		Path:                os.Args[0],
		CapabilitiesTimeout: 2 * time.Second,
		SnapshotTimeout:     2 * time.Second,
	})
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s, got nil", want)
	}
	var mgateErr *Error
	if !errors.As(err, &mgateErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if mgateErr.Code != want {
		t.Fatalf("error code = %q, want %q (%v)", mgateErr.Code, want, err)
	}
}

func fakeMGateMain() {
	mode := os.Getenv("MGATE_TEST_MODE")
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	command := os.Args[1]
	switch mode {
	case "sleep":
		time.Sleep(300 * time.Millisecond)
		fmt.Println(`{"schema_version":1}`)
	case "invalid-json":
		fmt.Println(`{"schema_version":`)
	case "unsupported-schema":
		fmt.Println(`{"schema_version":2}`)
	case "fail":
		_, _ = fmt.Fprintln(os.Stderr, "secret stderr should not leak")
		os.Exit(7)
	default:
		switch command {
		case "capabilities-json":
			fmt.Println(`{"ok":true,"schema_version":1,"component":"mgate","version":"test","features":["json"],"commands":{"read_only":["agent-snapshot"]},"agent_contract":{"safe_poll_command":"agent-snapshot"}}`)
		case "agent-snapshot":
			fmt.Println(`{"ok":true,"schema_version":1,"component":"mgate","version":"test","mode":"tproxy","overall_health":"healthy","wifi":{"connected":true,"password":"secret"},"ap":{"enabled":true},"gateway":{"state":"ok"},"tproxy":{"enabled":true},"mihomo":{"running":true},"subscription":{"updated":true},"web":{"port":31888,"token":"abc"}}`)
		default:
			os.Exit(3)
		}
	}
}
