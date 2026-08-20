# Build stage
FROM golang:1.27.0-alpine3.24 AS builder

# Install build dependencies
RUN apk add --no-cache ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments. CI provides release metadata explicitly; deterministic
# fallbacks keep local builds reproducible when no build arguments are supplied.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the application.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}" \
    -o audiologger .

# Runtime stage
FROM alpine:3.24

LABEL org.opencontainers.image.source="https://github.com/oszuidwest/zwfm-audiologger"
LABEL org.opencontainers.image.description="ZuidWest FM audiologger"

# Install runtime dependencies
RUN apk --no-cache upgrade && \
    apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata

# Create the non-root user and its writable data directories.
RUN addgroup -g 1001 audiologger && \
    adduser -u 1001 -G audiologger -s /sbin/nologin -D audiologger && \
    install -d -o audiologger -g audiologger -m 0755 /var/audio /var/log

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder --chown=0:0 --chmod=0555 /app/audiologger .

# Copy configuration template
COPY --chown=0:0 --chmod=0444 config.json .

# Switch to non-root user
USER 1001:1001

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["wget", "-q", "-T", "3", "-O", "/dev/null", "http://127.0.0.1:8080/health"]

# Default command
CMD ["./audiologger"]
