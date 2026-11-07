APP := bin/Hypercube

.PHONY: build bundle

# bundle builds $(APP): the Electron desktop app, hypercube-desktop, with
# hypercube and the harness plugins side by side at the top, and the pages,
# icon, and launcher file the app reads in resources/. The icon is rendered
# from cmd/hypercube/AppIcon.svg. build also links hypercube and the plugins
# into bin/.
bundle: ui bin/$(APP_ID).png
	cd desktop && ELECTRON_SKIP_BINARY_DOWNLOAD=1 bun install --frozen-lockfile
	rm -rf $(APP)
	mkdir -p $(dir $(APP))
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
		(cd desktop && $(PACKAGER) --platform=linux --out="$$tmp" --executable-name=hypercube-desktop) && \
		mv "$$tmp"/* $(APP)
	cp -R internal/ui/dist $(APP)/resources/ui
	cp bin/$(APP_ID).png desktop/$(APP_ID).desktop $(APP)/resources/
	$(MAKE) binaries OUT=$(APP)

bin/$(APP_ID).png: cmd/hypercube/AppIcon.svg
	mkdir -p bin
	rsvg-convert -w 512 -h 512 -o $@ $<

build: bundle
	ln -sf Hypercube/hypercube bin/hypercube
	for p in $(PLUGINS); do ln -sf Hypercube/hypercube-harness-$$p bin/hypercube-harness-$$p; done
