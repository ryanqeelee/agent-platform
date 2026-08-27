package types

import (
	"time"

	"gorm.io/gorm"
)

// BusinessRole is a tenant-local employee grouping used only to scope
// knowledge consumption. It is deliberately independent of TenantRole.
type BusinessRole struct {
	ID        string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID  uint64         `json:"tenant_id" gorm:"not null;index"`
	Name      string         `json:"name" gorm:"type:varchar(128);not null"`
	Enabled   bool           `json:"enabled" gorm:"not null;default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (BusinessRole) TableName() string { return "business_roles" }

// BusinessRoleMember assigns an enterprise member to a BusinessRole. Disabled
// roles retain these rows so re-enabling restores their access automatically.
type BusinessRoleMember struct {
	TenantID       uint64    `json:"tenant_id" gorm:"primaryKey"`
	RoleID         string    `json:"role_id" gorm:"type:varchar(36);primaryKey"`
	TenantMemberID uint64    `json:"tenant_member_id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"created_at"`
}

func (BusinessRoleMember) TableName() string { return "business_role_members" }

// KnowledgeBaseRoleGrant is a tenant-local allow-list entry. No rows for a
// knowledge base means enterprise-wide employee access.
type KnowledgeBaseRoleGrant struct {
	TenantID        uint64    `json:"tenant_id" gorm:"primaryKey"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);primaryKey"`
	RoleID          string    `json:"role_id" gorm:"type:varchar(36);primaryKey"`
	CreatedAt       time.Time `json:"created_at"`
}

func (KnowledgeBaseRoleGrant) TableName() string { return "knowledge_base_role_grants" }
