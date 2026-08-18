package models

import "time"

const ResourceLockJobAdmission = "job_admission"

// ResourceLock provides durable rows that transactions can update to serialize
// resource-sensitive decisions across API server processes.
type ResourceLock struct {
	Name      string    `gorm:"primaryKey;type:text" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
