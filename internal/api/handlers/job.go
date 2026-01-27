package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/valkey-io/valkey-go"
	"gorm.io/gorm"
)

type JobHandler struct {
	db           *gorm.DB
	broker       *logstream.LogBroker
	valkeyClient valkey.Client
}

func NewJobHandler(db *gorm.DB, broker *logstream.LogBroker, valkeyClient interface{}) *JobHandler {
	var client valkey.Client
	if valkeyClient != nil {
		client, _ = valkeyClient.(valkey.Client)
	}
	return &JobHandler{
		db:           db,
		broker:       broker,
		valkeyClient: client,
	}
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
		Select("jobs.*").
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
// @Param id path string true "Job ID"
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
		Select("jobs.*").
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

// StreamJobLogs godoc
// @Summary Stream job logs in real-time via Server-Sent Events
// @Tags jobs
// @Security BearerAuth
// @Produce text/event-stream
// @Param id path string true "Job ID"
// @Param token query string false "Auth token (alternative to Bearer header for EventSource compatibility)"
// @Success 200 {string} string "event stream"
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /jobs/{id}/logs/stream [get]
func (h *JobHandler) StreamJobLogs(c *gin.Context) {
	// Get userID from context (set by auth middleware)
	userID := getUserID(c)
	jobID := c.Param("id")

	// Parse job UUID
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid job ID"})
		return
	}

	// Verify job exists and user has access
	var job models.Job
	err = h.db.
		Select("jobs.*").
		Joins("JOIN environments ON environments.id = jobs.environment_id").
		Where("jobs.id = ? AND environments.owner_id = ?", jobUUID, userID).
		First(&job).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch job"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// If job is already completed or failed, send historical logs and close
	if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
		if job.Logs != "" {
			fmt.Fprintf(c.Writer, "data: %s\n\n", job.Logs)
		}
		fmt.Fprintf(c.Writer, "event: done\ndata: Job already completed\n\n")
		c.Writer.Flush()
		return
	}

	// Send historical logs if any exist
	if job.Logs != "" {
		fmt.Fprintf(c.Writer, "data: %s\n\n", job.Logs)
		c.Writer.Flush()
	}

	// Use Valkey pub/sub for distributed log streaming if available
	if h.valkeyClient != nil {
		h.streamLogsFromValkey(c, jobUUID)
	} else if h.broker != nil {
		// Fallback to in-memory broker
		h.streamLogsFromBroker(c, jobUUID)
	} else {
		// No streaming available, poll database
		fmt.Fprintf(c.Writer, "event: error\ndata: Log streaming not available\n\n")
		c.Writer.Flush()
	}
}

// streamLogsFromValkey streams logs from Valkey pub/sub channel
func (h *JobHandler) streamLogsFromValkey(c *gin.Context, jobID uuid.UUID) {
	channel := fmt.Sprintf("logs:%s", jobID.String())
	ctx := c.Request.Context()

	// Subscribe to Valkey pub/sub channel and receive messages
	subscribeCmd := h.valkeyClient.B().Subscribe().Channel(channel).Build()

	err := h.valkeyClient.Receive(ctx, subscribeCmd, func(msg valkey.PubSubMessage) {
		// Get log line from message
		logLine := msg.Message

		// Send log line to client via SSE
		fmt.Fprintf(c.Writer, "data: %s\n\n", logLine)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}

		// Check if this is a completion message
		if logLine == "\n[COMPLETED] Job finished successfully\n" ||
			(len(logLine) > 7 && logLine[:7] == "\n[ERROR]") {
			fmt.Fprintf(c.Writer, "event: done\ndata: Job completed\n\n")
			c.Writer.Flush()
		}
	})

	if err != nil {
		// Error subscribing or streaming (includes context cancellation)
		// This is normal when client disconnects
		if err.Error() != "context canceled" {
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", err.Error())
			c.Writer.Flush()
		}
	}
}

// streamLogsFromBroker streams logs from in-memory broker (fallback)
func (h *JobHandler) streamLogsFromBroker(c *gin.Context, jobID uuid.UUID) {
	// Subscribe to real-time log stream
	logChan := h.broker.Subscribe(jobID)
	defer h.broker.Unsubscribe(jobID, logChan)

	// Stream logs as they arrive
	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			// Client disconnected
			return
		case logLine, ok := <-logChan:
			if !ok {
				// Channel closed, job completed
				fmt.Fprintf(c.Writer, "event: done\ndata: Stream ended\n\n")
				c.Writer.Flush()
				return
			}

			// Send log line to client
			fmt.Fprintf(c.Writer, "data: %s\n\n", logLine)
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
