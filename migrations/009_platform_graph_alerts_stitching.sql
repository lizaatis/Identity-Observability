-- Platform state: graph sync visibility, alert dedupe, stitching review queue

CREATE TABLE IF NOT EXISTS graph_sync_state (
  id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  last_sync_at TIMESTAMPTZ,
  last_duration_ms INT,
  node_count INT,
  edge_count INT,
  last_error TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO graph_sync_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS alert_webhook_dedupe (
  identity_id BIGINT NOT NULL,
  rule_key TEXT NOT NULL,
  last_sent_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (identity_id, rule_key)
);

CREATE INDEX IF NOT EXISTS idx_alert_dedupe_sent ON alert_webhook_dedupe (last_sent_at);

CREATE TABLE IF NOT EXISTS stitching_review_queue (
  id BIGSERIAL PRIMARY KEY,
  identity_id BIGINT NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
  reason TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stitching_review_status ON stitching_review_queue (status)
  WHERE status = 'pending';
