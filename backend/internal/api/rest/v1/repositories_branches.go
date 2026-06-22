package v1

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/anthropics/agentsmesh/backend/internal/middleware"
	repositoryservice "github.com/anthropics/agentsmesh/backend/internal/service/repository"
	"github.com/anthropics/agentsmesh/backend/pkg/apierr"
	"github.com/anthropics/agentsmesh/backend/pkg/policy"
	"github.com/gin-gonic/gin"
)

// ListBranches lists repository branches
// GET /api/v1/organizations/:slug/repositories/:id/branches
func (h *RepositoryHandler) ListBranches(c *gin.Context) {
	repoID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apierr.InvalidInput(c, "Invalid repository ID")
		return
	}

	repo, err := h.repositoryService.GetByID(c.Request.Context(), repoID)
	if err != nil {
		apierr.ResourceNotFound(c, "Repository not found")
		return
	}

	tenant := middleware.GetTenant(c)
	sub := policy.NewSubject(tenant.OrganizationID, tenant.UserID, tenant.UserRole)
	if !policy.RepositoryPolicy.AllowRead(sub, h.repoResourceWithGrants(
		c.Request.Context(), repoID, repo.OrganizationID, repo.ImportedByUserID, repo.Visibility,
	)) {
		apierr.ForbiddenAccess(c)
		return
	}

	accessToken := c.Query("access_token")
	if accessToken == "" {
		accessToken = c.GetHeader("X-Git-Access-Token")
	}

	// Connect uses CodeFailedPrecondition; REST uses 409 Conflict — the
	// frontend keys its fallback off error-existence, not the specific code.
	branches, err := h.repositoryService.ListBranchesForUser(c.Request.Context(), repoID, tenant.UserID, accessToken)
	if err != nil {
		if errors.Is(err, repositoryservice.ErrNoGitCredential) {
			apierr.Conflict(c, apierr.MISSING_REQUIRED, "No git credential available to list branches")
			return
		}
		apierr.InternalError(c, "Failed to list branches")
		return
	}

	c.JSON(http.StatusOK, gin.H{"branches": branches})
}
