package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/service"
)

type JobHandler struct {
	svc    *service.JobService
	broker *logstream.LogBroker
}

func NewJobHandler(svc *service.JobService, broker *logstream.LogBroker) *JobHandler {
	return &JobHandler{
		svc:    svc,
		broker: broker,
	}
}

func writeSSEEvent(w io.Writer, event, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}

	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(data)
	for _, line := range strings.Split(normalized, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func writeSSEData(w io.Writer, data string) {
	writeSSEEvent(w, "", data)
}

// ListJobs godoc
// @Summary List all jobs for user's workspaces
// @Tags jobs
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Job
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /jobs [get]
func (h *JobHandler) ListJobs(c *gin.Context) {
	jobs, err := h.svc.ListJobs(getUserID(c))
	if err != nil {
		handleServiceError(c, err)
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
	job, err := h.svc.GetJob(c.Param("id"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
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
// @Success 200 {string} string "event stream"
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /jobs/{id}/logs/stream [get]
func (h *JobHandler) StreamJobLogs(c *gin.Context) {
	userID := getUserID(c)

	jobUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid job ID"})
		return
	}

	job, err := h.svc.GetJobForStreaming(jobUUID, userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// If job is already completed or failed, send historical logs and close
	if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
		if job.Logs != "" {
			writeSSEData(c.Writer, job.Logs)
		}
		writeSSEEvent(c.Writer, "done", "Job already completed")
		c.Writer.Flush()
		return
	}

	// Send historical logs if any exist
	if job.Logs != "" {
		writeSSEData(c.Writer, job.Logs)
		c.Writer.Flush()
	}

	// Stream real-time logs
	if h.broker != nil {
		h.streamLogsFromBroker(c, jobUUID)
	} else {
		writeSSEEvent(c.Writer, "error", "Log streaming not available")
		c.Writer.Flush()
	}
}

// streamLogsFromBroker streams logs from in-memory broker
func (h *JobHandler) streamLogsFromBroker(c *gin.Context, jobID uuid.UUID) {
	logChan := h.broker.Subscribe(jobID)
	defer h.broker.Unsubscribe(jobID, logChan)

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			return
		case logLine, ok := <-logChan:
			if !ok {
				writeSSEEvent(c.Writer, "done", "Stream ended")
				c.Writer.Flush()
				return
			}

			writeSSEData(c.Writer, logLine)
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
