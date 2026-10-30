# Pinned to the Ghostty commit the libghostty Go bindings build against, so
# the C API matches.
GHOSTTY_COMMIT := f54340ad77814fe319d136e5e915d6a48de8dd23
GHOSTTY_VT := $(CURDIR)/build/ghostty-vt
# Relative, so the stamp can also be built by name from the command line.
STAMP := build/ghostty-vt/.built

export PKG_CONFIG_PATH := $(GHOSTTY_VT)/share/pkgconfig

.PHONY: build test check

build: $(STAMP)
	go build -o bin/mux ./cmd/mux

test: $(STAMP)
	go test ./...

check: $(STAMP)
	go vet ./...
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	go test -race ./...

$(STAMP):
	rm -rf build
	mkdir -p build/ghostty
	cd build/ghostty && git init -q \
		&& git fetch -q --depth 1 https://github.com/ghostty-org/ghostty.git $(GHOSTTY_COMMIT) \
		&& git checkout -q FETCH_HEAD
	cd build/ghostty && zig build -Demit-lib-vt -Demit-xcframework=false \
		-Doptimize=ReleaseFast --prefix $(GHOSTTY_VT)
	touch $@
