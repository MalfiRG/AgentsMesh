package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anthropics/agentsmesh/backend/internal/domain/gitprovider"
	"github.com/anthropics/agentsmesh/backend/internal/middleware"
	repositoryservice "github.com/anthropics/agentsmesh/backend/internal/service/repository"
)

// branchRepoSvc stubs RepositoryServiceInterface for branch handler tests.
type branchRepoSvc struct {
	repositoryservice.RepositoryServiceInterface
	getByIDErr  error
	branchesErr error
	branches    []string
	lastUserID  int64
}

func (s *branchRepoSvc) GetByID(_ context.Context, _ int64) (*gitprovider.Repository, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	return &gitprovider.Repository{ID: 1, OrganizationID: 7, Visibility: "organization"}, nil
}

func (s *branchRepoSvc) ListBranchesForUser(
	_ context.Context, _ int64, userID int64, _ string,
) ([]string, error) {
	s.lastUserID = userID
	return s.branches, s.branchesErr
}

func setBranchTenantContext(c *gin.Context, orgID, userID int64) {
	tc := &middleware.TenantContext{
		OrganizationID:   orgID,
		OrganizationSlug: "acme",
		UserID:           userID,
		UserRole:         "admin",
	}
	c.Set("tenant", tc)
}

func parseBranchErrResp(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestREST_ListBranches_NoCredential_Returns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &branchRepoSvc{branchesErr: repositoryservice.ErrNoGitCredential}
	h := NewRepositoryHandler(svc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/repositories/1/branches", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	setBranchTenantContext(c, 7, 42)

	h.ListBranches(c)
	require.Equal(t, http.StatusConflict, w.Code)
	resp := parseBranchErrResp(t, w)
	assert.Equal(t, "MISSING_REQUIRED", resp["code"])
	assert.Equal(t, int64(42), svc.lastUserID)
}
