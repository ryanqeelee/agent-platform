package types

import "time"

const PlatformParserConfigSingletonID uint8 = 1

// PlatformParserConfig is the single deployment-wide parser connection row.
// Knowledge bases and agents may override parser selection. This singleton also
// holds the deployment-wide default selection rules and engine connections.
type PlatformParserConfig struct {
	ID        uint8               `json:"id" gorm:"primaryKey;column:id"`
	Config    *ParserEngineConfig `json:"config" gorm:"type:jsonb;not null"`
	UpdatedBy string              `json:"updated_by" gorm:"column:updated_by;type:varchar(36);not null"`
	CreatedAt time.Time           `json:"created_at" gorm:"column:created_at;not null"`
	UpdatedAt time.Time           `json:"updated_at" gorm:"column:updated_at;not null"`
}

func (*PlatformParserConfig) TableName() string { return "platform_parser_config" }
