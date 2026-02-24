-- Identity Observability — Effective access (materialized view + lineage view)
-- Run after 001_canonical_schema.sql. Requires canonical tables to exist.

-- =============================================================================
-- Effective permissions: all permissions an identity has (direct role + via group)
-- =============================================================================

-- Path 1: Identity -> Role -> Permission (direct role assignment)
-- Path 2: Identity -> Group -> Role -> Permission (via group membership)
CREATE OR REPLACE VIEW identity_effective_permissions AS
SELECT DISTINCT
  i.id AS identity_id,
  p.id AS permission_id,
  p.name AS permission_name,
  p.resource_type,
  p.source_system AS permission_source_system,
  r.id AS role_id,
  r.name AS role_name,
  r.privilege_level,
  r.source_system AS role_source_system,
  'direct_role' AS path_type,
  NULL::BIGINT AS group_id,
  NULL::TEXT AS group_name
FROM identities i
JOIN identity_role ir ON ir.identity_id = i.id
JOIN roles r ON r.id = ir.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

SELECT DISTINCT
  i.id AS identity_id,
  p.id AS permission_id,
  p.name AS permission_name,
  p.resource_type,
  p.source_system AS permission_source_system,
  r.id AS role_id,
  r.name AS role_name,
  r.privilege_level,
  r.source_system AS role_source_system,
  'via_group' AS path_type,
  g.id AS group_id,
  g.name AS group_name
FROM identities i
JOIN identity_group ig ON ig.identity_id = i.id
JOIN groups g ON g.id = ig.group_id
JOIN group_role gr ON gr.group_id = g.id
JOIN roles r ON r.id = gr.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id;

-- =============================================================================
-- Access lineage: one row per path for explainability (Identity -> Group? -> Role -> Permission)
-- =============================================================================

CREATE OR REPLACE VIEW identity_access_lineage AS
-- Direct: Identity -> Role -> Permission
SELECT
  i.id AS identity_id,
  p.id AS permission_id,
  'identity' AS hop_type,
  i.id AS hop_entity_id,
  i.display_name AS hop_name,
  NULL::TEXT AS hop_detail,
  1 AS ord
FROM identities i
JOIN identity_role ir ON ir.identity_id = i.id
JOIN roles r ON r.id = ir.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

SELECT
  i.id,
  p.id,
  'role' AS hop_type,
  r.id AS hop_entity_id,
  r.name AS hop_name,
  r.source_system AS hop_detail,
  2 AS ord
FROM identities i
JOIN identity_role ir ON ir.identity_id = i.id
JOIN roles r ON r.id = ir.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

SELECT
  i.id,
  p.id,
  'permission' AS hop_type,
  p.id AS hop_entity_id,
  p.name AS hop_name,
  p.source_system AS hop_detail,
  3 AS ord
FROM identities i
JOIN identity_role ir ON ir.identity_id = i.id
JOIN roles r ON r.id = ir.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

-- Via group: Identity -> Group -> Role -> Permission
SELECT
  i.id,
  p.id,
  'identity',
  i.id,
  i.display_name,
  NULL,
  1
FROM identities i
JOIN identity_group ig ON ig.identity_id = i.id
JOIN groups g ON g.id = ig.group_id
JOIN group_role gr ON gr.group_id = g.id
JOIN roles r ON r.id = gr.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

SELECT
  i.id,
  p.id,
  'group',
  g.id,
  g.name,
  g.source_system,
  2
FROM identities i
JOIN identity_group ig ON ig.identity_id = i.id
JOIN groups g ON g.id = ig.group_id
JOIN group_role gr ON gr.group_id = g.id
JOIN roles r ON r.id = gr.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

SELECT
  i.id,
  p.id,
  'role',
  r.id,
  r.name,
  r.source_system,
  3
FROM identities i
JOIN identity_group ig ON ig.identity_id = i.id
JOIN groups g ON g.id = ig.group_id
JOIN group_role gr ON gr.group_id = g.id
JOIN roles r ON r.id = gr.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id

UNION ALL

SELECT
  i.id,
  p.id,
  'permission',
  p.id,
  p.name,
  p.source_system,
  4
FROM identities i
JOIN identity_group ig ON ig.identity_id = i.id
JOIN groups g ON g.id = ig.group_id
JOIN group_role gr ON gr.group_id = g.id
JOIN roles r ON r.id = gr.role_id
JOIN role_permission rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id;

-- =============================================================================
-- Optional: materialized view for faster reads (refresh after sync)
-- =============================================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS identity_effective_permissions_mv AS
SELECT * FROM identity_effective_permissions;

CREATE UNIQUE INDEX IF NOT EXISTS idx_identity_effective_perm_mv
  ON identity_effective_permissions_mv (identity_id, permission_id, path_type, COALESCE(group_id, 0), role_id);

-- Refresh with: REFRESH MATERIALIZED VIEW CONCURRENTLY identity_effective_permissions_mv;
-- (CONCURRENTLY requires the unique index above)
