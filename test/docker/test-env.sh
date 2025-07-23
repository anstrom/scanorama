#!/bin/bash

# Script to manage Docker test container for scanorama testing
# Provides test services:
# - HTTP (Nginx) on port 8080
# - HTTPS (Nginx) on port 8443
# - SSH on port 8022
# - Redis on port 8379
# - Flask app on port 8888

set -e

CONTAINER_NAME="scanorama-test"
IMAGE_NAME="scanorama-test"

usage() {
    echo "Usage: $0 <command>"
    echo "Commands:"
    echo "  build    - Build the test container image"
    echo "  start    - Start the test container"
    echo "  stop     - Stop the test container"
    echo "  status   - Check container status"
    echo "  logs     - Show container logs"
    echo "  test     - Test if services are running"
    echo "  clean    - Remove container and image"
}

build() {
    # Get script directory for correct context
    SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
    echo "Building test container image..."
    docker build -t "$IMAGE_NAME" "$SCRIPT_DIR"
}

start() {
    echo "Starting test container..."
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Container already running"
        return
    fi

    if [ "$(docker ps -aq -f status=exited -f name=$CONTAINER_NAME)" ]; then
        docker rm "$CONTAINER_NAME"
    fi

    docker run -d \
        --name "$CONTAINER_NAME" \
        -p 8080:80 \
        -p 8443:443 \
        -p 8022:22 \
        -p 8379:6379 \
        -p 8888:8080 \
        --health-cmd="bash -c 'nc -z localhost 80 && nc -z localhost 22 && nc -z localhost 6379 && nc -z localhost 8080'" \
        --health-interval=2s \
        --health-retries=15 \
        "$IMAGE_NAME"

    echo "Waiting for services to start..."
    echo "Waiting for container health check..."
    for i in {1..30}; do
        status=$(docker inspect -f '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null)
        if [ "$status" = "healthy" ]; then
            echo "Services are ready"
            test_services
            return
        elif [ "$status" = "unhealthy" ]; then
            echo "Services failed to start"
            docker logs "$CONTAINER_NAME"
            return 1
        fi
        echo "Waiting for services... ($i/30)"
        sleep 2
    done
    echo "Error: Timeout waiting for services"
    docker logs "$CONTAINER_NAME"
    return 1
}

stop() {
    echo "Stopping test container..."
    docker stop "$CONTAINER_NAME" 2>/dev/null || true
    docker rm "$CONTAINER_NAME" 2>/dev/null || true
}

status() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Container status:"
        docker ps -f name=$CONTAINER_NAME
        echo -e "\nPort mappings:"
        docker port "$CONTAINER_NAME"
    else
        echo "Container is not running"
        exit 1
    fi
}

logs() {
    docker logs "$CONTAINER_NAME"
}

test_services() {
    echo "Testing services..."
    local failed=0

    # Test HTTP with retry
    for i in {1..3}; do
        if curl -sf http://localhost:8080/health > /dev/null; then
            echo "✓ HTTP service is up"
            break
        elif [ $i -eq 3 ]; then
            echo "✗ HTTP service is down"
            failed=1
        else
            echo "Retrying HTTP service check..."
            sleep 1
        fi
    done

    # Test SSH with retry
    for i in {1..3}; do
        if nc -z localhost 8022 2>/dev/null; then
            echo "✓ SSH service is up"
            break
        elif [ $i -eq 3 ]; then
            echo "✗ SSH service is down"
            failed=1
        else
            echo "Retrying SSH service check..."
            sleep 1
        fi
    done

    # Test Redis with retry
    for i in {1..3}; do
        if nc -z localhost 8379 2>/dev/null; then
            echo "✓ Redis service is up"
            break
        elif [ $i -eq 3 ]; then
            echo "✗ Redis service is down"
            failed=1
        else
            echo "Retrying Redis service check..."
            sleep 1
        fi
    done

    # Test Flask app with retry
    for i in {1..3}; do
        if curl -sf http://localhost:8888/health > /dev/null; then
            echo "✓ Flask service is up"
            break
        elif [ $i -eq 3 ]; then
            echo "✗ Flask service is down"
            failed=1
        else
            echo "Retrying Flask app check..."
            sleep 1
        fi
    done

    return $failed
}

clean() {
    echo "Cleaning up..."
    stop
    docker rmi "$IMAGE_NAME" 2>/dev/null || true
}

# Main command processing
case "$1" in
    build)
        build
        ;;
    start)
        start
        ;;
    stop)
        stop
        ;;
    status)
        status
        ;;
    logs)
        logs
        ;;
    test)
        test_services
        ;;
    clean)
        clean
        ;;
    *)
        usage
        exit 1
        ;;
esac
