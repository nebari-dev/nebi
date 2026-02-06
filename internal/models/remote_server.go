package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RemoteServer stores the connection details for a remote Nebi server.
// The desktop app (local mode) can connect to one remote server at a time.
type RemoteServer struct {
	ID        uuid.UUID `gorm:"type:text;primary_key" json:"id"`
	URL       string    `gorm:"not null" json:"url"`
	Token     string    `json:"-"` // JWT from remote login, hidden from JSON
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate hook to generate UUID
func (r *RemoteServer) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
