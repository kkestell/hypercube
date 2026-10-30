BINDIR := $(HOME)/.local/bin
BIN := bin/hypercube

.PHONY: build test check install reinstall dev dev-stop dev-clean

build:
	go build -o $(BIN) ./cmd/hypercube

test:
	go test ./...

check: build
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
	go test -race ./...
	$(MAKE) -C mux check

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 $(BIN) "$(BINDIR)/hypercube"

reinstall: build
	BINDIR="$(BINDIR)" scripts/reinstall

dev:
	scripts/dev up

dev-stop:
	scripts/dev stop

dev-clean:
	scripts/dev clean
