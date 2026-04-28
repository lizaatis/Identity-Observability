#!/bin/bash
# Setup and run GCP connector

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Setting up GCP connector...${NC}"

# Step 1: Go to gcp directory and download dependencies
cd "$(dirname "$0")"
echo -e "${YELLOW}Step 1: Downloading dependencies...${NC}"
go mod tidy
go mod download

echo -e "${GREEN}✓ Dependencies downloaded${NC}"

# Step 2: Check if environment variables are set
if [ -z "$DATABASE_URL" ]; then
    echo -e "${YELLOW}Warning: DATABASE_URL not set${NC}"
    echo "Set it with: export DATABASE_URL='postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable'"
fi

if [ -z "$GCP_SERVICE_ACCOUNT_PATH" ]; then
    echo -e "${YELLOW}Warning: GCP_SERVICE_ACCOUNT_PATH not set${NC}"
    echo "Set it with: export GCP_SERVICE_ACCOUNT_PATH='/path/to/service-account.json'"
fi

if [ -z "$GCP_PROJECT_ID" ]; then
    echo -e "${YELLOW}Warning: GCP_PROJECT_ID not set${NC}"
    echo "Set it with: export GCP_PROJECT_ID='your-project-id'"
fi

# Step 3: Run from cmd directory
echo -e "${GREEN}Step 2: Running connector from cmd/ directory...${NC}"
cd cmd
go run .
