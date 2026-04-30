# Build stage
FROM golang:1.26-alpine3.23 AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments. COMMIT and BUILD_TIME are normally provided by CI; for local
# builds without --build-arg they fall back to git rev-parse and date below.
ARG VERSION=dev
ARG COMMIT=
ARG BUILD_TIME=

# Build the application.
RUN COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}" && \
    BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${COMMIT}" \
    -o audiologger .

# Runtime stage
FROM alpine:3.23

LABEL org.opencontainers.image.source="https://github.com/oszuidwest/zwfm-audiologger"
LABEL org.opencontainers.image.description="ZuidWest FM audiologger"

# Install runtime dependencies
RUN apk --no-cache upgrade && \
    apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata

# Create non-root user
RUN addgroup -g 1001 audiologger && \
    adduser -u 1001 -G audiologger -s /bin/sh -D audiologger

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/audiologger .

# Copy configuration template
COPY config.json .

# Create directories with proper permissions
RUN mkdir -p /var/audio /var/log && \
    chown -R audiologger:audiologger /var/audio /var/log /app

# Switch to non-root user
USER audiologger

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --timeout=3 -O /dev/null http://localhost:8080/health || exit 1

# Default command
CMD ["./audiologger"]