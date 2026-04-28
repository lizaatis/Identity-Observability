#!/bin/bash
# Script to check if GCP service account has required permissions

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if service account path is provided
if [ -z "$GCP_SERVICE_ACCOUNT_PATH" ] && [ -z "$1" ]; then
    echo -e "${RED}Error: GCP_SERVICE_ACCOUNT_PATH not set${NC}"
    echo "Usage: $0 [path-to-service-account.json]"
    echo "Or: export GCP_SERVICE_ACCOUNT_PATH=/path/to/file.json"
    exit 1
fi

SERVICE_ACCOUNT_PATH="${1:-$GCP_SERVICE_ACCOUNT_PATH}"
PROJECT_ID="${GCP_PROJECT_ID:-}"

if [ ! -f "$SERVICE_ACCOUNT_PATH" ]; then
    echo -e "${RED}Error: Service account file not found: $SERVICE_ACCOUNT_PATH${NC}"
    exit 1
fi

# Extract project ID and service account email from JSON
if command -v jq &> /dev/null; then
    PROJECT_ID="${PROJECT_ID:-$(jq -r '.project_id' "$SERVICE_ACCOUNT_PATH")}"
    SERVICE_ACCOUNT_EMAIL=$(jq -r '.client_email' "$SERVICE_ACCOUNT_PATH")
else
    echo -e "${YELLOW}Warning: jq not found. Please set GCP_PROJECT_ID manually.${NC}"
    if [ -z "$PROJECT_ID" ]; then
        echo "Please set: export GCP_PROJECT_ID=your-project-id"
        exit 1
    fi
    SERVICE_ACCOUNT_EMAIL=$(grep -o '"client_email": "[^"]*"' "$SERVICE_ACCOUNT_PATH" | cut -d'"' -f4)
fi

echo -e "${GREEN}Checking permissions for:${NC}"
echo "  Service Account: $SERVICE_ACCOUNT_EMAIL"
echo "  Project: $PROJECT_ID"
echo ""

# Set credentials
export GOOGLE_APPLICATION_CREDENTIALS="$SERVICE_ACCOUNT_PATH"

# Check if gcloud is installed
if ! command -v gcloud &> /dev/null; then
    echo -e "${RED}Error: gcloud CLI not found. Please install it first.${NC}"
    exit 1
fi

# Test Cloud Identity API
echo -e "${YELLOW}Testing Cloud Identity API...${NC}"
if gcloud identity groups list --project="$PROJECT_ID" 2>&1 | grep -q "PERMISSION_DENIED\|403"; then
    echo -e "${RED}✗ Cloud Identity API: Missing permissions${NC}"
    echo "  Required: roles/cloudidentity.groups.reader or roles/viewer"
else
    echo -e "${GREEN}✓ Cloud Identity API: Access granted${NC}"
fi

# Test IAM API
echo -e "${YELLOW}Testing IAM API...${NC}"
if gcloud iam roles list --project="$PROJECT_ID" 2>&1 | grep -q "PERMISSION_DENIED\|403"; then
    echo -e "${RED}✗ IAM API: Missing permissions${NC}"
    echo "  Required: roles/iam.securityReviewer or roles/viewer"
else
    echo -e "${GREEN}✓ IAM API: Access granted${NC}"
fi

# Test Resource Manager API
echo -e "${YELLOW}Testing Resource Manager API...${NC}"
if gcloud projects describe "$PROJECT_ID" 2>&1 | grep -q "PERMISSION_DENIED\|403"; then
    echo -e "${RED}✗ Resource Manager API: Missing permissions${NC}"
    echo "  Required: roles/viewer or roles/resourcemanager.projectViewer"
else
    echo -e "${GREEN}✓ Resource Manager API: Access granted${NC}"
fi

# List assigned roles
echo ""
echo -e "${YELLOW}Assigned roles for this service account:${NC}"
gcloud projects get-iam-policy "$PROJECT_ID" \
  --flatten="bindings[].members" \
  --filter="bindings.members:$SERVICE_ACCOUNT_EMAIL" \
  --format="table(bindings.role)" 2>/dev/null || echo "  Could not list roles (may need permissions)"

echo ""
echo -e "${GREEN}Permission check complete!${NC}"
echo ""
echo "If any tests failed, grant these roles:"
echo "  gcloud projects add-iam-policy-binding $PROJECT_ID \\"
echo "    --member=\"serviceAccount:$SERVICE_ACCOUNT_EMAIL\" \\"
echo "    --role=\"roles/viewer\""
echo ""
echo "  gcloud projects add-iam-policy-binding $PROJECT_ID \\"
echo "    --member=\"serviceAccount:$SERVICE_ACCOUNT_EMAIL\" \\"
echo "    --role=\"roles/iam.securityReviewer\""
