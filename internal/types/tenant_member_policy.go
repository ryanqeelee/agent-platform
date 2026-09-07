package types

// CanManageMemberRole is the enterprise administrator/employee matrix.
// Self changes and the last active administrator are checked transactionally.
func CanManageMemberRole(actor, target TenantRole) bool {
	return actor == TenantRoleAdmin && target.IsValid()
}

func CanInviteMemberRole(actor, invited TenantRole) bool {
	return CanManageMemberRole(actor, invited)
}
