BINDIR := $(HOME)/.local/bin
BIN := bin/hypercube

.PHONY: build test check install reinstall dev dev-stop dev-clean ui

build:
	go build -o $(BIN) ./cmd/hypercube
	go build -o bin/hypercube-harness-claude ./plugins/claude
	go build -o bin/hypercube-harness-opencode ./plugins/opencode
	go build -o bin/hypercube-harness-codex ./plugins/codex
	go build -o bin/hypercube-harness-pi ./plugins/pi

test:
	go test ./...

check: build
	go vet ./...
	@test -z "$$(gofmt -l cmd internal plugins)" || { gofmt -l cmd internal plugins; exit 1; }
	go test -race ./...

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 $(BIN) bin/hypercube-harness-* "$(BINDIR)"

# ui regenerates the browser view's stylesheet from its Tailwind classes.
ui:
	cd internal/ui && bun install --frozen-lockfile && bun x tailwindcss -i style.css -o static/ui.css --minify

reinstall: build
	BINDIR="$(BINDIR)" scripts/reinstall

dev:
	scripts/dev up

dev-stop:
	scripts/dev stop

dev-clean:
	scripts/dev clean
