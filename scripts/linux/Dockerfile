# The Linux test container: systemd with a user manager for `tester`, Xvfb, the
# Electron runtime libraries, and the Go and Bun toolchains. scripts/linux/run
# builds and starts it.
FROM debian:trixie

ARG GO_VERSION
ARG BUN_VERSION
ARG PLAYWRIGHT_VERSION=1.65.0

RUN apt-get update && apt-get install -y --no-install-recommends \
		systemd systemd-sysv dbus-user-session ca-certificates curl git unzip xz-utils \
		make gcc g++ libc6-dev xvfb xauth librsvg2-bin nodejs npm procps sqlite3 \
		libgtk-3-0t64 libnss3 libasound2t64 libgbm1 libxss1 desktop-file-utils \
	&& rm -rf /var/lib/apt/lists/*

RUN set -eu; \
	curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz" | tar -xz -C /usr/local; \
	ln -s /usr/local/go/bin/go /usr/local/go/bin/gofmt /usr/local/bin/

RUN set -eu; \
	curl -fsSL -o /tmp/bun.zip "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-$(case "$(uname -m)" in x86_64) echo x64;; *) echo aarch64;; esac).zip"; \
	unzip -j -q /tmp/bun.zip '*/bun' -d /usr/local/bin; \
	rm /tmp/bun.zip; \
	ln -s bun /usr/local/bin/bunx

RUN npm install -g "playwright@${PLAYWRIGHT_VERSION}"

RUN useradd -m -u 1000 -s /bin/bash tester \
	&& mkdir -p /var/lib/systemd/linger /home/tester/.cache \
	&& touch /var/lib/systemd/linger/tester \
	&& chown tester:tester /home/tester/.cache \
	&& systemctl mask systemd-modules-load.service

# Chromium's sandbox does not work in a container.
ENV NODE_PATH=/usr/local/lib/node_modules \
	GOMODCACHE=/home/tester/.cache/go-mod \
	BUN_INSTALL_CACHE_DIR=/home/tester/.cache/bun \
	ELECTRON_DISABLE_SANDBOX=1

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
