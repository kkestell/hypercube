# The base image for Docker sessions: what an agent needs to work on a GitHub
# repo. Hypercube builds it when it is missing and tags it with a hash of this
# file and `extra-ca.crt`, so changing either makes the next Docker session
# build a new one.
FROM debian:trixie-slim

# Keep in step with the OpenCode on the host: credentials are exported from it and imported here.
ARG OPENCODE_VERSION=2.2.0

# procps is for pkill and pgrep, which stop what an agent or terminal started.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git gh procps ripgrep \
 && rm -rf /var/lib/apt/lists/*

# Hypercube puts the host's extra CA certificates here (the file `NODE_EXTRA_CA_CERTS` names, if the server has it set; empty otherwise),
# so a TLS-intercepting proxy on the host's network doesn't break HTTPS in the container.
COPY extra-ca.crt /usr/local/share/ca-certificates/extra-ca.crt
RUN update-ca-certificates
# OpenCode reads CA certificates from here and not from the system's store.
ENV NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt

RUN case "$(uname -m)" in aarch64) arch=arm64 ;; x86_64) arch=x64-baseline ;; esac \
 && curl -fsSL "https://opencode.ai/files/bin/${OPENCODE_VERSION}/opencode-linux-${arch}.tar.gz" | tar -xz -C /usr/local/bin opencode \
 && opencode --version

WORKDIR /workspace
