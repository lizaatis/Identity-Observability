-- 007_action_automation.sql
-- Action Hub, Approval Workflow, and Remediation Tracking

-- =============================================================================
-- Remediation Actions
-- =============================================================================

CREATE TABLE remediation_actions (
  id              BIGSERIAL PRIMARY KEY,
  identity_id     BIGINT REFERENCES identities (id) ON DELETE CASCADE,
  risk_flag_id   BIGINT REFERENCES risk_flags (id) ON DELETE SET NULL,
  action_type     TEXT NOT NULL,              -- 'disable_user', 'remove_permission', 'remove_group', 'create_ticket'
  target_system   TEXT NOT NULL,              -- 'okta', 'sailpoint', 'jira', etc.
  target_id       TEXT,                       -- ID in target system
  status          TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'approved', 'rejected', 'executed', 'failed'
  requested_by    TEXT NOT NULL,              -- User who requested action
  requested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  approved_by     TEXT,                       -- User who approved
  approved_at     TIMESTAMPTZ,
  executed_at     TIMESTAMPTZ,
  error_message   TEXT,
  metadata        JSONB,                      -- Additional action details
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_remediation_actions_identity ON remediation_actions (identity_id);
CREATE INDEX idx_remediation_actions_status ON remediation_actions (status);
CREATE INDEX idx_remediation_actions_requested_at ON remediation_actions (requested_at DESC);

-- =============================================================================
-- Approval Requests
-- =============================================================================

CREATE TABLE approval_requests (
  id              BIGSERIAL PRIMARY KEY,
  remediation_action_id BIGINT NOT NULL REFERENCES remediation_actions (id) ON DELETE CASCADE,
  channel         TEXT NOT NULL,              -- 'slack', 'email', 'internal'
  channel_id      TEXT,                       -- Slack channel ID, email address, etc.
  message_id      TEXT,                       -- Slack message timestamp, email ID, etc.
  status          TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'approved', 'rejected', 'expired'
  expires_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_approval_requests_action ON approval_requests (remediation_action_id);
CREATE INDEX idx_approval_requests_status ON approval_requests (status);

-- =============================================================================
-- Remediation History (for Executive Dashboard)
-- =============================================================================

CREATE TABLE remediation_history (
  id              BIGSERIAL PRIMARY KEY,
  remediation_action_id BIGINT REFERENCES remediation_actions (id) ON DELETE SET NULL,
  identity_id     BIGINT REFERENCES identities (id) ON DELETE CASCADE,
  risk_type       TEXT NOT NULL,              -- 'zombie_account', 'toxic_combo', 'orphaned_user', etc.
  action_type     TEXT NOT NULL,
  status          TEXT NOT NULL,               -- 'executed', 'failed'
  resolved_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  time_to_resolve INTERVAL,                   -- Time from detection to resolution
  metadata        JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_remediation_history_identity ON remediation_history (identity_id);
CREATE INDEX idx_remediation_history_risk_type ON remediation_history (risk_type);
CREATE INDEX idx_remediation_history_resolved_at ON remediation_history (resolved_at DESC);

-- =============================================================================
-- Custom Rules (User-defined)
-- =============================================================================

CREATE TABLE custom_rules (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  description     TEXT,
  rule_yaml       TEXT NOT NULL,              -- YAML rule definition
  severity        TEXT NOT NULL DEFAULT 'medium',
  enabled         BOOLEAN NOT NULL DEFAULT true,
  created_by      TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_tested_at  TIMESTAMPTZ,
  test_results    JSONB                       -- Last test execution results
);

CREATE INDEX idx_custom_rules_enabled ON custom_rules (enabled);
CREATE INDEX idx_custom_rules_created_at ON custom_rules (created_at DESC);

-- =============================================================================
-- Risk Trend Tracking (for Executive Dashboard)
-- =============================================================================

CREATE TABLE risk_trends (
  id              BIGSERIAL PRIMARY KEY,
  date            DATE NOT NULL,
  total_identities INTEGER NOT NULL DEFAULT 0,
  critical_count  INTEGER NOT NULL DEFAULT 0,
  high_count      INTEGER NOT NULL DEFAULT 0,
  medium_count    INTEGER NOT NULL DEFAULT 0,
  low_count       INTEGER NOT NULL DEFAULT 0,
  resolved_24h    INTEGER NOT NULL DEFAULT 0, -- Critical risks resolved within 24h
  resolved_7d     INTEGER NOT NULL DEFAULT 0, -- All risks resolved within 7d
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (date)
);

CREATE INDEX idx_risk_trends_date ON risk_trends (date DESC);

-- =============================================================================
-- Triggers
-- =============================================================================

CREATE TRIGGER remediation_actions_updated_at
  BEFORE UPDATE ON remediation_actions FOR EACH ROW EXECUTE PROCEDURE set_updated_at();

CREATE TRIGGER approval_requests_updated_at
  BEFORE UPDATE ON approval_requests FOR EACH ROW EXECUTE PROCEDURE set_updated_at();

CREATE TRIGGER custom_rules_updated_at
  BEFORE UPDATE ON custom_rules FOR EACH ROW EXECUTE PROCEDURE set_updated_at();
