GO ?= go
BINARY := mgate-agent
VERSION ?= v0.1.0-rc1
DIST := dist

.PHONY: fmt test vet check build build-linux-amd64 build-linux-arm64 build-linux-armv7 clean release release-linux-amd64 release-linux-arm64 release-linux-armv7 checksums verify-release

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

check: fmt vet test

build:
	$(GO) build -o bin/$(BINARY) ./cmd/mgate-agent

build-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build -o bin/$(BINARY)-linux-amd64 ./cmd/mgate-agent

build-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build -o bin/$(BINARY)-linux-arm64 ./cmd/mgate-agent

build-linux-armv7:
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -o bin/$(BINARY)-linux-armv7 ./cmd/mgate-agent

clean:
	rm -rf $(DIST)

release: clean release-linux-amd64 release-linux-arm64 release-linux-armv7 checksums
	rm -rf $(DIST)/pkg

release-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build -o bin/$(BINARY)-linux-amd64 ./cmd/mgate-agent
	rm -rf $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64
	mkdir -p $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/configs $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/packaging/systemd $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/scripts
	cp bin/$(BINARY)-linux-amd64 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/$(BINARY)
	cp configs/agent.example.yaml $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/configs/
	cp packaging/systemd/mgate-agent.service $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/packaging/systemd/
	cp scripts/install.sh scripts/uninstall.sh $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/scripts/
	chmod 0755 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/$(BINARY) $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/scripts/install.sh $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/scripts/uninstall.sh
	cp -R docs $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/
	cp README.md LICENSE $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-amd64/
	tar -C $(DIST)/pkg -czf $(DIST)/$(BINARY)-$(VERSION)-linux-amd64.tar.gz $(BINARY)-$(VERSION)-linux-amd64

release-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build -o bin/$(BINARY)-linux-arm64 ./cmd/mgate-agent
	rm -rf $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64
	mkdir -p $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/configs $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/packaging/systemd $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/scripts
	cp bin/$(BINARY)-linux-arm64 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/$(BINARY)
	cp configs/agent.example.yaml $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/configs/
	cp packaging/systemd/mgate-agent.service $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/packaging/systemd/
	cp scripts/install.sh scripts/uninstall.sh $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/scripts/
	chmod 0755 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/$(BINARY) $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/scripts/install.sh $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/scripts/uninstall.sh
	cp -R docs $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/
	cp README.md LICENSE $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-arm64/
	tar -C $(DIST)/pkg -czf $(DIST)/$(BINARY)-$(VERSION)-linux-arm64.tar.gz $(BINARY)-$(VERSION)-linux-arm64

release-linux-armv7:
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -o bin/$(BINARY)-linux-armv7 ./cmd/mgate-agent
	rm -rf $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7
	mkdir -p $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/configs $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/packaging/systemd $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/scripts
	cp bin/$(BINARY)-linux-armv7 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/$(BINARY)
	cp configs/agent.example.yaml $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/configs/
	cp packaging/systemd/mgate-agent.service $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/packaging/systemd/
	cp scripts/install.sh scripts/uninstall.sh $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/scripts/
	chmod 0755 $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/$(BINARY) $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/scripts/install.sh $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/scripts/uninstall.sh
	cp -R docs $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/
	cp README.md LICENSE $(DIST)/pkg/$(BINARY)-$(VERSION)-linux-armv7/
	tar -C $(DIST)/pkg -czf $(DIST)/$(BINARY)-$(VERSION)-linux-armv7.tar.gz $(BINARY)-$(VERSION)-linux-armv7

checksums:
	cd $(DIST) && sha256sum $(BINARY)-$(VERSION)-linux-amd64.tar.gz $(BINARY)-$(VERSION)-linux-arm64.tar.gz $(BINARY)-$(VERSION)-linux-armv7.tar.gz > checksums.txt

verify-release:
	test -f $(DIST)/$(BINARY)-$(VERSION)-linux-amd64.tar.gz
	test -f $(DIST)/$(BINARY)-$(VERSION)-linux-arm64.tar.gz
	test -f $(DIST)/$(BINARY)-$(VERSION)-linux-armv7.tar.gz
	test -f $(DIST)/checksums.txt
	cd $(DIST) && sha256sum -c checksums.txt
	for arch in amd64 arm64 armv7; do \
		archive="$(DIST)/$(BINARY)-$(VERSION)-linux-$$arch.tar.gz"; \
		top="$(BINARY)-$(VERSION)-linux-$$arch"; \
		tar -tzf "$$archive" | grep -qx "$$top/$(BINARY)"; \
		tar -tzf "$$archive" | grep -qx "$$top/configs/agent.example.yaml"; \
		tar -tzf "$$archive" | grep -qx "$$top/packaging/systemd/mgate-agent.service"; \
		tar -tzf "$$archive" | grep -qx "$$top/scripts/install.sh"; \
		tar -tzf "$$archive" | grep -qx "$$top/scripts/uninstall.sh"; \
		tar -tzf "$$archive" | grep -q "^$$top/docs/"; \
		tar -tzf "$$archive" | grep -qx "$$top/README.md"; \
		tar -tzf "$$archive" | grep -qx "$$top/LICENSE"; \
		if tar -tzf "$$archive" | grep -E '(^|/)(\.git|credentials\.json|outbox)(/|$$)' >/dev/null; then exit 1; fi; \
	done
