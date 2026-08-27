package types

// CanManageMemberRole is the product-level membership lifecycle matrix.
// TenantRole names deliberately remain the stable storage/authorization
// values; contributor and viewer are presented as Knowledge Administrator and
// Employee by product surfaces.
func CanManageMemberRole(actor, target TenantRole) bool {
	switch actor {
	case TenantRoleOwner:
		return target == TenantRoleAdmin || target == TenantRoleContributor || target == TenantRoleViewer
	case TenantRoleAdmin:
		return target == TenantRoleContributor || target == TenantRoleViewer
	default:
		return false
	}
}

// CanInviteMemberRole applies the same matrix before a target membership
// exists. Neither invitations nor ordinary role writes may mint an Owner.
func CanInviteMemberRole(actor, invited TenantRole) bool {
	return CanManageMemberRole(actor, invited)
}
