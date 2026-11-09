BINDIR := $(HOME)/.local/bin
APP_ID := com.github.kkestell.hypercube
APP_NAME := Hypercube
ARCH := $(shell go env GOARCH | sed s/amd64/x64/)
PORT := 4280
PLUGINS := claude opencode pi

# The packager options both platforms share; each bundle rule adds the
# platform, output directory, and executable name.
PACKAGER := bunx --bun electron-packager . "$(APP_NAME)" --arch=$(ARCH) \
	--no-asar --app-version=0.0.0 \
	--ignore='^/node_modules($$|/)' --ignore='^/bun\.lock$$' --ignore='\.test\.js$$' \
	--ignore='^/menubar\.svg$$' --ignore='\.desktop$$'

.DEFAULT_GOAL := bundle
ifeq ($(shell uname -s),Darwin)
include make/macos.mk
else
include make/linux.mk
endif

.PHONY: binaries check release dev dev-app dev-stop dev-clean linux-check linux-test linux-test-all ui screenshots

# binaries builds hypercube, its MCP bridge and holder, and the harness
# plugins side by side in $(OUT).
binaries:
	for c in hypercube hypercube-mcp hypercube-hold; do go build -o $(OUT)/$$c ./cmd/$$c || exit 1; done
	for p in $(PLUGINS); do go build -o $(OUT)/hypercube-harness-$$p ./plugins/$$p || exit 1; done

# check builds the programs without packaging the app.
check: ui
	go build ./cmd/... ./plugins/...
	cd internal/ui && bunx tsc -p . && bun test
	cd desktop && ELECTRON_SKIP_BINARY_DOWNLOAD=1 bun install --frozen-lockfile && bun test
	go vet ./...
	@test -z "$$(gofmt -l cmd internal plugins)" || { gofmt -l cmd internal plugins; exit 1; }
	go test -race ./...

# release packages a clean, tagged beta for this machine in bin/release.
release:
	scripts/release

# ui builds the browser view into internal/ui/dist, which the Go build embeds.
# Run it before go build, go vet, or go test.
ui:
	cd internal/ui && bun install --frozen-lockfile && bun build.ts dist

# screenshots regenerates docs/images from the scenes in docs/screenshots,
# running the real backend and desktop window with the scripted harness plugin
# installed under each harness name. The first run downloads Electron.
screenshots: ui
	cd desktop && bun install --frozen-lockfile && node node_modules/electron/install.js
	@tmp=$$(mktemp -d /tmp/hc-XXXXXX) && trap 'rm -rf "$$tmp"' EXIT && \
		for c in hypercube hypercube-mcp hypercube-hold; do go build -o $$tmp/$$c ./cmd/$$c || exit 1; done && \
		go build -o $$tmp/hypercube-harness-scripted ./plugins/scripted && \
		for p in $(PLUGINS); do ln -s hypercube-harness-scripted $$tmp/hypercube-harness-$$p || exit 1; done && \
		desktop/node_modules/.bin/electron scripts/screenshots/main.js --dir=$$tmp docs/screenshots/*.json

dev:
	scripts/dev up

dev-app:
	scripts/dev up --app

dev-stop:
	scripts/dev stop

dev-clean:
	scripts/dev clean

# LINUX_DISTRO chooses the Linux container's distribution: debian, ubuntu,
# fedora, or arch.
LINUX_DISTRO ?= debian
export LINUX_DISTRO

# linux-check runs make check in a fresh Linux container.
linux-check:
	scripts/linux/run

# linux-test runs the Linux tests in a fresh Linux container and copies their
# screenshots to bin/linux-test/$(LINUX_DISTRO)/.
linux-test:
	scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install && scripts/linux/test-desktop && scripts/linux/test-release'

# linux-test-all runs linux-test for each distribution that has an image for
# this machine's architecture, stopping at the first failure. Arch has AMD64
# images only.
LINUX_DISTROS = debian ubuntu fedora $(if $(filter x86_64,$(shell uname -m)),arch)
linux-test-all:
	for d in $(LINUX_DISTROS); do $(MAKE) linux-test LINUX_DISTRO=$$d || exit 1; done
