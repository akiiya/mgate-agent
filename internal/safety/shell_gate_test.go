package safety

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNoShellCommandExecutionPatterns(t *testing.T) {
	root := repoRoot(t)
	patterns := []string{
		"sh" + " -c",
		"bash" + " -c",
		"exec.Command(" + `"` + "sh" + `"`,
		"exec.Command(" + `"` + "bash" + `"`,
		"exec.CommandContext(" + "ctx, " + "userInput)",
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".gocache", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				t.Fatalf("unsafe shell execution pattern %q found in %s", pattern, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
