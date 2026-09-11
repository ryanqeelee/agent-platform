package types

import "time"

type PlatformInitialAdministratorReceipt struct {
	CommandID     string `gorm:"type:varchar(128);primaryKey"`
	RequestSHA256 string `gorm:"type:varchar(64);not null"`
	UserID        string `gorm:"type:varchar(36);not null;uniqueIndex"`
	Username      string `gorm:"type:varchar(100);not null"`
	Email         string `gorm:"type:varchar(255);not null"`
	Status        string `gorm:"type:varchar(32);not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PlatformInitialAdministratorResult struct {
	CommandID string `json:"command_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	Replayed  bool   `json:"replayed"`
}
