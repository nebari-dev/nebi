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

type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type AddMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
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

// CreateGroup godoc
// @Summary Create a native group (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param group body CreateGroupRequest true "Group details"
// @Success 201 {object} models.Group
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/groups [post]
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	g, err := h.svc.CreateGroup(service.CreateGroupRequest{
		Name:        req.Name,
		Description: req.Description,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, g)
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

// UpdateGroup godoc
// @Summary Update a native group's name or description (admin only)
// @Description OIDC-sourced groups cannot be edited and return 409.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Group ID"
// @Param group body UpdateGroupRequest true "Fields to update"
// @Success 200 {object} models.Group
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/groups/{id} [patch]
func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	g, err := h.svc.UpdateGroup(id, service.UpdateGroupRequest{
		Name:        req.Name,
		Description: req.Description,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

// DeleteGroup godoc
// @Summary Soft-delete a native group (admin only)
// @Description Removes all Casbin role bindings and group permissions. OIDC-sourced groups cannot be deleted and return 409.
// @Tags admin
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/groups/{id} [delete]
func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := h.svc.DeleteGroup(id, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AddMember godoc
// @Summary Add a user to a native group (admin only)
// @Description OIDC-sourced groups are managed via login claims and reject manual membership edits.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Param id path string true "Group ID"
// @Param member body AddMemberRequest true "User to add"
// @Success 201
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/groups/{id}/members [post]
func (h *GroupHandler) AddMember(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.AddMember(groupID, req.UserID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// RemoveMember godoc
// @Summary Remove a user from a native group (admin only)
// @Tags admin
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Param user_id path string true "User ID to remove"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/groups/{id}/members/{user_id} [delete]
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}
	if err := h.svc.RemoveMember(groupID, userID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
