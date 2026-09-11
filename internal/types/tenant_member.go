package types

import (
	"time"

	"gorm.io/gorm"
)

// TenantRole represents a user's role inside a single tenant.
//
// Tenant roles govern intra-tenant authority (who can create/edit/delete
// resources, manage tenant settings, etc.) and are orthogonal to the
// OrgMemberRole defined in organization.go, which governs cross-tenant
// sharing. A user may therefore carry different TenantRole values in
// different tenants (one TenantMember row per (user, tenant) pair).
type TenantRole string

const (
	// TenantRoleAdmin manages enterprise members and knowledge; platform infrastructure remains separate.
	TenantRoleAdmin TenantRole = "admin"
	// TenantRoleViewer consumes knowledge allowed by business-role grants.
	TenantRoleViewer TenantRole = "viewer"
	// Retired storage values. They are invalid at runtime and migrated to admin/viewer.
	TenantRoleOwner       TenantRole = "owner"
	TenantRoleContributor TenantRole = "contributor"
)

// tenantRoleLevel maps each role to a numeric level used for hierarchy
// comparisons. Higher means more privileged. Levels are spaced by 10 so
// new roles can be inserted between existing ones if needed.
var tenantRoleLevel = map[TenantRole]int{
	TenantRoleAdmin:  30,
	TenantRoleViewer: 10,
}

// IsValid reports whether r is one of the two active tenant roles.
func (r TenantRole) IsValid() bool {
	_, ok := tenantRoleLevel[r]
	return ok
}

// Level returns the numeric privilege level of the role. Unknown roles
// return 0, which is strictly less than any defined role.
func (r TenantRole) Level() int {
	return tenantRoleLevel[r]
}

// HasPermission reports whether r is at least as privileged as required.
// Used by RequireRole-style middleware to gate endpoints.
func (r TenantRole) HasPermission(required TenantRole) bool {
	return r.IsValid() && required.IsValid() && r.Level() >= required.Level()
}

// TenantMemberStatus enumerates the lifecycle states of a membership row.
type TenantMemberStatus string

// MemberActorAuthority is the small authority input for tenant lifecycle
// mutations. ServicePrincipal is an already route-authorized machine or
// cross-tenant operator and is still barred from Owner operations.
type MemberActorAuthority struct {
	UserID              string
	ServicePrincipal    bool
	SystemAdministrator bool
}

const (
	// TenantMemberStatusActive is the normal membership state; the user
	// can authenticate into the tenant and is subject to their role.
	TenantMemberStatusActive TenantMemberStatus = "active"
	// TenantMemberStatusInvited represents a pending invitation that has
	// not yet been accepted. The auth middleware treats this as "not a
	// member" until the status flips to active.
	TenantMemberStatusInvited TenantMemberStatus = "invited"
	// TenantMemberStatusSuspended is an admin-revoked membership. The
	// row is preserved for audit trail but the user cannot authenticate
	// into the tenant.
	TenantMemberStatusSuspended TenantMemberStatus = "suspended"
)

// TenantMember represents the (user, tenant) membership record that
// carries the user's TenantRole for that specific tenant.
//
// User.TenantID is the enterprise binding. TenantMember records the role
// held by that user inside the bound enterprise.
type TenantMember struct {
	// Surrogate primary key.
	ID uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	// UserID references users.id. Together with TenantID forms the logical
	// key enforced by the partial unique index uniq_user_tenant.
	UserID string `json:"user_id" gorm:"type:varchar(36);not null;index"`
	// TenantID references tenants.id.
	TenantID uint64 `json:"tenant_id" gorm:"not null;index"`
	// Role held by the user inside this tenant.
	Role TenantRole `json:"role" gorm:"type:varchar(20);not null;default:'viewer'"`
	// Status controls whether this membership is honoured by the auth
	// middleware; see TenantMemberStatus constants.
	Status TenantMemberStatus `json:"status" gorm:"type:varchar(20);not null;default:'active'"`
	// OperatingAnalysisAccess is the explicit, role-independent permission
	// to use the governed operating-analysis workspace for this tenant.
	OperatingAnalysisAccess bool `json:"operating_analysis_access" gorm:"not null;default:false"`
	// InvitedBy records the user ID of the admin who created this row via
	// an invitation flow. Nil for rows created by self-service registration.
	InvitedBy *string `json:"invited_by,omitempty" gorm:"type:varchar(36)"`
	// JoinedAt is when the membership became active.
	JoinedAt  time.Time      `json:"joined_at"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName binds TenantMember to the tenant_members table.
func (TenantMember) TableName() string {
	return "tenant_members"
}

// Membership is the login-response-friendly projection of a TenantMember
// joined with tenant name. Returned as part of LoginResponse so the
// frontend can render a tenant switcher and gate UI by role.
type Membership struct {
	TenantID   uint64     `json:"tenant_id"`
	TenantName string     `json:"tenant_name"`
	Role       TenantRole `json:"role"`
}

// OperatingAnalysisAccessV1 is the strict Product Base assertion consumed by
// Ringxun. It is built from the current persisted membership, never from the
// login membership fallback used for UI compatibility.
type OperatingAnalysisAccessV1 struct {
	Schema           string             `json:"schema"`
	TenantID         uint64             `json:"tenant_id"`
	MembershipStatus TenantMemberStatus `json:"membership_status"`
	Enabled          bool               `json:"enabled"`
}

// TenantMemberResponse is the API projection of a TenantMember row joined
// with the human-facing user fields the management UI needs (email,
// username, avatar). It is intentionally NOT the GORM model: returning
// the model directly would leak DeletedAt/UpdatedAt and lock the DB
// schema into the public API. Use this for `/tenants/:id/members` only.
type TenantMemberResponse struct {
	UserID                  string             `json:"user_id"`
	Email                   string             `json:"email"`
	Username                string             `json:"username"`
	Avatar                  string             `json:"avatar,omitempty"`
	Role                    TenantRole         `json:"role"`
	Status                  TenantMemberStatus `json:"status"`
	InvitedBy               *string            `json:"invited_by,omitempty"`
	JoinedAt                time.Time          `json:"joined_at"`
	BusinessRoleIDs         []string           `json:"business_role_ids"`
	OperatingAnalysisAccess *bool              `json:"operating_analysis_access,omitempty"`
}
