ALTER TABLE tenant_members
    ADD COLUMN IF NOT EXISTS operating_analysis_access BOOLEAN NOT NULL DEFAULT FALSE;
