package types

import "time"

const PlatformMemoryRuntimeConfigSingletonID uint8 = 1

// PlatformMemoryRuntimeConfig is the deployment singleton that owns memory
// runtime parameters independently of every enterprise's consent.
type PlatformMemoryRuntimeConfig struct {
	ID         uint8                `json:"id" gorm:"primaryKey;column:id"`
	Runtime    *MemoryRuntimeConfig `json:"runtime" gorm:"column:runtime;type:jsonb;not null"`
	Generation int64                `json:"generation" gorm:"column:generation;not null"`
	UpdatedBy  string               `json:"updated_by" gorm:"column:updated_by;type:varchar(36);not null"`
	CreatedAt  time.Time            `json:"created_at" gorm:"column:created_at;not null"`
	UpdatedAt  time.Time            `json:"updated_at" gorm:"column:updated_at;not null"`
}

func (*PlatformMemoryRuntimeConfig) TableName() string { return "platform_memory_runtime_config" }
