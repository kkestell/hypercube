APP := bin/Hypercube.app
APPDIR := $(HOME)/Applications
DESKTOP_EXE := Hypercube Desktop
SERVICE := com.github.kkestell.hypercube
MACOS := $(APP)/Contents/MacOS
LSREGISTER := /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister

.PHONY: build bundle install place quit-desktop install-service

# bundle builds $(APP): the Electron desktop app, Hypercube Desktop, with
# hypercube and the harness plugins side by side in Contents/MacOS, signed ad
# hoc. The icons are rendered from cmd/hypercube/AppIcon.svg and
# desktop/menubar.svg. build also links hypercube and the plugins into bin/.
bundle: ui bin/AppIcon.icns bin/trayTemplate.png bin/trayTemplate@2x.png
	cd desktop && ELECTRON_SKIP_BINARY_DOWNLOAD=1 bun install --frozen-lockfile
	rm -rf $(APP)
	mkdir -p $(dir $(APP))
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
		(cd desktop && $(PACKAGER) --platform=darwin --out="$$tmp" \
			--app-bundle-id=$(APP_ID) --executable-name="$(DESKTOP_EXE)") && \
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

build: bundle
	ln -sf Hypercube.app/Contents/MacOS/hypercube bin/hypercube
	for p in $(PLUGINS); do ln -sf Hypercube.app/Contents/MacOS/hypercube-harness-$$p bin/hypercube-harness-$$p; done

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
