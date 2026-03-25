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
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

RUN apk add --no-cache git

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=v0.1.0

WORKDIR /src
# Copy go.mod, go.sum, and the local replace target first for layer caching.
COPY go.mod go.sum ./
COPY pkg/config/dynamic/ext/ ./pkg/config/dynamic/ext/
RUN go mod download

# Copy full source.
COPY . .
# Overlay built webui assets.
COPY --from=webui /src/webui/static ./webui/static

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s \
      -X github.com/hanzoai/ingress/v3/pkg/version.Version=${VERSION} \
      -X github.com/hanzoai/ingress/v3/pkg/version.Codename=hanzo \
      -X github.com/hanzoai/ingress/v3/pkg/version.BuildDate=$(date -u '+%Y-%m-%d_%I:%M:%S%p')" \
    -o /hanzo-ingress ./cmd/traefik

# ---- Stage 3: Runtime ----
FROM alpine:3.23

RUN apk add --no-cache --no-progress ca-certificates tzdata

LABEL org.opencontainers.image.source="https://github.com/hanzoai/ingress"
LABEL org.opencontainers.image.title="Hanzo Ingress"
LABEL org.opencontainers.image.description="Cloud-native reverse proxy and load balancer for Hanzo infrastructure"

RUN addgroup -g 1000 ingress && adduser -u 1000 -G ingress -s /sbin/nologin -D ingress

COPY --from=builder /hanzo-ingress /hanzo-ingress

# Bind to unprivileged ports; K8s Service/DaemonSet maps 80->8080, 443->8443
EXPOSE 8080 8443
VOLUME ["/tmp"]

USER ingress
ENTRYPOINT ["/hanzo-ingress"]
