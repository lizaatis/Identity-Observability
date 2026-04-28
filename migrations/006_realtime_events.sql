-- 006_realtime_events.sql
-- Real-time event ingestion: TimescaleDB extension and identity_events hypertable
-- Run after all previous migrations

-- =============================================================================
-- Enable TimescaleDB extension
-- =============================================================================

-- Note: TimescaleDB must be installed in PostgreSQL
-- For Docker: Use timescale/timescaledb:latest-pg16 image instead of postgres:16-alpine
-- Or install extension: CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- =============================================================================
-- Identity Events Table (Time-series data)
-- =============================================================================

-- Raw events from IdP webhooks (Okta, SailPoint, etc.)
CREATE TABLE identity_events (
  id              BIGSERIAL,
  event_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  source_system   TEXT NOT NULL,              -- 'okta', 'sailpoint', etc.
  event_type      TEXT NOT NULL,              -- 'user.lifecycle.create', 'user.mfa.factor.enroll', etc.
  identity_id     BIGINT REFERENCES identities (id) ON DELETE SET NULL,
  source_user_id  TEXT,                       -- IdP's user ID (before resolution)
  event_data      JSONB NOT NULL,             -- Full event payload
  processed       BOOLEAN NOT NULL DEFAULT FALSE,
  processed_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (id, event_time)
);

-- Convert to hypertable (TimescaleDB)
SELECT create_hypertable('identity_events', 'event_time', 
  chunk_time_interval => INTERVAL '1 day',
  if_not_exists => TRUE
);

-- Indexes for fast queries
CREATE INDEX idx_identity_events_identity_time 
  ON identity_events (identity_id, event_time DESC) 
  WHERE identity_id IS NOT NULL;

CREATE INDEX idx_identity_events_source_time 
  ON identity_events (source_system, event_time DESC);

CREATE INDEX idx_identity_events_type_time 
  ON identity_events (event_type, event_time DESC);

CREATE INDEX idx_identity_events_processed 
  ON identity_events (processed, event_time DESC) 
  WHERE processed = FALSE;

-- =============================================================================
-- Event Timeline View (Human-readable changes)
-- =============================================================================

-- Aggregated view of identity changes for timeline UI
CREATE OR REPLACE VIEW identity_timeline AS
SELECT 
  ie.id AS event_id,
  ie.event_time,
  ie.source_system,
  ie.event_type,
  ie.identity_id,
  i.email,
  i.display_name,
  ie.event_data,
  -- Extract common fields from event_data JSONB
  ie.event_data->>'displayMessage' AS display_message,
  ie.event_data->>'severity' AS severity,
  ie.event_data->>'outcome' AS outcome,
  -- Categorize event types
  CASE 
    WHEN ie.event_type LIKE 'user.lifecycle.%' THEN 'lifecycle'
    WHEN ie.event_type LIKE 'user.mfa.%' THEN 'mfa'
    WHEN ie.event_type LIKE 'group.user_membership.%' THEN 'group_membership'
    WHEN ie.event_type LIKE 'user.account.%' THEN 'account'
    WHEN ie.event_type LIKE 'app.assignment.%' THEN 'app_assignment'
    ELSE 'other'
  END AS event_category
FROM identity_events ie
LEFT JOIN identities i ON i.id = ie.identity_id
WHERE ie.processed = TRUE
ORDER BY ie.event_time DESC;

-- =============================================================================
-- Risk Velocity View (High-risk events per week)
-- =============================================================================

-- Calculate risk velocity: count of high-risk events per identity per week
CREATE OR REPLACE VIEW identity_risk_velocity AS
SELECT 
  identity_id,
  DATE_TRUNC('week', event_time) AS week_start,
  COUNT(*) AS high_risk_event_count,
  ARRAY_AGG(DISTINCT event_type) AS event_types,
  MAX(event_time) AS last_event_time
FROM identity_events
WHERE 
  processed = TRUE
  AND (
    -- High-risk event types
    event_type IN (
      'user.lifecycle.activate',
      'user.lifecycle.suspend',
      'user.mfa.factor.enroll',
      'user.mfa.factor.deactivate',
      'group.user_membership.add',
      'app.assignment.create',
      'user.account.update_password',
      'user.account.lock'
    )
    OR
    -- Events with high severity
    (event_data->>'severity')::text IN ('HIGH', 'CRITICAL')
  )
GROUP BY identity_id, DATE_TRUNC('week', event_time)
HAVING COUNT(*) > 0;

-- =============================================================================
-- Helper function: Get timeline for an identity
-- =============================================================================

CREATE OR REPLACE FUNCTION get_identity_timeline(
  p_identity_id BIGINT,
  p_limit INTEGER DEFAULT 100,
  p_offset INTEGER DEFAULT 0
)
RETURNS TABLE (
  event_id BIGINT,
  event_time TIMESTAMPTZ,
  source_system TEXT,
  event_type TEXT,
  event_category TEXT,
  display_message TEXT,
  event_data JSONB
) AS $$
BEGIN
  RETURN QUERY
  SELECT 
    ie.id,
    ie.event_time,
    ie.source_system,
    ie.event_type,
    CASE 
      WHEN ie.event_type LIKE 'user.lifecycle.%' THEN 'lifecycle'
      WHEN ie.event_type LIKE 'user.mfa.%' THEN 'mfa'
      WHEN ie.event_type LIKE 'group.user_membership.%' THEN 'group_membership'
      WHEN ie.event_type LIKE 'user.account.%' THEN 'account'
      WHEN ie.event_type LIKE 'app.assignment.%' THEN 'app_assignment'
      ELSE 'other'
    END AS event_category,
    ie.event_data->>'displayMessage' AS display_message,
    ie.event_data
  FROM identity_events ie
  WHERE ie.identity_id = p_identity_id
    AND ie.processed = TRUE
  ORDER BY ie.event_time DESC
  LIMIT p_limit
  OFFSET p_offset;
END;
$$ LANGUAGE plpgsql;
