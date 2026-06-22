package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/agentsmesh/backend/internal/config"
	"github.com/anthropics/agentsmesh/backend/internal/domain/user"
	"github.com/anthropics/agentsmesh/backend/internal/infra"
	"github.com/anthropics/agentsmesh/backend/internal/infra/git"
	userSvc "github.com/anthropics/agentsmesh/backend/internal/service/user"
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

	t.Run("construction error is sanitized (M1)", func(t *testing.T) {
		s, db := setupTestService(t)
		seedRepo(t, db, "github", "https://github.com", "owner/repo")
		constructErr := errors.New("connection refused to https://github.com?token=SECRET-TOK")
		s.SetProviderFactory(func(_, _, _ string) (git.Provider, error) {
			return nil, constructErr
		})
		_, err := s.ListBranchesForUser(ctx, 1, 42, "SECRET-TOK")
		require.ErrorIs(t, err, ErrListBranchesProvider)
		require.NotContains(t, err.Error(), "SECRET-TOK")
	})

	t.Run("default credential preferred over arbitrary same-host one (X3)", func(t *testing.T) {
		s, db := setupTestService(t)
		seedRepo(t, db, "github", "https://github.com", "owner/repo")

		nonDefaultTok := "token-non-default"
		defaultTok := "token-default"
		userID := int64(99)

		db.Create(&user.RepositoryProvider{
			UserID:            userID,
			ProviderType:      "github",
			Name:              "non-default",
			BaseURL:           "https://github.com",
			BotTokenEncrypted: &nonDefaultTok,
			IsDefault:         false,
			IsActive:          true,
		})
		db.Create(&user.RepositoryProvider{
			UserID:            userID,
			ProviderType:      "github",
			Name:              "default",
			BaseURL:           "https://github.com",
			BotTokenEncrypted: &defaultTok,
			IsDefault:         true,
			IsActive:          true,
		})

		us := userSvc.NewService(infra.NewUserRepository(db))
		ws := NewWebhookService(infra.NewGitProviderRepository(db), &config.Config{}, us, nil)
		s.SetWebhookService(ws)

		var factoryToken string
		s.SetProviderFactory(func(_, _, token string) (git.Provider, error) {
			factoryToken = token
			return &fakeProvider{branches: []*git.Branch{{Name: "main", Default: true}}}, nil
		})

		_, err := s.ListBranchesForUser(ctx, 1, userID, "")
		require.NoError(t, err)
		require.Equal(t, defaultTok, factoryToken)
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
