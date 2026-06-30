package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionFileIsNotUsedAsReleaseSource(t *testing.T) {
	path := filepath.Join("..", "..", "VERSION")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("VERSION file should not exist; release version comes from GitHub Release tag")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(VERSION) error = %v", err)
	}
}

func TestReleaseTargetsRequireExplicitVersionAndPackageSafeContents(t *testing.T) {
	data := readRepoFile(t, "Makefile")
	for _, want := range []string{
		"VERSION ?=",
		"validate-version:",
		"Please run make release VERSION=<tag>.",
		"release: validate-version clean release-linux-amd64 release-linux-arm64 release-linux-armv7 checksums",
		"checksums: validate-version",
		"sha256sum $(BINARY)-$(VERSION)-linux-amd64.tar.gz $(BINARY)-$(VERSION)-linux-arm64.tar.gz $(BINARY)-$(VERSION)-linux-armv7.tar.gz > checksums.txt",
		"verify-release: validate-version",
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
	for _, forbidden := range []string{
		"VERSION_FILE := VERSION",
		"VERSION ?= $(shell cat $(VERSION_FILE) 2>/dev/null)",
		"VERSION ?= v0.1.0-rc1",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("Makefile contains forbidden version source %q", forbidden)
		}
	}
}

func TestDevWorkflowChecksCodeOnly(t *testing.T) {
	data := readRepoFile(t, ".github", "workflows", "ci.yml")
	for _, want := range []string{
		"name: Dev Verification",
		"branches: [\"dev\"]",
		"pull_request:",
		"branches: [\"main\"]",
		"contents: read",
		"go vet ./...",
		"go test ./...",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("ci.yml missing dev verification marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"make validate-version",
		"go build -o bin/mgate-agent ./cmd/mgate-agent",
		"make build-linux-amd64",
		"make build-linux-arm64",
		"make build-linux-armv7",
		"make release",
		"make verify-release",
		"sha256sum -c checksums.txt",
		"gh release create",
		"gh release upload",
		"git tag",
		"git push origin",
		"actions/upload-artifact",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("dev workflow contains forbidden release/build behavior %q", forbidden)
		}
	}
}

func TestReleaseWorkflowUploadsAssetsForPublishedRelease(t *testing.T) {
	data := readRepoFile(t, ".github", "workflows", "main-release.yml")
	for _, want := range []string{
		"name: Release Assets",
		"release:",
		"types: [published]",
		"contents: write",
		"ref: ${{ github.event.release.tag_name }}",
		"VERSION=\"${{ github.event.release.tag_name }}\"",
		"go vet ./...",
		"go test ./...",
		"go build -o bin/mgate-agent ./cmd/mgate-agent",
		"make build-linux-amd64",
		"make build-linux-arm64",
		"make build-linux-armv7",
		"make release VERSION=${{ steps.version.outputs.version }}",
		"make verify-release VERSION=${{ steps.version.outputs.version }}",
		"sha256sum -c checksums.txt",
		"GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
		"gh release view \"$VERSION\" --json assets",
		"gh release upload \"$VERSION\"",
		"dist/checksums.txt",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("main-release.yml missing release marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"branches: [\"main\"]",
		"Read and validate VERSION",
		"git tag -a",
		"git push origin",
		"gh release create",
		"--verify-tag",
		"actions/upload-artifact",
		"--clobber",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("release workflow contains forbidden marker %q", forbidden)
		}
	}
}
