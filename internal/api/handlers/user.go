package handlers

import (
	"net/http"

	"github.com/aktech/darb/internal/auth"
	"github.com/gin-gonic/gin"
)

// GetCurrentUser godoc
// @Summary Get current user
// @Description Get the currently authenticated user's information
// @Tags auth
// @Produce json
// @Success 200 {object} models.User
// @Failure 401 {object} map[string]string
// @Router /auth/me [get]
func GetCurrentUser(authenticator auth.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := authenticator.GetUserFromContext(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.JSON(http.StatusOK, user)
	}
}
