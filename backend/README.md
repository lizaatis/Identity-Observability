# Backend API

Go + Gin API for Identity Observability. Serves identity profile with effective permissions and access lineage.

## Prerequisites

- Go 1.21+
- PostgreSQL with canonical schema and effective-access views applied
- Mock connector run at least once (so `identities` and related tables have data)

## Setup

```bash
cd backend
export DATABASE_URL="postgres://observability:observability_dev@localhost:5433/identity_observability?sslmode=disable"
go mod tidy
go run .
```

Server listens on `:8080` by default. Override with `PORT`.

## Endpoints

### GET /api/v1/identities/:id

Returns a single identity with effective permissions and lineage.

**Response:**

- `identity` – canonical identity (id, employee_id, email, display_name, status, created_at, updated_at)
- `sources` – linked IdP accounts (source_system, source_user_id, source_status, synced_at)
- `effective_permissions` – all permissions the identity has (direct role + via group), with path_type, role, group
- `lineage` – for each permission, the path (ordered hops: identity → group? → role → permission) for explainability

**Example:**

```bash
curl -s http://localhost:8080/api/v1/identities/1 | jq .
```

**Status codes:** 200 OK, 400 Bad Request (invalid id), 404 Not Found (no such identity), 500 on DB errors.
