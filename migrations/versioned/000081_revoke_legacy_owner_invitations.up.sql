-- Owner is not an inviteable role. Preserve legacy pending rows for audit,
-- but make them unusable rather than silently rewriting their authority.
UPDATE tenant_invitations
SET status = 'revoked',
    responded_at = NOW(),
    updated_at = NOW()
WHERE status = 'pending'
  AND role = 'owner'
  AND deleted_at IS NULL;
