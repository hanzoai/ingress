# syntax=docker/dockerfile:1.2

# ---- Stage 1: Build WebUI dashboard ----
FROM --platform=$BUILDPLATFORM node:24-alpine3.22 AS webui

WORKDIR /src/webui
COPY webui/package.json webui/yarn.lock webui/.yarnrc.yml ./
RUN corepack enable && yarn workspaces focus --all --production
COPY webui/ ./
RUN yarn build

# ---- Stage 2: Build Go binary ----
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

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

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
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

COPY --from=builder /hanzo-ingress /hanzo-ingress

EXPOSE 80
VOLUME ["/tmp"]

ENTRYPOINT ["/hanzo-ingress"]
