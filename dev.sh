#!/bin/bash
# Kill any running pr-review-server, rebuild, and run in dev mode

set -e

# Load .env
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Kill any existing process
pkill -f "pr-review-server" 2>/dev/null || true
sleep 0.5

# Build
echo "Building..."
go build -o pr-review-server .

# Run
echo "Starting on port ${SERVER_PORT:-8080}..."
echo "Logs: ./server.log"
./pr-review-server 2>&1 | tee -a server.log
