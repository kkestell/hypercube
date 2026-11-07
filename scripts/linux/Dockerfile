# The Linux test container on the base image BASE: systemd with a user manager
# for `tester`, Xvfb and the desktop services the desktop test observes, the
# Electron runtime libraries, and the Go and Bun toolchains. scripts/linux/run
# builds and starts it.
ARG BASE
FROM ${BASE}

ARG GO_VERSION
ARG BUN_VERSION
ARG PLAYWRIGHT_VERSION=1.65.0

COPY packages /tmp/packages
RUN /tmp/packages && rm /tmp/packages

RUN set -eu; \
	case "$(uname -m)" in x86_64) go_arch=amd64;; *) go_arch=arm64;; esac; \
	curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" | tar -xz -C /usr/local; \
	ln -s /usr/local/go/bin/go /usr/local/go/bin/gofmt /usr/local/bin/

RUN set -eu; \
	case "$(uname -m)" in x86_64) bun_arch=x64;; *) bun_arch=aarch64;; esac; \
	curl -fsSL -o /tmp/bun.zip "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-${bun_arch}.zip"; \
	unzip -j -q /tmp/bun.zip '*/bun' -d /usr/local/bin; \
	rm /tmp/bun.zip; \
	ln -s bun /usr/local/bin/bunx

RUN npm install -g --prefix /usr/local "playwright@${PLAYWRIGHT_VERSION}"

# scripts/linux/run uses /run/user/1000, so the test user is UID 1000; remove a
# user the base image already has at that UID. A container loads no kernel
# modules and needs no first-boot setup, so those units are masked.
RUN set -eu; \
	user=$(getent passwd 1000 | cut -d: -f1); \
	if [ -n "$user" ]; then userdel -r "$user"; fi; \
	useradd -m -u 1000 -s /bin/bash tester \
	&& mkdir -p /var/lib/systemd/linger /home/tester/.cache \
	&& touch /var/lib/systemd/linger/tester \
	&& chown tester:tester /home/tester/.cache \
	&& systemctl mask systemd-modules-load.service systemd-firstboot.service

# Chromium's sandbox does not work in a container.
ENV NODE_PATH=/usr/local/lib/node_modules \
	GOMODCACHE=/home/tester/.cache/go-mod \
	BUN_INSTALL_CACHE_DIR=/home/tester/.cache/bun \
	ELECTRON_DISABLE_SANDBOX=1

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
