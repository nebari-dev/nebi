package metrics

import (
	"fmt"
	"time"

	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MetricQuotaRejectedGlobal    = "quota_rejected_global"
	MetricQuotaRejectedUser      = "quota_rejected_user"
	MetricQuotaRejectedWorkspace = "quota_rejected_workspace"
	MetricJobTimeouts            = "job_timeouts"
)

type ResourceSnapshot struct {
	QuotaRejections QuotaRejectionSnapshot `json:"quota_rejections"`
	JobTimeouts     int64                  `json:"job_timeouts_total"`
}

type QuotaRejectionSnapshot struct {
	Global    int64 `json:"global"`
	User      int64 `json:"user"`
	Workspace int64 `json:"workspace"`
	Total     int64 `json:"total"`
}

func IncQuotaRejected(db *gorm.DB, scope string) error {
	switch scope {
	case "global":
		return increment(db, MetricQuotaRejectedGlobal, 1)
	case "user":
		return increment(db, MetricQuotaRejectedUser, 1)
	case "workspace":
		return increment(db, MetricQuotaRejectedWorkspace, 1)
	default:
		return nil
	}
}

func IncJobTimeout(db *gorm.DB) error {
	return increment(db, MetricJobTimeouts, 1)
}

func Snapshot(db *gorm.DB) (ResourceSnapshot, error) {
	names := []string{
		MetricQuotaRejectedGlobal,
		MetricQuotaRejectedUser,
		MetricQuotaRejectedWorkspace,
		MetricJobTimeouts,
	}
	var rows []models.ResourceMetric
	if err := db.Where("name IN ?", names).Find(&rows).Error; err != nil {
		return ResourceSnapshot{}, fmt.Errorf("fetch resource metrics: %w", err)
	}

	values := make(map[string]int64, len(names))
	for _, row := range rows {
		values[row.Name] = row.Value
	}

	global := values[MetricQuotaRejectedGlobal]
	user := values[MetricQuotaRejectedUser]
	workspace := values[MetricQuotaRejectedWorkspace]
	return ResourceSnapshot{
		QuotaRejections: QuotaRejectionSnapshot{
			Global:    global,
			User:      user,
			Workspace: workspace,
			Total:     global + user + workspace,
		},
		JobTimeouts: values[MetricJobTimeouts],
	}, nil
}

func increment(db *gorm.DB, name string, delta int64) error {
	now := time.Now().UTC()
	metric := models.ResourceMetric{Name: name, Value: delta, CreatedAt: now, UpdatedAt: now}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":      gorm.Expr("value + ?", delta),
			"updated_at": now,
		}),
	}).Create(&metric).Error
}
