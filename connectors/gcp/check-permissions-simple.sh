#!/bin/bash
# Simple permission check that doesn't require gcloud or jq

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
    exit 1
fi

SERVICE_ACCOUNT_PATH="${1:-$GCP_SERVICE_ACCOUNT_PATH}"

if [ ! -f "$SERVICE_ACCOUNT_PATH" ]; then
    echo -e "${RED}Error: Service account file not found: $SERVICE_ACCOUNT_PATH${NC}"
    exit 1
fi

# Extract project ID and service account email using grep (no jq needed)
PROJECT_ID=$(grep -o '"project_id": "[^"]*"' "$SERVICE_ACCOUNT_PATH" | cut -d'"' -f4)
SERVICE_ACCOUNT_EMAIL=$(grep -o '"client_email": "[^"]*"' "$SERVICE_ACCOUNT_PATH" | cut -d'"' -f4)

if [ -z "$PROJECT_ID" ]; then
    echo -e "${RED}Error: Could not extract project_id from JSON${NC}"
    exit 1
fi

if [ -z "$SERVICE_ACCOUNT_EMAIL" ]; then
    echo -e "${RED}Error: Could not extract client_email from JSON${NC}"
    exit 1
fi

echo -e "${GREEN}Service Account Information:${NC}"
echo "  Project ID: $PROJECT_ID"
echo "  Service Account: $SERVICE_ACCOUNT_EMAIL"
echo ""

echo -e "${YELLOW}To grant required permissions:${NC}"
echo ""
echo "Option 1: Via GCP Console (Recommended if gcloud not installed)"
echo "  1. Go to: https://console.cloud.google.com/iam-admin/iam?project=$PROJECT_ID"
echo "  2. Find service account: $SERVICE_ACCOUNT_EMAIL"
echo "  3. Click 'Edit' (pencil icon)"
echo "  4. Add these roles:"
echo "     - Viewer (roles/viewer)"
echo "     - Security Reviewer (roles/iam.securityReviewer)"
echo "     - Cloud Identity Groups Reader (roles/cloudidentity.groups.reader)"
echo ""
echo "Option 2: Install gcloud CLI"
echo "  macOS: brew install google-cloud-sdk"
echo "  Then run:"
echo "    gcloud projects add-iam-policy-binding $PROJECT_ID \\"
echo "      --member=\"serviceAccount:$SERVICE_ACCOUNT_EMAIL\" \\"
echo "      --role=\"roles/viewer\""
echo ""
echo "Option 3: Test connector anyway"
echo "  The connector will attempt to sync and report permission errors."
echo "  Run: cd connectors/gcp/cmd && go run ."
echo ""
echo -e "${GREEN}Required APIs to enable:${NC}"
echo "  Go to: https://console.cloud.google.com/apis/library?project=$PROJECT_ID"
echo "  Enable these APIs:"
echo "    - Cloud Identity API"
echo "    - Identity and Access Management (IAM) API"
echo "    - Cloud Resource Manager API"
echo ""
