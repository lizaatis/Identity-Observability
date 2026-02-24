-- Identity Observability — Canonical schema (MVP)
-- Run against your PostgreSQL database (e.g. psql $DATABASE_URL -f migrations/001_canonical_schema.sql)

-- Extensions (optional; uncomment if you use UUID primary keys)
-- CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =============================================================================
-- Core entities
-- =============================================================================

-- Canonical person (one row per human after resolution)
CREATE TABLE identities (
  id         BIGSERIAL PRIMARY KEY,
  employee_id TEXT,
  email       TEXT NOT NULL,
  display_name TEXT,
  status      TEXT NOT NULL DEFAULT 'active',  -- effective status (active, inactive, etc.)
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_identities_employee_id ON identities (employee_id);
CREATE INDEX idx_identities_email ON identities (email);
CREATE INDEX idx_identities_status ON identities (status);

-- Link canonical identity to each IdP's user (for resolution + per-source status)
CREATE TABLE identity_sources (
  id                  BIGSERIAL PRIMARY KEY,
  identity_id         BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  source_system       TEXT NOT NULL,
  source_user_id      TEXT NOT NULL,
  source_status       TEXT NOT NULL DEFAULT 'active',  -- per-source (for disabled drift)
  confidence          DECIMAL(5,4) NOT NULL DEFAULT 1.0,
  synced_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (source_system, source_user_id)
);

CREATE INDEX idx_identity_sources_identity_id ON identity_sources (identity_id);
CREATE INDEX idx_identity_sources_source ON identity_sources (source_system, source_user_id);

-- Group in a source system
CREATE TABLE groups (
  id           BIGSERIAL PRIMARY KEY,
  name         TEXT NOT NULL,
  source_system TEXT NOT NULL,
  source_id    TEXT NOT NULL,
  synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (source_system, source_id)
);

CREATE INDEX idx_groups_source ON groups (source_system, source_id);

-- Role in a source system
CREATE TABLE roles (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  privilege_level TEXT NOT NULL DEFAULT 'read',  -- e.g. admin, read, custom
  source_system   TEXT NOT NULL,
  source_id       TEXT NOT NULL,
  synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (source_system, source_id)
);

CREATE INDEX idx_roles_source ON roles (source_system, source_id);
CREATE INDEX idx_roles_privilege_level ON roles (privilege_level);

-- Permission / entitlement in a source system
CREATE TABLE permissions (
  id            BIGSERIAL PRIMARY KEY,
  name          TEXT NOT NULL,
  resource_type TEXT,
  source_system TEXT NOT NULL,
  source_id     TEXT NOT NULL,
  synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (source_system, source_id)
);

CREATE INDEX idx_permissions_source ON permissions (source_system, source_id);

-- =============================================================================
-- Relationship tables (many-to-many, with source for lineage)
-- =============================================================================

CREATE TABLE identity_group (
  id           BIGSERIAL PRIMARY KEY,
  identity_id  BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  group_id     BIGINT NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
  source_system TEXT NOT NULL,
  source_id    TEXT,
  synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (identity_id, group_id, source_system)
);

CREATE INDEX idx_identity_group_identity ON identity_group (identity_id);
CREATE INDEX idx_identity_group_group ON identity_group (group_id);
CREATE INDEX idx_identity_group_source ON identity_group (source_system, source_id);

CREATE TABLE identity_role (
  id           BIGSERIAL PRIMARY KEY,
  identity_id  BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  role_id      BIGINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
  source_system TEXT NOT NULL,
  source_id    TEXT,
  synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (identity_id, role_id, source_system)
);

CREATE INDEX idx_identity_role_identity ON identity_role (identity_id);
CREATE INDEX idx_identity_role_role ON identity_role (role_id);
CREATE INDEX idx_identity_role_source ON identity_role (source_system, source_id);

CREATE TABLE group_role (
  id           BIGSERIAL PRIMARY KEY,
  group_id     BIGINT NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
  role_id      BIGINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
  source_system TEXT NOT NULL,
  source_id    TEXT,
  synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (group_id, role_id, source_system)
);

CREATE INDEX idx_group_role_group ON group_role (group_id);
CREATE INDEX idx_group_role_role ON group_role (role_id);
CREATE INDEX idx_group_role_source ON group_role (source_system, source_id);

CREATE TABLE role_permission (
  id            BIGSERIAL PRIMARY KEY,
  role_id       BIGINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
  permission_id BIGINT NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
  source_system TEXT NOT NULL,
  source_id     TEXT,
  synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (role_id, permission_id, source_system)
);

CREATE INDEX idx_role_permission_role ON role_permission (role_id);
CREATE INDEX idx_role_permission_permission ON role_permission (permission_id);
CREATE INDEX idx_role_permission_source ON role_permission (source_system, source_id);

-- =============================================================================
-- Optional: updated_at trigger (reuse for any table with updated_at)
-- =============================================================================

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply to tables that have updated_at (use EXECUTE PROCEDURE for PG 11–14; EXECUTE FUNCTION for PG 14+)
CREATE TRIGGER identities_updated_at
  BEFORE UPDATE ON identities FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
CREATE TRIGGER identity_sources_updated_at
  BEFORE UPDATE ON identity_sources FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
CREATE TRIGGER groups_updated_at
  BEFORE UPDATE ON groups FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
CREATE TRIGGER roles_updated_at
  BEFORE UPDATE ON roles FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
CREATE TRIGGER permissions_updated_at
  BEFORE UPDATE ON permissions FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
