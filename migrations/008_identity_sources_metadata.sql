-- 008_identity_sources_metadata.sql
-- Add metadata JSONB to identity_sources for MFA and other per-source attributes.

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'identity_sources' AND column_name = 'metadata'
  ) THEN
    ALTER TABLE identity_sources ADD COLUMN metadata JSONB;
  END IF;
END $$;
