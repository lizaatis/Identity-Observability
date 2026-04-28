#!/bin/bash
# Seed local sample data for dashboard/testing
# Usage: ./scripts/seed-sample-data.sh

set -e

if [ -f .env ]; then
  export $(rg -v '^#' .env | xargs)
fi

if [ -z "$DATABASE_URL" ]; then
  export DATABASE_URL="postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable"
fi

echo "Running mock connector against: $DATABASE_URL"
(cd connectors/mock && go run .)

echo "Triggering graph sync (optional but recommended for graph pages)..."
curl -s -X POST http://localhost:8080/api/v1/graph/sync >/dev/null || true

echo "Sample data seeded."
echo "Now refresh dashboard at http://localhost:5173/"
