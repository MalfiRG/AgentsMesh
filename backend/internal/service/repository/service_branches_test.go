package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/agentsmesh/backend/internal/infra"
	"github.com/anthropics/agentsmesh/backend/internal/infra/git"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedRepo(t *testing.T, db *gorm.DB, providerType, baseURL, externalID string) {
	t.Helper()
	svc := NewService(infra.NewGitProviderRepository(db))
	_, err := svc.Create(context.Background(), &CreateRequest{
		OrganizationID:  1,
		ProviderType:    providerType,
		ProviderBaseURL: baseURL,
		ExternalID:      externalID,
		Name:            "repo",
		Slug:            externalID,
		Visibility:      "organization",
	})
	require.NoError(t, err)
}

func TestListBranchesForUser(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit token bypasses resolution and lists", func(t *testing.T) {
		s, db := setupTestService(t)
		seedRepo(t, db, "github", "https://github.com", "owner/repo")
		var factoryToken string
		s.SetProviderFactory(func(_, _, token string) (git.Provider, error) {
			factoryToken = token
			return &fakeProvider{branches: []*git.Branch{{Name: "main", Default: true}}}, nil
		})
		got, err := s.ListBranchesForUser(ctx, 1, 42, "explicit-tok")
		require.NoError(t, err)
		require.Equal(t, []string{"main"}, got)
		require.Equal(t, "explicit-tok", factoryToken)
	})

	t.Run("empty token with no credential returns ErrNoGitCredential", func(t *testing.T) {
		s, db := setupTestService(t)
		seedRepo(t, db, "github", "https://github.com", "owner/repo")
		_, err := s.ListBranchesForUser(ctx, 1, 42, "")
		require.ErrorIs(t, err, ErrNoGitCredential)
	})

	t.Run("provider error is sanitized and never leaks the token (X4)", func(t *testing.T) {
		s, db := setupTestService(t)
		seedRepo(t, db, "github", "https://github.com", "owner/repo")
		leaky := errors.New("Get \"https://gitee.com/api/v5/repos/owner/repo/branches?access_token=SECRET-TOK\": dial tcp: timeout")
		s.SetProviderFactory(func(_, _, _ string) (git.Provider, error) {
			return &fakeProvider{err: leaky}, nil
		})
		_, err := s.ListBranchesForUser(ctx, 1, 42, "SECRET-TOK")
		require.ErrorIs(t, err, ErrListBranchesProvider)
		require.NotContains(t, err.Error(), "SECRET-TOK")
		require.NotContains(t, err.Error(), "access_token")
		require.NotContains(t, err.Error(), "gitee.com")
	})

	t.Run("unsupported provider degrades to ErrNoGitCredential", func(t *testing.T) {
		s, db := setupTestService(t)
		seedRepo(t, db, "ssh", "ssh://git@host", "owner/repo")
		_, err := s.ListBranchesForUser(ctx, 1, 42, "tok")
		require.ErrorIs(t, err, ErrNoGitCredential)
	})
}

type fakeProvider struct {
	git.Provider
	branches []*git.Branch
	err      error
}

func (f *fakeProvider) ListBranches(_ context.Context, _ string) ([]*git.Branch, error) {
	return f.branches, f.err
}
