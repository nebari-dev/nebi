package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/service"
)

type GroupHandler struct {
	svc *service.GroupService
}

func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

// ListGroups godoc
// @Summary List all groups with member counts (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} service.GroupWithMemberCount
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/groups [get]
func (h *GroupHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroups()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, groups)
}

// GetGroup godoc
// @Summary Get a group by ID with member count (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path string true "Group ID"
// @Success 200 {object} service.GroupWithMemberCount
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/groups/{id} [get]
func (h *GroupHandler) GetGroup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	g, err := h.svc.GetGroup(id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

// ListMembers godoc
// @Summary List all members of a group (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path string true "Group ID"
// @Success 200 {array} models.GroupMember
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/groups/{id}/members [get]
func (h *GroupHandler) ListMembers(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	members, err := h.svc.ListMembers(groupID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

// MyGroups godoc
// @Summary List the caller's group memberships
// @Description Used by the ShareDialog picker to populate group options for the current user.
// @Tags groups
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Group
// @Failure 401 {object} ErrorResponse
// @Router /groups/me [get]
func (h *GroupHandler) MyGroups(c *gin.Context) {
	uid := getUserID(c)
	if uid == uuid.Nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return
	}
	groups, err := h.svc.ListGroupsForUser(uid)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, groups)
}
