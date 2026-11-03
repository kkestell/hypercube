BINDIR := $(HOME)/.local/bin
APPDIR := $(HOME)/Applications
APP := bin/Hypercube.app
APP_ID := com.github.kkestell.hypercube
APP_NAME := Hypercube
SERVICE := com.github.kkestell.hypercube
PORT := 4280
PLUGINS := claude opencode codex pi
MACOS := $(APP)/Contents/MacOS
LSREGISTER := /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister

.PHONY: build bundle check install install-service dev dev-stop dev-clean ui

# bundle builds $(APP), the application bundle macOS notifications need, with
# hypercube and the harness plugins side by side in Contents/MacOS, signed ad
# hoc. build also links them into bin/.
bundle:
	mkdir -p $(MACOS)
	cp cmd/hypercube/Info.plist $(APP)/Contents/Info.plist
	plutil -replace CFBundleIdentifier -string $(APP_ID) $(APP)/Contents/Info.plist
	plutil -replace CFBundleName -string "$(APP_NAME)" $(APP)/Contents/Info.plist
	go build -o $(MACOS)/hypercube ./cmd/hypercube
	for p in $(PLUGINS); do go build -o $(MACOS)/hypercube-harness-$$p ./plugins/$$p && codesign --force --sign - $(MACOS)/hypercube-harness-$$p || exit 1; done
	codesign --force --sign - $(APP)

build: bundle
	ln -sf Hypercube.app/Contents/MacOS/hypercube bin/hypercube
	for p in $(PLUGINS); do ln -sf Hypercube.app/Contents/MacOS/hypercube-harness-$$p bin/hypercube-harness-$$p; done

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

# install puts the bundle in APPDIR and links hypercube and the plugins into
# BINDIR, replacing the executables of earlier installs.
install: build
	mkdir -p "$(APPDIR)" "$(BINDIR)"
	rm -rf "$(APPDIR)/Hypercube.app"
	cp -R $(APP) "$(APPDIR)/Hypercube.app"
	$(LSREGISTER) -f "$(APPDIR)/Hypercube.app"
	ln -sf "$(APPDIR)/Hypercube.app/Contents/MacOS/hypercube" "$(BINDIR)/hypercube"
	for p in $(PLUGINS); do ln -sf "$(APPDIR)/Hypercube.app/Contents/MacOS/hypercube-harness-$$p" "$(BINDIR)/hypercube-harness-$$p"; done

# install-service installs the build and restarts the login service.
install-service: install
	APPDIR="$(APPDIR)" SERVICE="$(SERVICE)" PORT="$(PORT)" scripts/install-service

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
