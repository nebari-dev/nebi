package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/logstream"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/valkey-io/valkey-go"
)

type JobHandler struct {
	svc          *service.JobService
	broker       *logstream.LogBroker
	valkeyClient valkey.Client
}

func NewJobHandler(svc *service.JobService, broker *logstream.LogBroker, valkeyClient interface{}) *JobHandler {
	var client valkey.Client
	if valkeyClient != nil {
		client, _ = valkeyClient.(valkey.Client)
	}
	return &JobHandler{
		svc:          svc,
		broker:       broker,
		valkeyClient: client,
	}
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
// @Param token query string false "Auth token (alternative to Bearer header for EventSource compatibility)"
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

	// Stream real-time logs
	if h.valkeyClient != nil {
		h.streamLogsFromValkey(c, jobUUID)
	} else if h.broker != nil {
		h.streamLogsFromBroker(c, jobUUID)
	} else {
		fmt.Fprintf(c.Writer, "event: error\ndata: Log streaming not available\n\n")
		c.Writer.Flush()
	}
}

// streamLogsFromValkey streams logs from Valkey pub/sub channel
func (h *JobHandler) streamLogsFromValkey(c *gin.Context, jobID uuid.UUID) {
	channel := fmt.Sprintf("logs:%s", jobID.String())
	ctx := c.Request.Context()

	subscribeCmd := h.valkeyClient.B().Subscribe().Channel(channel).Build()

	err := h.valkeyClient.Receive(ctx, subscribeCmd, func(msg valkey.PubSubMessage) {
		logLine := msg.Message

		fmt.Fprintf(c.Writer, "data: %s\n\n", logLine)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}

		if logLine == "\n[COMPLETED] Job finished successfully\n" ||
			(len(logLine) > 7 && logLine[:7] == "\n[ERROR]") {
			fmt.Fprintf(c.Writer, "event: done\ndata: Job completed\n\n")
			c.Writer.Flush()
		}
	})

	if err != nil {
		if err.Error() != "context canceled" {
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", err.Error())
			c.Writer.Flush()
		}
	}
}

// streamLogsFromBroker streams logs from in-memory broker (fallback)
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
				fmt.Fprintf(c.Writer, "event: done\ndata: Stream ended\n\n")
				c.Writer.Flush()
				return
			}

			fmt.Fprintf(c.Writer, "data: %s\n\n", logLine)
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
