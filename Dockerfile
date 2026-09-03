# syntax=docker/dockerfile:1

# ---- Stage 1: Build WebUI dashboard ----
FROM --platform=$BUILDPLATFORM node:24-alpine3.22 AS webui

RUN corepack enable && corepack prepare pnpm@9.15.0 --activate

WORKDIR /src/webui
COPY webui/package.json webui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY webui/ ./
RUN pnpm build

# ---- Stage 2: Build Go binary ----
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS builder
# go.mod pins the toolchain. The golang base image sets GOTOOLCHAIN=local,
# which turns a `go` directive newer than the image into a hard build
# failure instead of a download.
ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=v0.1.0

WORKDIR /src
# hanzoai/* is private, so it is fetched via authenticated git, bypassing the
# public proxy. gh_token is the shared docker-build.yml BuildKit secret; no-op
# when absent (local/dev).
#
# zap-proto/* is NOT private — the repos are public and proxy.golang.org serves
# them. Listing it here sent go to direct git for a module the proxy already
# had, and that path is the fragile one:
#
#   reading github.com/zap-proto/go/go.mod at revision v1.3.0: git ls-remote …
#   fatal: unable to access 'https://github.com/zap-proto/go/': SSL: certificate
#   subject name 'dotcom.glb' does not match target hostname 'github.com'
#
# GOPRIVATE names what is actually private. Anything public goes through the
# proxy, which is faster and checksum-verified.
ENV GOPRIVATE=github.com/hanzoai/*
# Copy go.mod, go.sum, and the local replace target first for layer caching.
COPY go.mod go.sum ./
COPY pkg/config/dynamic/ext/ ./pkg/config/dynamic/ext/
RUN --mount=type=secret,id=gh_token \
    if [ -s /run/secrets/gh_token ]; then \
      export GIT_CONFIG_COUNT=1 \
             GIT_CONFIG_KEY_0="url.https://x-access-token:$(cat /run/secrets/gh_token)@github.com/.insteadOf" \
             GIT_CONFIG_VALUE_0="https://github.com/"; \
    fi; \
    go mod download

# Copy full source.
COPY . .
# Overlay built webui assets.
COPY --from=webui /src/webui/static ./webui/static

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s \
      -X github.com/hanzoai/ingress/v3/pkg/version.Version=${VERSION} \
      -X github.com/hanzoai/ingress/v3/pkg/version.Codename=hanzo \
      -X github.com/hanzoai/ingress/v3/pkg/version.BuildDate=$(date -u '+%Y-%m-%d_%I:%M:%S%p')" \
    -o /hanzo-ingress ./cmd/ingress

# ---- Stage 3: Runtime ----

# What the scratch stage copies in place of adduser and mkdir, which it lacks.
RUN printf 'ingress:x:1000:1000::/:/sbin/nologin\n' > /etc/passwd.ingress && \
    printf 'ingress:x:1000:\n' > /etc/group.ingress && \
    mkdir -p /emptytmp && chmod 1777 /emptytmp

# THE IMAGE IS THE BINARY.
#
# hanzo-ingress is CGO_ENABLED=0 and statically linked, so it takes nothing from
# a host. Alpine was supplying three things and none of them survives inspection:
#
#   ca-certificates and tzdata are DATA the binary reads, and they copy.
#   The account is two files the kernel reads to name a uid it already enforces.
#   libcap-utils existed for ONE line, `setcap cap_net_bind_service=+ep`.
#
# THE FILE CAPABILITY IS REDUNDANT AND THAT IS MEASURED, not assumed. The live
# deployment runs with securityContext capabilities `add: [NET_BIND_SERVICE],
# drop: [ALL]` — the orchestrator grants the capability to the process, so a
# capability baked into the file grants nothing it does not already have. The
# image's own ports are unprivileged anyway (8080/8443, mapped by the Service),
# which is the arrangement the line below the EXPOSE has always described.
#
# The one posture this changes is `docker run` as a non-root user binding :80
# directly, with no --cap-add. That is not how this is deployed, and the remedy
# there is --cap-add=NET_BIND_SERVICE rather than a capability in the artifact.
FROM scratch

LABEL org.opencontainers.image.source="https://github.com/hanzoai/ingress"
LABEL org.opencontainers.image.title="Hanzo Ingress"
LABEL org.opencontainers.image.description="Cloud-native reverse proxy and load balancer for Hanzo infrastructure"

# Data, read by the binary, executed by nothing.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# The account. scratch has no adduser, so the two files it would have written are
# written in the builder and copied.
COPY --from=builder /etc/passwd.ingress /etc/passwd
COPY --from=builder /etc/group.ingress /etc/group

# /tmp, owned by the account. The VOLUME below replaces it at run time; this is
# what the image holds when nothing is mounted, and scratch has no mkdir.
COPY --from=builder --chown=1000:1000 /emptytmp /tmp

COPY --from=builder /hanzo-ingress /hanzo-ingress

# Bind to unprivileged ports; K8s Service/DaemonSet maps 80->8080, 443->8443
EXPOSE 8080 8443
VOLUME ["/tmp"]
USER 1000:1000
ENTRYPOINT ["/hanzo-ingress"]
