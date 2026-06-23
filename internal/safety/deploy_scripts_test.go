package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptDoesNotOverwriteConfigOrGenerateCredentials(t *testing.T) {
	data := readRepoFile(t, "scripts", "install.sh")
	for _, want := range []string{
		"if [ ! -f /etc/mgate-agent/agent.yaml ]",
		"安装脚本不会生成 credentials",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("install.sh missing safety marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"> /etc/mgate-agent/agent.yaml",
		"device_secret",
		"fake credentials",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("install.sh contains forbidden text %q", forbidden)
		}
	}
}

func TestUninstallScriptPreservesConfigCredentialsAndData(t *testing.T) {
	data := readRepoFile(t, "scripts", "uninstall.sh")
	for _, want := range []string{
		"默认保留以下目录",
		"/etc/mgate-agent",
		"/var/lib/mgate-agent",
		"/var/log/mgate-agent",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("uninstall.sh missing preservation marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"rm -rf /etc/mgate-agent",
		"rm -rf /var/lib/mgate-agent",
		"rm -rf /var/log/mgate-agent",
		"credentials.json",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("uninstall.sh contains forbidden text %q", forbidden)
		}
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
