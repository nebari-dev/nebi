package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/service"
)

func TestHandleServiceError_StatusAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "ErrNotFound keeps its generic body",
			err:        fmt.Errorf("load: %w", service.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"Not found"}`,
		},
		{
			name:       "NotFoundError carries its message",
			err:        &service.NotFoundError{Message: "repository or tag not found: quay.io/org/repo:v1"},
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"repository or tag not found: quay.io/org/repo:v1"}`,
		},
		{
			name:       "ValidationError",
			err:        &service.ValidationError{Message: "Invalid registry ID"},
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"Invalid registry ID"}`,
		},
		{
			name:       "ConflictError",
			err:        &service.ConflictError{Message: "already exists"},
			wantStatus: http.StatusConflict,
			wantBody:   `{"error":"already exists"}`,
		},
		{
			name:       "ForbiddenError",
			err:        &service.ForbiddenError{Message: "no access"},
			wantStatus: http.StatusForbidden,
			wantBody:   `{"error":"no access"}`,
		},
		{
			name:       "UnprocessableError: not a Nebi artifact",
			err:        &service.UnprocessableError{Message: "not a Nebi artifact"},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"error":"not a Nebi artifact"}`,
		},
		{
			name:       "UnprocessableError: invalid bundle",
			err:        &service.UnprocessableError{Message: "invalid bundle: missing pixi.{toml,lock}"},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"error":"invalid bundle: missing pixi.{toml,lock}"}`,
		},
		{
			name: "UpstreamError reports the upstream status",
			err: &service.UpstreamError{
				Message:        "registry refused access to quay.io/org/repo",
				UpstreamStatus: http.StatusUnauthorized,
				Op:             "pull bundle",
				Target:         "quay.io/org/repo",
			},
			wantStatus: http.StatusBadGateway,
			wantBody:   `{"error":"registry refused access to quay.io/org/repo","upstream_status":401}`,
		},
		{
			name:       "anything else is an opaque 500",
			err:        errors.New("pull bundle: dial tcp 10.0.0.1:443: connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"Internal server error"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			handleServiceError(c, tc.err)

			if w.Code != tc.wantStatus {
				t.Errorf("status: got %d want %d", w.Code, tc.wantStatus)
			}
			if got := strings.TrimSpace(w.Body.String()); got != tc.wantBody {
				t.Errorf("body: got %s want %s", got, tc.wantBody)
			}
		})
	}
}
