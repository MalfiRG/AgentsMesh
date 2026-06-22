package repositoryconnect

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anthropics/agentsmesh/backend/internal/domain/gitprovider"
	repositoryservice "github.com/anthropics/agentsmesh/backend/internal/service/repository"
	repositoryv1 "github.com/anthropics/agentsmesh/proto/gen/go/repository/v1"
)

func TestMapServiceError_NoGitCredential_FailedPrecondition(t *testing.T) {
	got := connectCodeOf(t, mapServiceError(repositoryservice.ErrNoGitCredential))
	assert.Equal(t, connect.CodeFailedPrecondition, got)
}

func TestListRepositoryBranches_MissingOrgSlug_InvalidArgument(t *testing.T) {
	svc := &fakeRepoService{branches: []string{"main"}}
	srv := NewServer(svc, fakeOrgSvc())
	_, err := srv.ListRepositoryBranches(ctxAsUser(42), connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCodeOf(t, err))
}

func TestListRepositoryBranches_NoAuth_Unauthenticated(t *testing.T) {
	svc := &fakeRepoService{branches: []string{"main"}}
	srv := NewServer(svc, fakeOrgSvc())
	_, err := srv.ListRepositoryBranches(context.Background(), connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{OrgSlug: "acme"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
}

func TestListRepositoryBranches_NonMember_PermissionDenied(t *testing.T) {
	noMemberOrg := &fakeOrgService{role: "admin"}
	noMemberOrg.noMember = true
	svc := &fakeRepoService{branches: []string{"main"}}
	srv := NewServer(svc, noMemberOrg)
	_, err := srv.ListRepositoryBranches(ctxAsUser(42), connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{OrgSlug: "acme", Id: 1}))
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connectCodeOf(t, err))
}

func TestListRepositoryBranches_EmptyToken_ResolvesServerSide(t *testing.T) {
	svc := &fakeRepoService{
		branches: []string{"main", "dev"},
		getByID:  func(_ context.Context, _ int64) (*gitprovider.Repository, error) { return orgRepo(), nil },
	}
	srv := NewServer(svc, fakeOrgSvc())
	resp, err := srv.ListRepositoryBranches(ctxAsUser(42), connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{
		OrgSlug: "acme", Id: 1, AccessToken: "",
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"main", "dev"}, namesOf(resp.Msg.Items))
	require.Equal(t, int64(42), svc.lastUserID)
}

func TestListRepositoryBranches_NoCredential_FailedPrecondition(t *testing.T) {
	svc := &fakeRepoService{
		err:     repositoryservice.ErrNoGitCredential,
		getByID: func(_ context.Context, _ int64) (*gitprovider.Repository, error) { return orgRepo(), nil },
	}
	srv := NewServer(svc, fakeOrgSvc())
	_, err := srv.ListRepositoryBranches(ctxAsUser(42), connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{OrgSlug: "acme", Id: 1}))
	require.Equal(t, connect.CodeFailedPrecondition, connectCodeOf(t, err))
}

func TestListRepositoryBranches_ProviderError_SurfacesNoToken(t *testing.T) {
	svc := &fakeRepoService{
		err:     repositoryservice.ErrListBranchesProvider,
		getByID: func(_ context.Context, _ int64) (*gitprovider.Repository, error) { return orgRepo(), nil },
	}
	srv := NewServer(svc, fakeOrgSvc())
	const tok = "super-secret-token"
	_, err := srv.ListRepositoryBranches(ctxAsUser(42), connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{
		OrgSlug: "acme", Id: 1, AccessToken: tok,
	}))
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, tok)
	assert.NotContains(t, msg, "access_token")
	assert.NotContains(t, msg, "https://")
}
