// Package types: platform sandbox backend config entity.
//
// Sandbox provider identities and credentials are platform configuration. A
// tenant may reference a config ID from an agent or session, but does not own
// the config row. The credential-bearing payload lives in Config, reusing
// TenantSandboxConfig's encrypted Value/Scan hooks.
package types

import (
	"time"

	"gorm.io/gorm"
)

// SandboxConfigIDGlobalDefault is retained for sessions created by older
// versions that used a deployment-wide sandbox configuration.
//
// A sentinel rather than the empty string, so NULL on sessions.sandbox_config_id
// unambiguously means "this session has no live sandbox".
const SandboxConfigIDGlobalDefault = "-"

// SandboxCordonLease bounds how long a cordon is honoured. Identity changes
// take the cordon for the duration of two provider API calls; anything older
// is a crashed handler's leftover and must not wedge the config.
const SandboxCordonLease = 2 * time.Minute

// SandboxWorkspacePolicyConfigName identifies the legacy pseudo-config that
// migrations promote to Tenant.SandboxScriptsDisabled. New code must never
// create or interpret this row.
const SandboxWorkspacePolicyConfigName = "__workspace_scripts_policy__"

// IsSandboxWorkspacePolicyRow reports whether e is the internal policy row.
func IsSandboxWorkspacePolicyRow(e *TenantSandboxConfigEntity) bool {
	return e != nil && e.Name == SandboxWorkspacePolicyConfigName
}

// TenantSandboxConfigEntity is one platform-owned sandbox backend
// configuration. The historical Go name is retained because the encrypted
// payload type is also named TenantSandboxConfig; persistence is platform
// global and deliberately has no tenant field.
type TenantSandboxConfigEntity struct {
	ID          string `gorm:"type:varchar(36);primaryKey"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text"`

	// SandboxType is promoted out of Config so listing and cleanup decisions
	// do not have to decrypt and unmarshal the payload.
	SandboxType string `gorm:"type:varchar(32);not null"`
	IsDefault   bool   `gorm:"not null;default:false"`

	Config *TenantSandboxConfig `gorm:"type:jsonb"`

	// CordonedAt is held while identity fields are being changed. See
	// IsCordoned: it is a lease, never a permanent lock.
	CordonedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName pins the table so GORM's pluralizer cannot drift.
func (e *TenantSandboxConfigEntity) TableName() string {
	return "platform_sandbox_configs"
}

// IsCordoned reports whether sandbox resolution must be refused for this
// config right now.
func (e *TenantSandboxConfigEntity) IsCordoned(now time.Time, lease time.Duration) bool {
	if e == nil || e.CordonedAt == nil {
		return false
	}
	return now.Sub(*e.CordonedAt) < lease
}
