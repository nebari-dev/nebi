package api

// Owner: U8 (jobs).

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

func (a *API) registerJobsRoutes(g *gin.RouterGroup) {
	g.GET("/jobs", a.listJobs)
	g.GET("/jobs/:jobId", a.getJob)
	g.POST("/jobs/:jobId/cancel", a.cancelJob)
	g.GET("/jobs/:jobId/logs/stream", a.streamJobLogs)
}

// listJobs: GET /client/jobs?status=running,failed&type=create_version&
// project_id=<uuid>&sort=-started_at&limit=100 -> []shared.JobSummary
func (a *API) listJobs(c *gin.Context) {
	filter := shared.JobListFilter{Sort: c.DefaultQuery("sort", "-started_at")}
	for _, s := range splitCSV(c.Query("status")) {
		filter.Statuses = append(filter.Statuses, models.JobStatus(s))
	}
	for _, t := range splitCSV(c.Query("type")) {
		filter.Types = append(filter.Types, models.JobType(t))
	}
	if p := c.Query("project_id"); p != "" {
		id, err := uuid.Parse(p)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid project_id", Code: "bad_request", Field: "project_id"})
			return
		}
		filter.ProjectID = &id
	}
	if l := c.Query("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid limit", Code: "bad_request", Field: "limit"})
			return
		}
		filter.Limit = n
	}
	res, err := a.Jobs.List(c.Request.Context(), filter)
	if err != nil {
		writeError(c, err)
		return
	}
	if res == nil {
		res = []shared.JobSummary{}
	}
	c.JSON(http.StatusOK, res)
}

// getJob: GET /client/jobs/:jobId -> shared.JobDetail
func (a *API) getJob(c *gin.Context) {
	id, ok := uuidParam(c, "jobId")
	if !ok {
		return
	}
	res, err := a.Jobs.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// cancelJob: POST /client/jobs/:jobId/cancel -> shared.JobSummary
func (a *API) cancelJob(c *gin.Context) {
	id, ok := uuidParam(c, "jobId")
	if !ok {
		return
	}
	res, err := a.Jobs.Cancel(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// streamJobLogs: GET /client/jobs/:jobId/logs/stream -> text/event-stream.
// Same wire format as the legacy /jobs/:id/logs/stream: "data:" lines with
// log chunks, then an "event: done" when the job ends.
func (a *API) streamJobLogs(c *gin.Context) {
	writeError(c, shared.NotImplemented("jobs log stream"))
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
