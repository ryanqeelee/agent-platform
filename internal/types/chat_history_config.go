package types

import "time"

const PlatformChatHistoryConfigSingletonID int16 = 1

// PlatformChatHistoryConfig is the sole deployment-wide switch and embedding
// model selection for message indexing. Tenant-specific KB identity is kept in
// TenantChatHistoryIndex and never accepted from the management API.
type PlatformChatHistoryConfig struct {
	ID               int16     `json:"-" gorm:"primaryKey;column:id"`
	Enabled          bool      `json:"enabled" gorm:"column:enabled;not null"`
	EmbeddingModelID string    `json:"embedding_model_id" gorm:"column:embedding_model_id;type:varchar(64);not null"`
	UpdatedBy        string    `json:"-" gorm:"column:updated_by;type:varchar(36);not null"`
	CreatedAt        time.Time `json:"-" gorm:"column:created_at"`
	UpdatedAt        time.Time `json:"-" gorm:"column:updated_at"`
}

func (*PlatformChatHistoryConfig) TableName() string { return "platform_chat_history_config" }

func (c *PlatformChatHistoryConfig) IsEnabled() bool {
	return c != nil && c.Enabled && c.EmbeddingModelID != ""
}

// TenantChatHistoryIndex binds one tenant to its private hidden KB. It stores
// no policy knobs; PlatformChatHistoryConfig remains the only configuration
// authority.
type TenantChatHistoryIndex struct {
	TenantID        uint64    `json:"-" gorm:"primaryKey;column:tenant_id"`
	KnowledgeBaseID string    `json:"-" gorm:"column:knowledge_base_id;type:varchar(36);not null;uniqueIndex"`
	CreatedAt       time.Time `json:"-" gorm:"column:created_at"`
	UpdatedAt       time.Time `json:"-" gorm:"column:updated_at"`
}

func (*TenantChatHistoryIndex) TableName() string { return "tenant_chat_history_indexes" }

// PlatformChatHistoryStats is the tenantless management projection.
type PlatformChatHistoryStats struct {
	Enabled                  bool   `json:"enabled"`
	EmbeddingModelID         string `json:"embedding_model_id,omitempty"`
	TenantKnowledgeBaseCount int64  `json:"tenant_knowledge_base_count"`
	IndexedMessageCount      int64  `json:"indexed_message_count"`
	HasIndexedMessages       bool   `json:"has_indexed_messages"`
}
