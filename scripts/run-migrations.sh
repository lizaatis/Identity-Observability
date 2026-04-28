#!/bin/bash
# Helper script to run migrations
# Usage: ./scripts/run-migrations.sh [migration_file]
# Or: ./scripts/run-migrations.sh all

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if DATABASE_URL is set
if [ -z "$DATABASE_URL" ]; then
    echo -e "${YELLOW}DATABASE_URL not set. Using default Docker Compose connection...${NC}"
    export DATABASE_URL="postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable"
fi

# Function to run migration using Docker
run_with_docker() {
    local migration_file=$1
    echo -e "${GREEN}Running migration via Docker: $migration_file${NC}"
    docker exec -i identity-observability-db psql -U observability -d identity_observability < "$migration_file"
}

# Function to run migration using psql
run_with_psql() {
    local migration_file=$1
    echo -e "${GREEN}Running migration with psql: $migration_file${NC}"
    psql "$DATABASE_URL" -f "$migration_file"
}

# Check if Docker container is running
if docker ps --format '{{.Names}}' | grep -q '^identity-observability-db$'; then
    echo -e "${GREEN}PostgreSQL container is running. Using Docker method.${NC}"
    USE_DOCKER=true
elif command -v psql &> /dev/null; then
    echo -e "${GREEN}psql found. Using direct psql method.${NC}"
    USE_DOCKER=false
else
    echo -e "${RED}Error: Neither Docker container nor psql found.${NC}"
    echo ""
    echo "Options:"
    echo "1. Start Docker: docker compose up -d"
    echo "2. Install psql: brew install libpq && brew link --force libpq"
    exit 1
fi

# Get migration file(s)
if [ "$1" = "all" ] || [ -z "$1" ]; then
    # Run all migrations in order
    MIGRATIONS=(
        "migrations/001_canonical_schema.sql"
        "migrations/002_effective_access.sql"
        "migrations/003_resources_risk_and_sync.sql"
        "migrations/004_group_owner_column.sql"
        "migrations/005_deadend_rules.sql"
        "migrations/005_fix_orphaned_groups.sql"
        "migrations/006_realtime_events.sql"
        "migrations/007_action_automation.sql"
        "migrations/008_identity_sources_metadata.sql"
        "migrations/009_platform_graph_alerts_stitching.sql"
    )
    
    for migration in "${MIGRATIONS[@]}"; do
        if [ ! -f "$migration" ]; then
            echo -e "${RED}Error: Migration file not found: $migration${NC}"
            exit 1
        fi
        
        if [ "$USE_DOCKER" = true ]; then
            run_with_docker "$migration"
        else
            run_with_psql "$migration"
        fi
        echo ""
    done
    echo -e "${GREEN}All migrations completed!${NC}"
elif [ -f "$1" ]; then
    # Run specific migration file
    if [ "$USE_DOCKER" = true ]; then
        run_with_docker "$1"
    else
        run_with_psql "$1"
    fi
else
    echo -e "${RED}Error: Migration file not found: $1${NC}"
    exit 1
fi
