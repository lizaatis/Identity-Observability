#!/bin/bash
# Start the backend server with proper environment setup

# Set Go cache to a local directory to avoid permission issues
export GOCACHE="${PWD}/.go-cache"
mkdir -p "$GOCACHE"

# Load environment variables
if [ -f .env ]; then
    echo "Loading environment variables from .env..."
    export $(cat .env | grep -v '^#' | xargs)
    echo "✅ Environment variables loaded"
else
    echo "⚠️  .env file not found, using defaults"
fi

# Set default DATABASE_URL if not set
if [ -z "$DATABASE_URL" ]; then
    export DATABASE_URL="postgres://observability:observability_dev@localhost:5433/identity_observability?sslmode=disable"
    echo "Using default DATABASE_URL"
fi

# Navigate to backend directory
cd "$(dirname "$0")/../backend" || exit 1

echo "Starting backend server on port 8080..."
echo "DATABASE_URL: $DATABASE_URL"
echo ""

# Download dependencies and generate go.sum
echo "Downloading dependencies..."
go mod download
go mod tidy

# Start the server
go run .
