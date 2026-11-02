BINDIR := $(HOME)/.local/bin
BIN := bin/hypercube
SERVICE := com.github.kkestell.hypercube
PORT := 4280

.PHONY: build check install install-service dev dev-stop dev-clean ui

build:
	go build -o $(BIN) ./cmd/hypercube
	go build -o bin/hypercube-harness-claude ./plugins/claude
	go build -o bin/hypercube-harness-opencode ./plugins/opencode
	go build -o bin/hypercube-harness-codex ./plugins/codex
	go build -o bin/hypercube-harness-pi ./plugins/pi

# check runs every check, and fails when internal/ui/dist differs from a fresh
# build of its sources.
check: build
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && cd internal/ui && \
		bun install --frozen-lockfile && bun build.js "$$tmp" && \
		{ diff -rq dist "$$tmp" || { echo "internal/ui/dist is stale; run make ui"; exit 1; }; }
	cd internal/ui && bun test
	go vet ./...
	@test -z "$$(gofmt -l cmd internal plugins)" || { gofmt -l cmd internal plugins; exit 1; }
	go test -race ./...

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 $(BIN) bin/hypercube-harness-* "$(BINDIR)"

# install-service installs the build and restarts the login service.
install-service: install
	BINDIR="$(BINDIR)" SERVICE="$(SERVICE)" PORT="$(PORT)" scripts/install-service

# ui builds the browser view into internal/ui/dist, which is committed and
# embedded, so go build needs no bun.
ui:
	cd internal/ui && bun install --frozen-lockfile && bun build.js dist

dev:
	scripts/dev up

dev-stop:
	scripts/dev stop

dev-clean:
	scripts/dev clean
