#!/bin/bash

set -e

echo "Starting services..."

# Function to start a service and verify it's running
start_service() {
    local service=$1
    local check_port=$2
    local max_attempts=30
    local wait_time=1

    echo "Starting $service..."
    service $service start

    echo "Waiting for $service to be ready..."
    for ((i=1; i<=max_attempts; i++)); do
        if nc -z localhost $check_port; then
            echo "$service is ready (port $check_port)"
            return 0
        fi
        echo "Waiting for $service (attempt $i/$max_attempts)..."
        sleep $wait_time
    done

    echo "Failed to start $service"
    return 1
}

# Start and verify each service
start_service ssh 22 || exit 1
start_service redis-server 6379 || exit 1
start_service nginx 80 || exit 1

# Start Flask app with proper logging
cd /app
python3 app.py > /var/log/flask.log 2>&1 &
FLASK_PID=$!

# Wait for Flask to start
for ((i=1; i<=30; i++)); do
    if nc -z localhost 8080; then
        echo "Flask app is ready"
        break
    fi
    if ! kill -0 $FLASK_PID 2>/dev/null; then
        echo "Flask app failed to start"
        cat /var/log/flask.log
        exit 1
    fi
    echo "Waiting for Flask app (attempt $i/30)..."
    sleep 1
done

echo "All services started successfully"
echo "Service status:"
service ssh status
service nginx status
service redis-server status
ps aux | grep python3

# Keep container running while monitoring services
while true; do
    sleep 10
    if ! nc -z localhost 80 || ! nc -z localhost 22 || ! nc -z localhost 6379 || ! nc -z localhost 8080; then
        echo "Service check failed"
        exit 1
    fi
done
