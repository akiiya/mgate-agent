package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"mgate-agent/internal/config"
)

func TestConfigDefaultOutputsParsableYAML(t *testing.T) {
	stdout := captureStdout(t, func() {
		if err := run([]string{"config", "default"}); err != nil {
			t.Fatalf("run(config default) error = %v", err)
		}
	})

	if stdout != config.DefaultConfigYAML {
		t.Fatal("config default output differs from DefaultConfigYAML")
	}
	if _, err := config.DecodeYAML(strings.NewReader(stdout)); err != nil {
		t.Fatalf("config default output is not valid YAML config: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	return buf.String()
}
