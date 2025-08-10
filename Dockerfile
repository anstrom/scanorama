FROM ubuntu:22.04

# Multi-platform build support
ARG TARGETPLATFORM

# Install system dependencies
RUN apt-get update && apt-get install -y \
    wget \
    curl \
    ca-certificates \
    git \
    nmap \
    && rm -rf /var/lib/apt/lists/*

# Install Go with multi-platform support
ENV GO_VERSION=1.21.5
RUN case "${TARGETPLATFORM}" in \
        "linux/amd64") GOARCH=amd64 ;; \
        "linux/arm64") GOARCH=arm64 ;; \
        *) echo "Unsupported platform: ${TARGETPLATFORM}" && exit 1 ;; \
    esac \
    && wget https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz \
    && tar -C /usr/local -xzf go${GO_VERSION}.linux-${GOARCH}.tar.gz \
    && rm go${GO_VERSION}.linux-${GOARCH}.tar.gz

# Set Go environment
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV GOBIN="/go/bin"

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o scanorama ./cmd/scanorama

# Create config directory
RUN mkdir -p /app/config

# Expose port
EXPOSE 8080

# Default command
CMD ["./scanorama", "daemon", "start", "--port", "8080"]
