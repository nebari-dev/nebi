package models

import "time"

// ResourceMetric stores monotonically increasing counters that must be visible
// across split API-server and worker deployments.
type ResourceMetric struct {
	Name      string    `gorm:"primaryKey;type:text" json:"name"`
	Value     int64     `gorm:"not null;default:0" json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
