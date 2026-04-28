-- 003_resources_risk_and_sync.sql
-- Adds resources, risk tables, effective permission snapshots, and sync run tracking.

-- =============================================================================
-- Resources
-- =============================================================================

CREATE TABLE resources (
  id             BIGSERIAL PRIMARY KEY,
  name           TEXT NOT NULL,
  resource_type  TEXT NOT NULL,
  source_system  TEXT NOT NULL,
  source_id      TEXT NOT NULL,
  synced_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (source_system, source_id)
);

CREATE INDEX idx_resources_source ON resources (source_system, source_id);
CREATE INDEX idx_resources_type   ON resources (resource_type);

-- Reuse set_updated_at() from 001_canonical_schema.sql
CREATE TRIGGER resources_updated_at
  BEFORE UPDATE ON resources
  FOR EACH ROW EXECUTE PROCEDURE set_updated_at();

-- Optional: link permissions to resources (nullable for backward compatibility)
ALTER TABLE permissions
  ADD COLUMN resource_id BIGINT REFERENCES resources (id) ON DELETE SET NULL;

CREATE INDEX idx_permissions_resource_id ON permissions (resource_id);

-- =============================================================================
-- Risk scoring tables
-- =============================================================================

-- Latest risk score per identity
CREATE TABLE risk_scores (
  id           BIGSERIAL PRIMARY KEY,
  identity_id  BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  score        INTEGER NOT NULL CHECK (score >= 0 AND score <= 100),
  max_severity TEXT   NOT NULL,          -- e.g. 'low','medium','high','critical'
  computed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  detail       JSONB,                    -- optional per-rule breakdown
  UNIQUE (identity_id)
);

CREATE INDEX idx_risk_scores_score       ON risk_scores (score DESC);
CREATE INDEX idx_risk_scores_computed_at ON risk_scores (computed_at DESC);

-- Historical risk scores (time series)
CREATE TABLE risk_score_history (
  id           BIGSERIAL PRIMARY KEY,
  identity_id  BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  score        INTEGER NOT NULL CHECK (score >= 0 AND score <= 100),
  max_severity TEXT   NOT NULL,
  computed_at  TIMESTAMPTZ NOT NULL,
  detail       JSONB
);

CREATE INDEX idx_risk_score_history_identity_time
  ON risk_score_history (identity_id, computed_at DESC);

-- Per-identity flags for rules that fired (including "deadend")
CREATE TABLE risk_flags (
  id           BIGSERIAL PRIMARY KEY,
  identity_id  BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  rule_key     TEXT   NOT NULL,          -- e.g. 'deadend_orphaned_user', 'stale_group'
  severity     TEXT   NOT NULL,          -- e.g. 'low','medium','high','critical'
  is_deadend   BOOLEAN NOT NULL DEFAULT FALSE,
  message      TEXT,
  metadata     JSONB,                    -- arbitrary structured context
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cleared_at   TIMESTAMPTZ
);

CREATE INDEX idx_risk_flags_identity      ON risk_flags (identity_id);
CREATE INDEX idx_risk_flags_rule          ON risk_flags (rule_key);
CREATE INDEX idx_risk_flags_active_open   ON risk_flags (identity_id)
  WHERE cleared_at IS NULL;

-- =============================================================================
-- Sync runs
-- =============================================================================

CREATE TABLE sync_runs (
  id             BIGSERIAL PRIMARY KEY,
  source_system  TEXT NOT NULL,           -- e.g. 'okta_mock', 'entra', 'aws'
  connector_name TEXT NOT NULL,           -- e.g. 'mock_connector', 'okta_sync'
  started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at    TIMESTAMPTZ,
  status         TEXT NOT NULL,           -- 'running','success','error','partial'
  error_count    INTEGER NOT NULL DEFAULT 0,
  warning_count  INTEGER NOT NULL DEFAULT 0,
  last_error     TEXT,
  metadata       JSONB
);

CREATE INDEX idx_sync_runs_source_time
  ON sync_runs (source_system, started_at DESC);

CREATE INDEX idx_sync_runs_status
  ON sync_runs (status);

-- =============================================================================
-- Effective permission snapshots
-- =============================================================================

CREATE TABLE effective_permission_snapshots (
  id            BIGSERIAL PRIMARY KEY,
  sync_run_id   BIGINT NOT NULL REFERENCES sync_runs (id) ON DELETE CASCADE,
  identity_id   BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  permission_id BIGINT NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
  path_type     TEXT   NOT NULL,          -- 'direct_role' or 'via_group' (matches view)
  role_id       BIGINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
  group_id      BIGINT     REFERENCES groups (id) ON DELETE SET NULL,
  resource_id   BIGINT     REFERENCES resources (id) ON DELETE SET NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_effective_perm_snapshots_uniq
  ON effective_permission_snapshots (
    sync_run_id,
    identity_id,
    permission_id,
    path_type,
    COALESCE(group_id, 0),
    role_id
  );

CREATE INDEX idx_effective_perm_snapshots_identity
  ON effective_permission_snapshots (identity_id);

CREATE INDEX idx_effective_perm_snapshots_permission
  ON effective_permission_snapshots (permission_id);
