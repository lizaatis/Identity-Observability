# Mock connector

Loads the test dataset into the canonical schema and runs identity resolution. Idempotent: safe to run multiple times.

## Prerequisites

- Go 1.21+
- PostgreSQL with canonical schema applied (`migrations/001_canonical_schema.sql` and `002_effective_access.sql`)
- `DATABASE_URL` set (e.g. `postgres://observability:observability_dev@localhost:5433/identity_observability?sslmode=disable`)

## Run

```bash
cd connectors/mock
export DATABASE_URL="postgres://observability:observability_dev@localhost:5433/identity_observability?sslmode=disable"
go run .
```

Optional: `MOCK_DATA_PATH` to override the dataset file (default: `data/test_dataset.json`).

## Test dataset

`data/test_dataset.json` includes:

- **Users** in `okta_mock`, `entra_mock`, `aws_mock` (Alice, Bob, Carol; Alice and Bob have multiple source accounts for resolution).
- **Groups, roles, permissions** and **identity_group**, **identity_role**, **group_role**, **role_permission** so that effective access can be computed (including via-group paths).

After running, query effective access:

```sql
SELECT * FROM identity_effective_permissions WHERE identity_id = 1;
-- or use the materialized view:
SELECT * FROM identity_effective_permissions_mv WHERE identity_id = 1;
```

Lineage (path explainability):

```sql
SELECT identity_id, permission_id, ord, hop_type, hop_name, hop_detail
FROM identity_access_lineage
WHERE identity_id = 1 AND permission_id = 1
ORDER BY permission_id, ord;
```
