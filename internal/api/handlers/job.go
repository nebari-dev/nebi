package handlers

import (
	"net/http"

	"github.com/aktech/darb/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type JobHandler struct {
	db *gorm.DB
}

func NewJobHandler(db *gorm.DB) *JobHandler {
	return &JobHandler{db: db}
}

// ListJobs godoc
// @Summary List all jobs for user's environments
// @Tags jobs
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Job
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /jobs [get]
func (h *JobHandler) ListJobs(c *gin.Context) {
	userID := getUserID(c)

	var jobs []models.Job
	err := h.db.
		Joins("JOIN environments ON environments.id = jobs.environment_id").
		Where("environments.owner_id = ?", userID).
		Order("jobs.created_at DESC").
		Find(&jobs).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch jobs"})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

// GetJob godoc
// @Summary Get a job by ID
// @Tags jobs
// @Security BearerAuth
// @Produce json
// @Param id path int true "Job ID"
// @Success 200 {object} models.Job
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /jobs/{id} [get]
func (h *JobHandler) GetJob(c *gin.Context) {
	userID := getUserID(c)
	jobID := c.Param("id")

	var job models.Job
	err := h.db.
		Joins("JOIN environments ON environments.id = jobs.environment_id").
		Where("jobs.id = ? AND environments.owner_id = ?", jobID, userID).
		First(&job).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch job"})
		return
	}

	c.JSON(http.StatusOK, job)
}
