DROP TABLE IF EXISTS knowledge_base_role_grants;
DROP TABLE IF EXISTS business_role_members;
DROP TABLE IF EXISTS business_roles;
DROP INDEX IF EXISTS idx_knowledge_bases_tenant_id_for_access_grants;
DROP INDEX IF EXISTS idx_tenant_members_tenant_id_for_business_roles;
-- Keep the corrected partial active-membership index and all membership rows:
-- rollback removes governance tables only, so a later re-upgrade remains
-- able to preserve historical rows and let soft-deleted members rejoin.
