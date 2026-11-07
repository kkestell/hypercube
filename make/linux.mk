APP := bin/Hypercube
INSTALLED := $(or $(XDG_DATA_HOME),$(HOME)/.local/share)/hypercube
SERVICE := $(APP_ID)

.PHONY: build bundle install install-service uninstall

# bundle builds $(APP): the Electron desktop app, hypercube-desktop, with
# hypercube and the harness plugins side by side at the top, and the pages,
# icon, and launcher file the app reads in resources/, beside the installer
# scripts. The icon is rendered from cmd/hypercube/AppIcon.svg. build also
# links hypercube and the plugins into bin/.
bundle: ui bin/$(APP_ID).png
	cd desktop && ELECTRON_SKIP_BINARY_DOWNLOAD=1 bun install --frozen-lockfile
	rm -rf $(APP)
	mkdir -p $(dir $(APP))
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
		(cd desktop && $(PACKAGER) --platform=linux --out="$$tmp" --executable-name=hypercube-desktop) && \
		mv "$$tmp"/* $(APP)
	cp -R internal/ui/dist $(APP)/resources/ui
	cp bin/$(APP_ID).png desktop/$(APP_ID).desktop $(APP)/resources/
	cp scripts/linux-install $(APP)/resources/install
	cp scripts/linux-install-service $(APP)/resources/install-service
	cp scripts/linux-uninstall $(APP)/resources/uninstall
	$(MAKE) binaries OUT=$(APP)

bin/$(APP_ID).png: cmd/hypercube/AppIcon.svg
	mkdir -p bin
	rsvg-convert -w 512 -h 512 -o $@ $<

build: bundle
	ln -sf Hypercube/hypercube bin/hypercube
	for p in $(PLUGINS); do ln -sf Hypercube/hypercube-harness-$$p bin/hypercube-harness-$$p; done

# install puts the bundle in ~/.local/share/hypercube, links hypercube and the
# plugins into BINDIR, and adds the launcher entry and icon.
install: build
	BINDIR="$(BINDIR)" $(APP)/resources/install

# install-service installs the bundle, then runs the backend as a systemd user
# service that starts at login, replacing a running copy while agents keep
# running.
install-service: install
	APP="$(INSTALLED)" SERVICE="$(SERVICE)" PORT="$(PORT)" "$(INSTALLED)/resources/install-service"

# uninstall removes the service and everything install adds. Running agents end.
uninstall:
	BINDIR="$(BINDIR)" scripts/linux-uninstall
