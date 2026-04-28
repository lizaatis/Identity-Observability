-- 004_group_owner_column.sql
-- Adds optional owner_identity_id to groups for orphaned-group deadend detection.

ALTER TABLE groups
  ADD COLUMN owner_identity_id BIGINT REFERENCES identities (id);

CREATE INDEX idx_groups_owner_identity_id
  ON groups (owner_identity_id);
