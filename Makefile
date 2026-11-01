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

check: build
	go vet ./...
	@test -z "$$(gofmt -l cmd internal plugins)" || { gofmt -l cmd internal plugins; exit 1; }
	go test -race ./...

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 $(BIN) bin/hypercube-harness-* "$(BINDIR)"

# install-service installs the build and restarts the login service.
install-service: install
	BINDIR="$(BINDIR)" SERVICE="$(SERVICE)" PORT="$(PORT)" scripts/install-service

# ui regenerates the browser view's stylesheet from its Tailwind classes.
ui:
	cd internal/ui && bun install --frozen-lockfile && bun x tailwindcss -i style.css -o static/ui.css --minify

dev:
	scripts/dev up

dev-stop:
	scripts/dev stop

dev-clean:
	scripts/dev clean
