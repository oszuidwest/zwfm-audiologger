# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o bin/audiologger ./cmd/audiologger/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata \
    jq

# Create non-root user
RUN addgroup -g 1000 audiologger && \
    adduser -D -s /bin/sh -u 1000 -G audiologger audiologger

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/bin/audiologger .

# Create directories with proper permissions
RUN mkdir -p /var/audio /var/log && \
    chown -R audiologger:audiologger /var/audio /var/log /app

# Switch to non-root user
USER audiologger

# Expose HTTP port
EXPOSE 8080

# Default command
CMD ["./audiologger"]