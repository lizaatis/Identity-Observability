#!/bin/bash
# Script to create .env file with your Okta credentials
# Run this from the project root: ./scripts/setup-env.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

if [ -f .env ]; then
    echo "⚠️  .env file already exists!"
    echo ""
    read -p "Do you want to overwrite it? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cancelled. Existing .env file preserved."
        exit 0
    fi
fi

cat > .env << 'EOF'
# Okta Connector Credentials
OKTA_DOMAIN=integrator-7793999.okta.com
OKTA_API_TOKEN=002M4nemUJtUuZRN-1gZPWBlwjxXT2k3FJ1KIkoxGh
OKTA_SOURCE_SYSTEM=okta_prod
OKTA_CONNECTOR_NAME=okta_connector

# Database Connection (matches docker-compose.yml defaults)
DATABASE_URL=postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable

# PostgreSQL (for docker-compose.yml)
POSTGRES_USER=observability
POSTGRES_PASSWORD=observability_dev
POSTGRES_DB=identity_observability
POSTGRES_PORT=5434

# Redis (for docker-compose.yml)
REDIS_PORT=6379
EOF

echo "✅ Created .env file with your Okta credentials"
echo ""
echo "📝 File location: $PROJECT_ROOT/.env"
echo ""
echo "Next steps:"
echo "1. Load environment: source scripts/load-env.sh"
echo "2. Start database: docker-compose up -d postgres"
echo "3. Run migrations: ./scripts/run-migrations.sh"
echo "4. Run connector: cd connectors/okta/cmd && go run ."
echo ""
echo "⚠️  Remember: .env is in .gitignore and won't be committed to git."
