package queue

import (
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

func tenantKeyForJob(job *models.Job) string {
	if job.UserID != uuid.Nil {
		return "user:" + job.UserID.String()
	}
	if job.WorkspaceID != uuid.Nil {
		return "workspace:" + job.WorkspaceID.String()
	}
	return "anonymous"
}
