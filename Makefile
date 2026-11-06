BINDIR := $(HOME)/.local/bin
APPDIR := $(HOME)/Applications
APP := bin/Hypercube.app
APP_ID := com.github.kkestell.hypercube
APP_NAME := Hypercube
DESKTOP_EXE := Hypercube Desktop
ARCH := $(shell uname -m | sed s/x86_64/x64/)
SERVICE := com.github.kkestell.hypercube
PORT := 4280
PLUGINS := claude opencode codex pi
MACOS := $(APP)/Contents/MacOS
LSREGISTER := /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister

.PHONY: binaries build bundle check release install place quit-desktop install-service dev dev-app dev-stop dev-clean ui screenshots web-build web-serve

# bundle builds $(APP): the Electron desktop app, Hypercube Desktop, with
# hypercube and the harness plugins side by side in Contents/MacOS, signed ad
# hoc. The icons are rendered from cmd/hypercube/AppIcon.svg and
# desktop/menubar.svg. build also links hypercube and the plugins into bin/.
bundle: ui bin/AppIcon.icns bin/trayTemplate.png bin/trayTemplate@2x.png
	cd desktop && ELECTRON_SKIP_BINARY_DOWNLOAD=1 bun install --frozen-lockfile
	rm -rf $(APP)
	mkdir -p $(dir $(APP))
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
		(cd desktop && bunx --bun electron-packager . "$(APP_NAME)" --platform=darwin --arch=$(ARCH) --out="$$tmp" \
			--no-asar --app-version=0.0.0 --app-bundle-id=$(APP_ID) --executable-name="$(DESKTOP_EXE)" \
			--ignore='^/node_modules($$|/)' --ignore='^/bun\.lock$$' --ignore='\.test\.js$$' --ignore='^/menubar\.svg$$') && \
		mv "$$tmp"/*/*.app $(APP)
	plutil -replace productName -string "$(APP_NAME)" $(APP)/Contents/Resources/app/package.json
	rm $(APP)/Contents/Resources/electron.icns
	cp bin/AppIcon.icns bin/trayTemplate.png bin/trayTemplate@2x.png $(APP)/Contents/Resources/
	cp -R internal/ui/dist $(APP)/Contents/Resources/ui
	cp scripts/install-service $(APP)/Contents/Resources/install-service
	plutil -replace CFBundleIconFile -string AppIcon $(APP)/Contents/Info.plist
	plutil -replace CFBundleDisplayName -string "$(APP_NAME)" $(APP)/Contents/Info.plist
	$(MAKE) binaries OUT=$(MACOS)
	@info=$$("$(MACOS)/hypercube" version) && \
		version=$$(printf '%s\n' "$$info" | awk '{print $$2}' | sed -E 's/^v//; s/[-+].*//') && \
		{ [ "$$version" != devel ] || version=0.0.0; } && \
		plutil -replace CFBundleShortVersionString -string "$$version" $(APP)/Contents/Info.plist && \
		plutil -replace CFBundleVersion -string "$$version" $(APP)/Contents/Info.plist && \
		plutil -replace CFBundleGetInfoString -string "$$info" $(APP)/Contents/Info.plist
	for p in $(PLUGINS); do codesign --force --sign - $(MACOS)/hypercube-harness-$$p || exit 1; done
	codesign --force --deep --sign - $(APP)

bin/AppIcon.icns: cmd/hypercube/AppIcon.svg
	mkdir -p bin
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && mkdir "$$tmp/AppIcon.iconset" && \
		for s in 16 32 128 256 512; do \
			sips -s format png -z $$s $$s "$<" --out "$$tmp/AppIcon.iconset/icon_$${s}x$${s}.png" >/dev/null && \
			sips -s format png -z $$((s*2)) $$((s*2)) "$<" --out "$$tmp/AppIcon.iconset/icon_$${s}x$${s}@2x.png" >/dev/null || exit 1; \
		done && \
		iconutil -c icns -o "$@" "$$tmp/AppIcon.iconset"

# The menu bar icon is a template image, which macOS tints for the menu bar.
bin/trayTemplate.png: desktop/menubar.svg
	mkdir -p bin
	sips -s format png -z 18 18 "$<" --out "$@" >/dev/null

bin/trayTemplate@2x.png: desktop/menubar.svg
	mkdir -p bin
	sips -s format png -z 36 36 "$<" --out "$@" >/dev/null

# binaries builds hypercube and the harness plugins side by side in $(OUT).
binaries:
	go build -o $(OUT)/hypercube ./cmd/hypercube
	for p in $(PLUGINS); do go build -o $(OUT)/hypercube-harness-$$p ./plugins/$$p || exit 1; done

build: bundle
	ln -sf Hypercube.app/Contents/MacOS/hypercube bin/hypercube
	for p in $(PLUGINS); do ln -sf Hypercube.app/Contents/MacOS/hypercube-harness-$$p bin/hypercube-harness-$$p; done

# check builds the programs and website without packaging the app.
check: ui web-build
	go build ./cmd/... ./plugins/...
	cd internal/ui && bunx tsc -p . && bun test
	cd desktop && ELECTRON_SKIP_BINARY_DOWNLOAD=1 bun install --frozen-lockfile && bun test
	go vet ./...
	@test -z "$$(gofmt -l cmd internal plugins)" || { gofmt -l cmd internal plugins; exit 1; }
	go test -race ./...

# Official packaging has stricter requirements than checkout builds/installs.
release:
	scripts/release

# install puts the bundle in APPDIR and links hypercube and the plugins into
# BINDIR, replacing the executables of earlier installs.
install: build place

place:
	mkdir -p "$(APPDIR)" "$(BINDIR)"
	rm -rf "$(APPDIR)/Hypercube.app"
	cp -R $(APP) "$(APPDIR)/Hypercube.app"
	$(LSREGISTER) -f "$(APPDIR)/Hypercube.app"
	ln -sf "$(APPDIR)/Hypercube.app/Contents/MacOS/hypercube" "$(BINDIR)/hypercube"
	for p in $(PLUGINS); do ln -sf "$(APPDIR)/Hypercube.app/Contents/MacOS/hypercube-harness-$$p" "$(BINDIR)/hypercube-harness-$$p"; done

# quit-desktop quits the installed desktop app when it is running. The backend
# and its agents keep running. An app with this ID running another executable
# is an earlier build's backend, whose quit ends its agents; install-service
# stops it with SIGTERM instead.
quit-desktop:
	@if lsappinfo info -only executablepath -app $(APP_ID) | grep -q '/$(DESKTOP_EXE)"$$'; then \
		osascript -e 'tell application id "$(APP_ID)" to quit'; \
	fi

# install-service installs the build, restarts the login service, and opens
# the app. Agents keep running through the restart.
install-service: build quit-desktop place
	APP="$(APPDIR)/Hypercube.app" SERVICE="$(SERVICE)" PORT="$(PORT)" scripts/install-service
	open "$(APPDIR)/Hypercube.app"

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
		go build -o $$tmp/hypercube ./cmd/hypercube && \
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

# web-build builds the Taikonaut website into web/public. web-serve runs its dev
# server at http://localhost:1313. Both go through web/Makefile, which installs
# the pinned Hugo into web/bin on first use.
web-build:
	$(MAKE) -C web build

web-serve:
	$(MAKE) -C web serve
