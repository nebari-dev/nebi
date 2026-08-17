package models

import "time"

// SystemSetting is a key/value row for internal bookkeeping flags
// (e.g. one-time seed markers). Not exposed via the API.
type SystemSetting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name for SystemSetting
func (SystemSetting) TableName() string {
	return "system_settings"
}
