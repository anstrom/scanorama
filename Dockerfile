# Multi-stage build for Scanorama network scanner
# Stage 1: Build the Go application
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o scanorama \
    ./cmd/scanorama

# Stage 2: Create minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    nmap \
    nmap-scripts \
    curl \
    && rm -rf /var/cache/apk/*

# Create non-root user for security
RUN addgroup -g 1001 -S scanorama && \
    adduser -u 1001 -S scanorama -G scanorama

# Create directories
RUN mkdir -p /app/config /app/logs /app/static /app/data && \
    chown -R scanorama:scanorama /app

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/scanorama /app/scanorama

# Copy configuration files
COPY config.example.yaml /app/config/config.yaml
COPY --chown=scanorama:scanorama config.example.yaml /app/config/

# Create static directory for future frontend assets
RUN mkdir -p /app/static/css /app/static/js /app/static/images

# Switch to non-root user
USER scanorama

# Expose port for daemon API
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Environment variables
ENV SCANORAMA_CONFIG_FILE=/app/config/config.yaml
ENV SCANORAMA_LOG_LEVEL=info
ENV SCANORAMA_LOG_FORMAT=json

# Labels for metadata
LABEL maintainer="scanorama-team"
LABEL description="Scanorama network scanner and discovery tool"
LABEL version="${VERSION}"

# Default command - run daemon in foreground
CMD ["./scanorama", "daemon", "start", "--config", "/app/config/config.yaml", "--background=false", "--port", "8080"]
