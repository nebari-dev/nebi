package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/service"
)

// handleServiceError maps service-layer errors to HTTP status codes.
func handleServiceError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not found"})
		return
	}
	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: validationErr.Message})
		return
	}
	var conflictErr *service.ConflictError
	if errors.As(err, &conflictErr) {
		c.JSON(http.StatusConflict, ErrorResponse{Error: conflictErr.Message})
		return
	}
	var forbiddenErr *service.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: forbiddenErr.Message})
		return
	}
	slog.Error("unhandled service error", "error", err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
}

func handleBindError(c *gin.Context, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: fmt.Sprintf("request body exceeds %d bytes", maxBytesErr.Limit),
		})
		return
	}
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
}

// getUserID extracts the user ID from the Gin context.
func getUserID(c *gin.Context) uuid.UUID {
	user, exists := c.Get("user")
	if !exists {
		return uuid.Nil
	}
	return user.(*models.User).ID
}
