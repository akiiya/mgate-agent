package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mgate-agent/internal/actions"
	"mgate-agent/internal/protocol"
)

func TestRunnerExecutesFakeMGate(t *testing.T) {
	result := runWithMode(t, "", 1024, 2*time.Second)

	if result.State != protocol.CommandSucceeded {
		t.Fatalf("state = %s, want succeeded: %+v", result.State, result)
	}
	if !strings.Contains(result.Stdout, "args=status,--json") {
		t.Fatalf("stdout did not include argv: %q", result.Stdout)
	}
}

func TestRunnerCapturesStderr(t *testing.T) {
	result := runWithMode(t, "stderr", 1024, 2*time.Second)

	if result.State != protocol.CommandSucceeded {
		t.Fatalf("state = %s, want succeeded: %+v", result.State, result)
	}
	if !strings.Contains(result.Stdout, "fake stdout") {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "fake stderr") {
		t.Fatalf("stderr = %q", result.Stderr)
	}
}

func TestRunnerTimeout(t *testing.T) {
	result := runWithMode(t, "sleep", 1024, 50*time.Millisecond)

	if result.State != protocol.CommandTimedOut {
		t.Fatalf("state = %s, want timed_out: %+v", result.State, result)
	}
	if result.ErrorCode != "timeout" {
		t.Fatalf("error_code = %q", result.ErrorCode)
	}
}

func TestRunnerCapturesNonZeroExit(t *testing.T) {
	result := runWithMode(t, "fail", 1024, 2*time.Second)

	if result.State != protocol.CommandFailed {
		t.Fatalf("state = %s, want failed: %+v", result.State, result)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit_code = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "fake failure") {
		t.Fatalf("stderr = %q", result.Stderr)
	}
}

func TestRunnerTruncatesCombinedOutput(t *testing.T) {
	result := runWithMode(t, "bigout", 100, 2*time.Second)

	if result.State != protocol.CommandSucceeded {
		t.Fatalf("state = %s, want succeeded: %+v", result.State, result)
	}
	if !result.OutputTruncated {
		t.Fatal("OutputTruncated = false, want true")
	}
	if got := len(result.Stdout) + len(result.Stderr); got > 100 {
		t.Fatalf("combined output length = %d, want <= 100", got)
	}
}

func TestRunnerRedactsSensitiveOutputLines(t *testing.T) {
	result := runWithMode(t, "secretout", 1024, 2*time.Second)

	for _, leaked := range []string{"token=abc", "password=123"} {
		if strings.Contains(result.Stdout, leaked) || strings.Contains(result.Stderr, leaked) {
			t.Fatalf("result leaked sensitive output %q: stdout=%q stderr=%q", leaked, result.Stdout, result.Stderr)
		}
	}
	if !strings.Contains(result.Stdout, "safe stdout") {
		t.Fatalf("safe stdout line was removed: %q", result.Stdout)
	}
}

func runWithMode(t *testing.T, mode string, maxOutput int, timeout time.Duration) protocol.ResultPayload {
	t.Helper()
	t.Setenv("MGATE_FAKE_MODE", mode)

	registry, err := actions.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	spec, args, err := registry.Validate("status.snapshot", nil)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	return Run(context.Background(), Request{
		CommandID:      "cmd_test",
		Spec:           spec,
		Args:           args,
		MGatePath:      fakeMGatePath(t),
		Timeout:        timeout,
		MaxOutputBytes: maxOutput,
	})
}

func fakeMGatePath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fake-mgate.sh"))
		if err != nil {
			t.Fatalf("ReadFile(fake-mgate.sh) error = %v", err)
		}
		path := filepath.Join(t.TempDir(), "fake-mgate.sh")
		if err := os.WriteFile(path, data, 0o755); err != nil {
			t.Fatalf("WriteFile(fake-mgate.sh) error = %v", err)
		}
		return path
	}
	return buildFakeMGate(t)
}

func buildFakeMGate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "fake-mgate.exe")
	source := `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	switch os.Getenv("MGATE_FAKE_MODE") {
	case "stderr":
		fmt.Fprintln(os.Stderr, "fake stderr")
		fmt.Println("fake stdout")
	case "sleep":
		time.Sleep(2 * time.Second)
		fmt.Println("late stdout")
	case "fail":
		fmt.Fprintln(os.Stderr, "fake failure")
		os.Exit(7)
	case "bigout":
		fmt.Print(strings.Repeat("O", 8192))
		fmt.Fprint(os.Stderr, strings.Repeat("E", 8192))
	case "secretout":
		fmt.Println("safe stdout")
		fmt.Println("token=abc")
		fmt.Fprintln(os.Stderr, "password=123")
	default:
		fmt.Printf("args=%s\n", strings.Join(os.Args[1:], ","))
	}
}
`
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile(helper source) error = %v", err)
	}

	goExe := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goExe += ".exe"
	}
	if _, err := os.Stat(goExe); err != nil {
		goExe = "go"
	}
	cmd := exec.Command(goExe, "build", "-o", bin, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake mgate error = %v\n%s", err, string(out))
	}
	return bin
}
