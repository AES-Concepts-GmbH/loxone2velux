# Frontend build stage
FROM node:20-alpine AS frontend

WORKDIR /web

# Copy package files
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund

# Copy source and build
COPY web/ .
RUN npm run build

# Backend build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install ca-certificates for TLS
RUN apk add --no-cache ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with version from VERSION file
RUN VERSION=$(cat VERSION 2>/dev/null || echo "dev") && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s -X main.version=${VERSION}" -o /loxone2velux ./cmd/gateway/

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for TLS connections to KLF-200
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /loxone2velux /app/loxone2velux

# Copy frontend from frontend builder
COPY --from=frontend /web/dist /app/web/dist

# Copy example config. Without a mounted /app/config.yaml the gateway starts
# with defaults and reads KLF200_HOST / KLF200_PASSWORD from the environment.
COPY config.example.yaml /app/config.example.yaml

# Create non-root user
RUN adduser -D -u 1000 appuser
USER appuser

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
ENTRYPOINT ["/app/loxone2velux"]
CMD ["-config", "/app/config.yaml"]
