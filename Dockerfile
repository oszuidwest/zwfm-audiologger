# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_TIME}" \
    -o audiologger .

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata \
    jq \
    curl \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1001 audiologger && \
    adduser -u 1001 -G audiologger -s /bin/sh -D audiologger

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/audiologger .

# Copy configuration template
COPY streams.json .

# Create directories with proper permissions
RUN mkdir -p /var/audio /var/log && \
    chown -R audiologger:audiologger /var/audio /var/log /app

# Switch to non-root user
USER audiologger

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Default command
CMD ["./audiologger"]