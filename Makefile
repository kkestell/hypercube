ZIG_VERSION := 0.17.0
ZIG_NAME := zig-aarch64-macos-$(ZIG_VERSION)
ZIG_URL := https://ziglang.org/download/$(ZIG_VERSION)/$(ZIG_NAME).tar.xz
ZIG_SHA256 := b607e9b9234790a008116ae5bdb71c6243b84b9fb42a53a9e70fde41c06c536a
ZIG := .toolchain/$(ZIG_NAME)/zig
GHOSTTY := vendor/ghostty/build.zig
DOWNLOAD := .toolchain/download
BINDIR := $(HOME)/.local/bin

.PHONY: build test check install reinstall dev dev-stop dev-clean

build: $(ZIG) $(GHOSTTY)
	cargo build

test: $(ZIG) $(GHOSTTY)
	cargo test

check: $(ZIG) $(GHOSTTY)
	cargo build
	cargo clippy --all-targets -- -D warnings
	cargo fmt --check
	cargo test

install: $(ZIG) $(GHOSTTY)
	cargo build --release --bin hypercube
	mkdir -p $(BINDIR)
	install -m 755 target/release/hypercube $(BINDIR)/hypercube

reinstall: $(ZIG) $(GHOSTTY)
	cargo build --release --bin hypercube
	BINDIR=$(BINDIR) scripts/reinstall

dev: $(ZIG) $(GHOSTTY)
	scripts/dev up

dev-stop:
	scripts/dev stop

dev-clean:
	scripts/dev clean

$(ZIG):
	rm -rf $(DOWNLOAD)
	mkdir -p $(DOWNLOAD)
	curl -fL -o $(DOWNLOAD)/zig.tar.xz $(ZIG_URL)
	echo "$(ZIG_SHA256)  $(DOWNLOAD)/zig.tar.xz" | shasum -a 256 -c
	tar -xJf $(DOWNLOAD)/zig.tar.xz -C $(DOWNLOAD)
	rm -rf .toolchain/$(ZIG_NAME)
	mv $(DOWNLOAD)/$(ZIG_NAME) .toolchain/$(ZIG_NAME)
	rm -rf $(DOWNLOAD)

$(GHOSTTY):
	git submodule update --init vendor/ghostty
