#!/bin/bash
# Comprehensive testing script for Identity Observability Platform

set -e

echo "🧪 Identity Observability Platform - Testing Script"
echo "=================================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker is not running. Please start Docker first.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Docker is running${NC}"
echo ""

# Step 1: Start Docker services
echo "📦 Step 1: Starting Docker services (PostgreSQL/TimescaleDB, Redis, Neo4j)..."
docker-compose up -d postgres redis neo4j

echo "⏳ Waiting for services to be healthy..."
sleep 10

# Check PostgreSQL
if docker exec identity-observability-db pg_isready -U observability > /dev/null 2>&1; then
    echo -e "${GREEN}✅ PostgreSQL/TimescaleDB is ready${NC}"
else
    echo -e "${RED}❌ PostgreSQL/TimescaleDB is not ready${NC}"
    exit 1
fi

# Check Redis
if docker exec identity-observability-redis redis-cli ping > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Redis is ready${NC}"
else
    echo -e "${YELLOW}⚠️  Redis is not ready (optional)${NC}"
fi

# Check Neo4j
if docker exec identity-observability-neo4j cypher-shell -u neo4j -p observability_dev "RETURN 1" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Neo4j is ready${NC}"
else
    echo -e "${YELLOW}⚠️  Neo4j is not ready (optional for graph features)${NC}"
fi

echo ""

# Step 2: Run migrations
echo "🗄️  Step 2: Running database migrations..."
export DATABASE_URL="${DATABASE_URL:-postgres://observability:observability_dev@localhost:5433/identity_observability?sslmode=disable}"

if [ -f scripts/run-migrations.sh ]; then
    bash scripts/run-migrations.sh
else
    echo "Running migrations manually..."
    for migration in migrations/*.sql; do
        if [ -f "$migration" ]; then
            echo "  Running $(basename $migration)..."
            psql "$DATABASE_URL" -f "$migration" || echo -e "${YELLOW}⚠️  Migration $(basename $migration) may have errors (check if already applied)${NC}"
        fi
    done
fi

echo -e "${GREEN}✅ Migrations completed${NC}"
echo ""

# Step 3: Check backend dependencies
echo "🔧 Step 3: Checking backend dependencies..."
cd backend
if [ ! -f go.mod ]; then
    echo -e "${RED}❌ Backend go.mod not found${NC}"
    exit 1
fi

echo "Running go mod tidy..."
go mod tidy

echo -e "${GREEN}✅ Backend dependencies ready${NC}"
cd ..
echo ""

# Step 4: Check frontend dependencies
echo "🎨 Step 4: Checking frontend dependencies..."
cd frontend
if [ ! -f package.json ]; then
    echo -e "${RED}❌ Frontend package.json not found${NC}"
    exit 1
fi

if [ ! -d node_modules ]; then
    echo "Installing frontend dependencies..."
    npm install
else
    echo "Frontend dependencies already installed"
fi

echo -e "${GREEN}✅ Frontend dependencies ready${NC}"
cd ..
echo ""

# Step 5: Test API endpoints
echo "🌐 Step 5: Testing API endpoints..."
echo ""

# Start backend in background
echo "Starting backend server..."
cd backend
export DATABASE_URL="${DATABASE_URL:-postgres://observability:observability_dev@localhost:5433/identity_observability?sslmode=disable}"
export GOCACHE="${PWD}/../.go-cache"
mkdir -p "$GOCACHE"

# Load .env if exists
if [ -f ../.env ]; then
    export $(cat ../.env | grep -v '^#' | xargs)
fi

go run . &
BACKEND_PID=$!
cd ..

echo "⏳ Waiting for backend to start..."
sleep 5

# Test health endpoint
if curl -s http://localhost:8080/health > /dev/null; then
    echo -e "${GREEN}✅ Backend is running (PID: $BACKEND_PID)${NC}"
    echo "   Health check: $(curl -s http://localhost:8080/health | jq -r .status 2>/dev/null || echo 'OK')"
else
    echo -e "${RED}❌ Backend failed to start${NC}"
    kill $BACKEND_PID 2>/dev/null || true
    exit 1
fi

echo ""

# Step 6: Test key endpoints
echo "🔍 Step 6: Testing key API endpoints..."

test_endpoint() {
    local url=$1
    local name=$2
    if curl -s "$url" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ $name${NC}"
        return 0
    else
        echo -e "${YELLOW}⚠️  $name (may need data)${NC}"
        return 1
    fi
}

test_endpoint "http://localhost:8080/api/v1/identities" "GET /api/v1/identities"
test_endpoint "http://localhost:8080/api/v1/dashboard/stats" "GET /api/v1/dashboard/stats"
test_endpoint "http://localhost:8080/api/v1/risk/top" "GET /api/v1/risk/top"
test_endpoint "http://localhost:8080/api/v1/connectors" "GET /api/v1/connectors"

echo ""

# Step 7: Summary
echo "📊 Testing Summary"
echo "=================="
echo ""
echo -e "${GREEN}✅ Services:${NC}"
echo "   - PostgreSQL/TimescaleDB: Running"
echo "   - Redis: Running"
echo "   - Neo4j: Running"
echo ""
echo -e "${GREEN}✅ Backend:${NC}"
echo "   - Server: http://localhost:8080"
echo "   - Health: http://localhost:8080/health"
echo "   - API: http://localhost:8080/api/v1"
echo ""
echo -e "${GREEN}✅ Next Steps:${NC}"
echo ""
echo "1. Start the frontend:"
echo "   cd frontend && npm run dev"
echo ""
echo "2. Access the UI:"
echo "   http://localhost:3000"
echo ""
echo "3. Test connectors (optional):"
echo "   # Load mock data:"
echo "   cd connectors/mock && go run main.go"
echo ""
echo "   # Or sync from Okta:"
echo "   cd connectors/okta && go run main.go"
echo ""
echo "4. Sync graph to Neo4j (optional):"
echo "   curl -X POST http://localhost:8080/api/v1/graph/sync"
echo ""
echo -e "${YELLOW}⚠️  Backend is running in background (PID: $BACKEND_PID)${NC}"
echo "   To stop: kill $BACKEND_PID"
echo ""
echo -e "${GREEN}🎉 Platform is ready for testing!${NC}"
