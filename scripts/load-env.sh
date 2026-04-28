#!/bin/bash
# Helper script to load environment variables from .env file
# Usage: source scripts/load-env.sh
# Or: . scripts/load-env.sh

if [ -f .env ]; then
    echo "Loading environment variables from .env..."
    export $(cat .env | grep -v '^#' | xargs)
    echo "✅ Environment variables loaded"
else
    echo "⚠️  .env file not found"
    echo ""
    echo "Create a .env file with your credentials:"
    echo ""
    cat << 'EOF'
# Okta Connector Credentials
OKTA_DOMAIN=integrator-7793999.okta.com
OKTA_API_TOKEN=002M4nemUJtUuZRN-1gZPWBlwjxXT2k3FJ1KIkoxGh
OKTA_SOURCE_SYSTEM=okta_prod
OKTA_CONNECTOR_NAME=okta_connector

# Database Connection
DATABASE_URL=postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable
EOF
    echo ""
    echo "Then run: source scripts/load-env.sh"
fi
