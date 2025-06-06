#!/bin/bash
# build.sh - Build script for the log analyzer

set -e

echo "Building Log Analyzer Docker image..."

# Build the Docker image
docker build -t loganalyzer:latest .

echo "Build completed successfully!"

# Optional: Build with specific version tag
if [ "$1" != "" ]; then
    echo "Tagging with version: $1"
    docker tag loganalyzer:latest loganalyzer:$1
fi

echo "Available images:"
docker images | grep loganalyzer

---

#!/bin/bash
# run.sh - Run script for the log analyzer

set -e

# Default values
LOG_FILE=""
COMMAND=""
MOUNT_PATH=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--file)
            LOG_FILE="$2"
            shift 2
            ;;
        -m|--mount)
            MOUNT_PATH="$2"
            shift 2
            ;;
        *)
            COMMAND="$COMMAND $1"
            shift
            ;;
    esac
done

# Set default mount path if not provided
if [ -z "$MOUNT_PATH" ]; then
    MOUNT_PATH=$(pwd)/logs
fi

# Create logs directory if it doesn't exist
mkdir -p "$MOUNT_PATH"

echo "Running Log Analyzer..."
echo "Mount path: $MOUNT_PATH"
echo "Command: $COMMAND"

# Run the container
docker run --rm -it \
    -v "$MOUNT_PATH:/logs:ro" \
    -v "$(pwd)/output:/output" \
    loganalyzer:latest \
    $COMMAND

---

#!/bin/bash
# analyze.sh - Quick analysis script

set -e

if [ $# -eq 0 ]; then
    echo "Usage: $0 <log-file> [options]"
    echo "Example: $0 /var/log/nginx/access.log -stats"
    exit 1
fi

LOG_FILE="$1"
shift

# Get directory and filename
LOG_DIR=$(dirname "$LOG_FILE")
LOG_NAME=$(basename "$LOG_FILE")

echo "Analyzing log file: $LOG_FILE"

# Run analysis
docker run --rm -it \
    -v "$LOG_DIR:/logs:ro" \
    -v "$(pwd)/output:/output" \
    loganalyzer:latest \
    -f "/logs/$LOG_NAME" \
    "$@"

---

#!/bin/bash
# follow.sh - Follow log file in real-time

set -e

if [ $# -eq 0 ]; then
    echo "Usage: $0 <log-file> [options]"
    echo "Example: $0 /var/log/app.log -level ERROR"
    exit 1
fi

LOG_FILE="$1"
shift

# Get directory and filename
LOG_DIR=$(dirname "$LOG_FILE")
LOG_NAME=$(basename "$LOG_FILE")

echo "Following log file: $LOG_FILE"
echo "Press Ctrl+C to stop..."

# Follow log file
docker run --rm -it \
    -v "$LOG_DIR:/logs:ro" \
    loganalyzer:latest \
    -f "/logs/$LOG_NAME" \
    -follow \
    "$@"

---

#!/bin/bash
# docker-cleanup.sh - Cleanup script

set -e

echo "Cleaning up Docker resources..."

# Stop and remove containers
echo "Stopping containers..."
docker-compose down 2>/dev/null || true

# Remove unused images
echo "Removing unused images..."
docker image prune -f

# Remove unused volumes
echo "Removing unused volumes..."
docker volume prune -f

# Remove unused networks
echo "Removing unused networks..."
docker network prune -f

echo "Cleanup completed!"

---

# Makefile for easy Docker operations
.PHONY: build run clean test

# Build the Docker image
build:
	docker build -t loganalyzer:latest .

# Run with default settings
run:
	docker run --rm -it -v $(PWD)/logs:/logs:ro loganalyzer:latest --help

# Run with specific log file
run-file:
	@if [ -z "$(FILE)" ]; then echo "Usage: make run-file FILE=/path/to/logfile.log"; exit 1; fi
	docker run --rm -it -v $(dir $(FILE)):/logs:ro loganalyzer:latest -f /logs/$(notdir $(FILE)) $(ARGS)

# Run with Docker Compose
compose-up:
	docker-compose up -d

# Stop Docker Compose
compose-down:
	docker-compose down

# Follow logs in real-time
follow:
	@if [ -z "$(FILE)" ]; then echo "Usage: make follow FILE=/path/to/logfile.log"; exit 1; fi
	docker run --rm -it -v $(dir $(FILE)):/logs:ro loganalyzer:latest -f /logs/$(notdir $(FILE)) -follow

# Show statistics
stats:
	@if [ -z "$(FILE)" ]; then echo "Usage: make stats FILE=/path/to/logfile.log"; exit 1; fi
	docker run --rm -it -v $(dir $(FILE)):/logs:ro loganalyzer:latest -f /logs/$(notdir $(FILE)) -stats

# Clean up Docker resources
clean:
	docker image prune -f
	docker container prune -f
	docker volume prune -f

# Test with sample logs
test:
	mkdir -p logs
	echo "2024-01-01 10:00:00 [INFO] Application started" > logs/test.log
	echo "2024-01-01 10:01:00 [ERROR] Database connection failed" >> logs/test.log
	echo "2024-01-01 10:02:00 [WARN] High memory usage" >> logs/test.log
	docker run --rm -v $(PWD)/logs:/logs:ro loganalyzer:latest -f /logs/test.log -stats

# Push to registry (customize as needed)
push:
	docker tag loganalyzer:latest your-registry/loganalyzer:latest
	docker push your-registry/loganalyzer:latest