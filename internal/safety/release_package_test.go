package safety

import (
	"regexp"
	"strings"
	"testing"
)

func TestVersionFileIsSingleValidSource(t *testing.T) {
	versionFile := readRepoFile(t, "VERSION")
	version := strings.TrimSpace(versionFile)
	if version == "" {
		t.Fatal("VERSION is empty")
	}
	if strings.Count(strings.TrimRight(versionFile, "\r\n"), "\n") != 0 {
		t.Fatal("VERSION must contain exactly one effective line")
	}
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`).MatchString(version) {
		t.Fatalf("VERSION has invalid format: %q", version)
	}
}

func TestReleaseTargetsReadVersionFileAndPackageSafeContents(t *testing.T) {
	data := readRepoFile(t, "Makefile")
	for _, want := range []string{
		"VERSION_FILE := VERSION",
		"VERSION ?= $(shell cat $(VERSION_FILE) 2>/dev/null)",
		"validate-version:",
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
	if strings.Contains(data, "VERSION ?= "+"v0.1.0-rc1") {
		t.Fatal("Makefile must not hardcode the default release version")
	}
}

func TestDevWorkflowVerifiesOnly(t *testing.T) {
	data := readRepoFile(t, ".github", "workflows", "ci.yml")
	for _, want := range []string{
		"name: Dev Verification",
		"branches: [\"dev\"]",
		"pull_request:",
		"branches: [\"main\"]",
		"contents: read",
		"make validate-version",
		"go vet ./...",
		"go test ./...",
		"go build -o bin/mgate-agent ./cmd/mgate-agent",
		"make build-linux-amd64",
		"make build-linux-arm64",
		"make build-linux-armv7",
		"make release",
		"make verify-release",
		"sha256sum -c checksums.txt",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("ci.yml missing dev verification marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"gh release create",
		"git tag",
		"git push origin",
		"actions/upload-artifact",
		"VERSION=" + "v0.1.0-rc1",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("dev workflow contains forbidden release behavior %q", forbidden)
		}
	}
}

func TestMainWorkflowPublishesFromVersionFile(t *testing.T) {
	data := readRepoFile(t, ".github", "workflows", "main-release.yml")
	for _, want := range []string{
		"name: Main Release",
		"branches: [\"main\"]",
		"fetch-depth: 0",
		"contents: write",
		"Read and validate VERSION",
		"git ls-remote --exit-code --tags origin",
		"gh release view \"$VERSION\"",
		"go vet ./...",
		"go test ./...",
		"go build -o bin/mgate-agent ./cmd/mgate-agent",
		"make release VERSION=${{ steps.version.outputs.version }}",
		"make verify-release VERSION=${{ steps.version.outputs.version }}",
		"sha256sum -c checksums.txt",
		"git tag -a \"$VERSION\" -m \"mgate-agent ${VERSION}\"",
		"git push origin \"refs/tags/${VERSION}\"",
		"GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
		"gh release create \"$VERSION\"",
		"--verify-tag",
		"*-rc*|*-beta*|*-alpha*",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("main-release.yml missing release marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"tags:",
		"VERSION=" + "v0.1.0-rc1",
		"workflow_dispatch:\n    inputs:",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("main workflow contains forbidden marker %q", forbidden)
		}
	}
}
