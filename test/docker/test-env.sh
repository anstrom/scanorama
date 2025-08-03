#!/bin/bash

set -e

CONTAINER_NAME="scanorama-test"
IMAGE_NAME="scanorama-test"

usage() {
    echo "Usage: $0 <command>"
    echo "Commands:"
    echo "  build  - Build test container"
    echo "  start  - Start test services"
    echo "  stop   - Stop test services"
    echo "  clean  - Remove container and image"
}

build() {
    echo "Building test image..."
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    docker build -t "$IMAGE_NAME" "$SCRIPT_DIR"
}

start() {
    echo "Starting test services..."

    # Remove existing container if present
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

    # Start new container
    docker run -d \
        --name "$CONTAINER_NAME" \
        -p 8080:80 \
        -p 8022:22 \
        -p 8379:6379 \
        -p 8888:8888 \
        "$IMAGE_NAME"

    # Wait for services to be ready
    echo "Waiting for services..."
    for i in {1..30}; do
        if docker exec "$CONTAINER_NAME" bash -c 'nc -z localhost 80 && nc -z localhost 22 && nc -z localhost 6379 && nc -z localhost 8888' 2>/dev/null; then
            echo "Services ready"
            return 0
        fi
        sleep 1
    done

    echo "Services failed to start"
    docker logs "$CONTAINER_NAME"
    return 1
}

stop() {
    echo "Stopping test services..."
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
}

clean() {
    echo "Cleaning up..."
    stop
    docker rmi "$IMAGE_NAME" 2>/dev/null || true
}

case "$1" in
    build) build ;;
    start) start ;;
    stop) stop ;;
    clean) clean ;;
    *) usage; exit 1 ;;
esac
