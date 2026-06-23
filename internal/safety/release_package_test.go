package safety

import (
	"strings"
	"testing"
)

func TestReleaseTargetsUseVersionedArchivesChecksumsAndSafeContents(t *testing.T) {
	data := readRepoFile(t, "Makefile")
	for _, want := range []string{
		"VERSION ?= v0.1.0-rc1",
		"release: clean release-linux-amd64 release-linux-arm64 release-linux-armv7 checksums",
		"checksums:",
		"sha256sum $(BINARY)-$(VERSION)-linux-amd64.tar.gz $(BINARY)-$(VERSION)-linux-arm64.tar.gz $(BINARY)-$(VERSION)-linux-armv7.tar.gz > checksums.txt",
		"verify-release:",
		"cp -R docs",
		"chmod 0755 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/$(BINARY)",
		"tar -C $(DIST)/pkg -czf",
		"$(BINARY)-$(VERSION)-linux-amd64",
		"$(BINARY)-$(VERSION)-linux-arm64",
		"$(BINARY)-$(VERSION)-linux-armv7",
		"grep -E '(^|/)(\\.git|credentials\\.json|outbox)(/|$$)'",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("Makefile release target missing %q", want)
		}
	}
}

func TestCIValidatesReleaseChecksumsAndUploadsArtifacts(t *testing.T) {
	data := readRepoFile(t, ".github", "workflows", "ci.yml")
	for _, want := range []string{
		"make release VERSION=v0.1.0-rc1",
		"make verify-release VERSION=v0.1.0-rc1",
		"sha256sum -c checksums.txt",
		"actions/upload-artifact@v4",
		"path: dist/",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("ci.yml missing release contract marker %q", want)
		}
	}
}

func TestReleaseWorkflowBuildsVersionedArtifacts(t *testing.T) {
	data := readRepoFile(t, ".github", "workflows", "release.yml")
	for _, want := range []string{
		"tags:",
		"make release VERSION=${{ steps.version.outputs.value }}",
		"make verify-release VERSION=${{ steps.version.outputs.value }}",
		"sha256sum -c checksums.txt",
		"actions/upload-artifact@v4",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("release.yml missing release contract marker %q", want)
		}
	}
}
