# Global build args (must be before first FROM)
ARG BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.22
ARG BUILD_ARCH=amd64
ARG BUILD_VERSION=

# ============================================================
# Stage 1: Fetch source
# (HA builder sets context to add-on subdir, so we need to
#  clone the full repo to access Go/web source files)
#
# The add-on version (BUILD_VERSION, passed by the HA builder from
# config.yaml) selects the git tag "v<version>". If that tag does not
# exist the SOURCE_BRANCH is used, so an add-on build always matches
# a released version when one is tagged.
# ============================================================
FROM alpine:3.22 AS source

RUN apk add --no-cache git

# Use custom arg names to avoid HA Supervisor overriding them
ARG SOURCE_REPO=https://github.com/AES-Concepts-GmbH/loxone2velux.git
ARG SOURCE_BRANCH=main
ARG BUILD_VERSION

# Cache bust: COPY config.yaml from build context — changes with every
# version bump, which invalidates this layer and the git fetch below
COPY config.yaml /tmp/addon-config.yaml

RUN set -eu; \
    git clone --depth 1 --branch "${SOURCE_BRANCH}" "${SOURCE_REPO}" /src; \
    cd /src; \
    if [ -n "${BUILD_VERSION}" ] && git fetch --depth 1 origin "refs/tags/v${BUILD_VERSION}:refs/tags/v${BUILD_VERSION}" 2>/dev/null; then \
        git checkout -q "v${BUILD_VERSION}"; \
        echo "Building from tag v${BUILD_VERSION}"; \
    else \
        echo "WARNING: tag v${BUILD_VERSION} not found, building from branch ${SOURCE_BRANCH}"; \
    fi; \
    git log -1 --format='%H %s'

# ============================================================
# Stage 2: Build frontend
# ============================================================
# The dashboard is architecture independent. Build on the native builder so
# Rolldown does not need an ARMv7 musl binary on 32-bit add-on targets.
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend

WORKDIR /web
COPY --from=source /src/web/package.json /src/web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY --from=source /src/web/ .
COPY --from=source /src/VERSION /tmp/VERSION
RUN VITE_APP_VERSION=$(cat /tmp/VERSION) npm run build

# ============================================================
# Stage 3: Build Go binary
# ============================================================
FROM golang:1.26.10-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git

COPY --from=source /src/go.mod /src/go.sum ./
RUN go mod download

COPY --from=source /src/ .

# Re-declare to use within this stage
ARG BUILD_ARCH
RUN case "${BUILD_ARCH}" in \
        "amd64")   export GOARCH=amd64 ;; \
        "aarch64") export GOARCH=arm64 ;; \
        "armv7")   export GOARCH=arm && export GOARM=7 ;; \
        *)         export GOARCH=amd64 ;; \
    esac && \
    VERSION=$(cat VERSION 2>/dev/null || echo "dev") && \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-w -s -X main.version=${VERSION}" -o /loxone2velux ./cmd/gateway/

# ============================================================
# Stage 4: Runtime on Home Assistant base image
# (init: false in config.yaml disables s6-overlay)
# ============================================================
FROM ${BUILD_FROM}

RUN apk add --no-cache ca-certificates tzdata jq

# Copy binary
COPY --from=builder /loxone2velux /usr/bin/loxone2velux
RUN chmod +x /usr/bin/loxone2velux

# Copy frontend
COPY --from=frontend /web/dist /app/web/dist

# Copy startup script and fix potential Windows CRLF line endings
COPY run.sh /
RUN sed -i 's/\r$//' /run.sh && chmod a+x /run.sh

# Prepare persistent config directory
RUN mkdir -p /data /config/loxone2velux

EXPOSE 8099

# Override base image's /init entrypoint (s6-overlay) since init: false
ENTRYPOINT []
CMD ["/run.sh"]
