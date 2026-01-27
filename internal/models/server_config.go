package models

import (
	"time"
)

// ServerConfig stores server-wide configuration as key-value pairs
type ServerConfig struct {
	Key       string    `gorm:"primarykey;not null" json:"key"`
	Value     string    `gorm:"not null" json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServerConfigKeys defines known configuration keys
const (
	ServerConfigKeyServerID = "server_id"
)
