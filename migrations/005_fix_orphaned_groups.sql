-- Fix for deadend_orphaned_group_identities view
-- Drop and recreate with correct column references

DROP VIEW IF EXISTS deadend_orphaned_group_identities;

CREATE OR REPLACE VIEW deadend_orphaned_group_identities AS
SELECT DISTINCT
  i.id        AS identity_id,
  g.group_id  AS group_id,
  g.group_name AS group_name
FROM deadend_orphaned_groups g
JOIN identity_group ig ON ig.group_id = g.group_id
JOIN identities    i  ON i.id = ig.identity_id;
