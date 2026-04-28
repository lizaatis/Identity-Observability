-- 005_deadend_rules.sql
-- Views that surface "deadend" conditions for identities, groups, roles, and permissions.

-- NOTE: Replace 'okta_primary' below with your actual main IdP source_system value.

-- =============================================================================
-- 1) Orphaned users: Disabled in main IdP but active elsewhere
-- =============================================================================

CREATE OR REPLACE VIEW deadend_orphaned_users AS
SELECT
  i.id          AS identity_id,
  i.email,
  i.display_name,
  main.source_system    AS main_source_system,
  main.source_user_id   AS main_source_user_id
FROM identities i
JOIN identity_sources main
  ON main.identity_id   = i.id
 AND main.source_system = 'okta_primary'    -- TODO: change to your main IdP key
 AND main.source_status = 'disabled'
WHERE EXISTS (
  SELECT 1
  FROM identity_sources other
  WHERE other.identity_id   = i.id
    AND other.source_system <> main.source_system
    AND other.source_status = 'active'
);

-- =============================================================================
-- 2) Orphaned groups: no owner, yet assigned (to identities or roles)
-- =============================================================================

CREATE OR REPLACE VIEW deadend_orphaned_groups AS
SELECT DISTINCT
  g.id          AS group_id,
  g.name        AS group_name,
  g.source_system,
  g.source_id
FROM groups g
LEFT JOIN identity_group ig ON ig.group_id = g.id
LEFT JOIN group_role    gr ON gr.group_id = g.id
WHERE g.owner_identity_id IS NULL
  AND (ig.id IS NOT NULL OR gr.id IS NOT NULL);

-- Optional: expand to identity-level impact
CREATE OR REPLACE VIEW deadend_orphaned_group_identities AS
SELECT DISTINCT
  i.id        AS identity_id,
  g.group_id  AS group_id,
  g.group_name AS group_name
FROM deadend_orphaned_groups g
JOIN identity_group ig ON ig.group_id = g.group_id
JOIN identities    i  ON i.id = ig.identity_id;

-- =============================================================================
-- 3) Stale roles or groups: >90 days with no updates
-- =============================================================================

CREATE OR REPLACE VIEW deadend_stale_roles AS
SELECT
  r.*
FROM roles r
WHERE r.updated_at < NOW() - INTERVAL '90 days';

CREATE OR REPLACE VIEW deadend_stale_groups AS
SELECT
  g.*
FROM groups g
WHERE g.updated_at < NOW() - INTERVAL '90 days';

-- Optional: identities impacted by stale groups/roles
CREATE OR REPLACE VIEW deadend_stale_group_identities AS
SELECT DISTINCT
  i.id   AS identity_id,
  g.id   AS group_id,
  g.name AS group_name,
  g.updated_at
FROM deadend_stale_groups g
JOIN identity_group ig ON ig.group_id = g.id
JOIN identities    i  ON i.id = ig.identity_id;

CREATE OR REPLACE VIEW deadend_stale_role_identities AS
SELECT DISTINCT
  i.id   AS identity_id,
  r.id   AS role_id,
  r.name AS role_name,
  r.updated_at
FROM deadend_stale_roles r
LEFT JOIN identity_role ir ON ir.role_id = r.id
LEFT JOIN group_role   gr  ON gr.role_id = r.id
LEFT JOIN identity_group ig ON ig.group_id = gr.group_id
LEFT JOIN identities    i  ON i.id = COALESCE(ir.identity_id, ig.identity_id)
WHERE i.id IS NOT NULL;

-- =============================================================================
-- 4) Disconnected permissions: unreachable from any active identity
-- =============================================================================

CREATE OR REPLACE VIEW deadend_disconnected_permissions AS
SELECT
  p.*
FROM permissions p
WHERE NOT EXISTS (
  SELECT 1
  FROM identity_effective_permissions iep
  JOIN identities i ON i.id = iep.identity_id
  WHERE i.status = 'active'
    AND iep.permission_id = p.id
);
