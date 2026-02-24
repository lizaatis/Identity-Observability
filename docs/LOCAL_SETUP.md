# Local Setup: PostgreSQL & Redis

Ways to provision PostgreSQL 15+ and Redis for development.

---

## Option 1: Docker Compose (recommended)

**Prerequisite:** [Docker](https://docs.docker.com/get-docker/) and Docker Compose (v2+).

1. **Optional — set env vars**  
   Copy `.env.example` to `.env` and change passwords for local dev if you want:
   ```bash
   cp .env.example .env
   ```

2. **Start PostgreSQL and Redis**
   ```bash
   docker compose up -d
   ```

3. **Check they’re running**
   ```bash
   docker compose ps
   ```
   Both services should be “Up” and healthy.

4. **Connection details (defaults)**

   | Service   | Host     | Port | User        | Password / URL              |
   |-----------|----------|------|-------------|-----------------------------|
   | PostgreSQL | localhost | 5432 | observability | observability_dev           |
   | Redis    | localhost | 6379 | —           | `redis://localhost:6379/0`  |

   **PostgreSQL URL:**
   ```text
   postgres://observability:observability_dev@localhost:5432/identity_observability?sslmode=disable
   ```

   **Redis URL:**
   ```text
   redis://localhost:6379/0
   ```

5. **Stop when done**
   ```bash
   docker compose down
   ```
   Data is kept in Docker volumes. To remove data too: `docker compose down -v`.

---

## Option 2: Install on your machine

### PostgreSQL 15+

- **macOS (Homebrew):**
  ```bash
  brew install postgresql@16
  brew services start postgresql@16
  createdb identity_observability
  ```
- **Windows:** [PostgreSQL installer](https://www.postgresql.org/download/windows/).
- **Linux:** Use your distro’s package manager (e.g. `apt install postgresql-16`).

Create a user and database for the app, then set `DATABASE_URL` in your app config.

### Redis

- **macOS (Homebrew):**
  ```bash
  brew install redis
  brew services start redis
  ```
- **Windows:** [Redis for Windows](https://github.com/microsoftarchive/redis/releases) or WSL.
- **Linux:** `sudo apt install redis-server` (or equivalent).

Use `REDIS_URL=redis://localhost:6379/0` in your app.

---

## Option 3: Managed / cloud

For shared or production-like environments:

- **PostgreSQL:** AWS RDS, Azure Database for PostgreSQL, Google Cloud SQL, or Supabase/Neon.
- **Redis:** AWS ElastiCache, Azure Cache for Redis, Redis Cloud, or Upstash.

Create the instance in the provider’s console, then set `DATABASE_URL` and `REDIS_URL` in your environment (e.g. CI, staging, production). Never commit those URLs if they contain secrets.

---

## Using the DB in your app

1. Set env (or `.env`):
   - `DATABASE_URL=postgres://observability:observability_dev@localhost:5432/identity_observability?sslmode=disable`
   - `REDIS_URL=redis://localhost:6379/0`

2. Run migrations (once you have them) against this DB, e.g.:
   ```bash
   psql $DATABASE_URL -f migrations/001_canonical_schema.sql
   ```
   or use your Go migration tool.

3. Point your backend (Go) at `DATABASE_URL` and `REDIS_URL`; no code changes needed when you switch from Docker to managed later if you keep using env vars.
