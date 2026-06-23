package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/agentsmesh/backend/internal/config"
	"github.com/anthropics/agentsmesh/backend/internal/domain/user"
	"github.com/anthropics/agentsmesh/backend/internal/infra"
	userSvc "github.com/anthropics/agentsmesh/backend/internal/service/user"
	"github.com/anthropics/agentsmesh/backend/internal/testkit"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newGithubStubServer(t *testing.T, externalID string) *httptest.Server {
	t.Helper()
	repoPath := "/repos/" + externalID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case repoPath + "/branches":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"abc123"},"protected":false}]`))
		case repoPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":1,"name":"repo","full_name":"` + externalID + `","default_branch":"main"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newGithubStubServer5xx(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal server error"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func seedProviderCredential(t *testing.T, db *gorm.DB, userID int64, providerType, baseURL, rawToken string) {
	t.Helper()
	result := db.Create(&user.RepositoryProvider{
		UserID:            userID,
		ProviderType:      providerType,
		Name:              "default-" + providerType,
		BaseURL:           baseURL,
		BotTokenEncrypted: &rawToken,
		IsDefault:         true,
		IsActive:          true,
	})
	require.NoError(t, result.Error)
}

func wireService(t *testing.T, db *gorm.DB) *Service {
	t.Helper()
	repoSvc := NewService(infra.NewGitProviderRepository(db))
	us := userSvc.NewService(infra.NewUserRepository(db))
	ws := NewWebhookService(infra.NewGitProviderRepository(db), &config.Config{}, us, nil)
	repoSvc.SetWebhookService(ws)
	return repoSvc
}

func createTestRepo(t *testing.T, svc *Service, providerType, baseURL, externalID string) int64 {
	t.Helper()
	repo, err := svc.Create(context.Background(), &CreateRequest{
		OrganizationID:  1,
		ProviderType:    providerType,
		ProviderBaseURL: baseURL,
		ExternalID:      externalID,
		Name:            "repo",
		Slug:            strings.ReplaceAll(externalID, "/", "-"),
		Visibility:      "organization",
	})
	require.NoError(t, err)
	return repo.ID
}

func TestListBranchesForUser_Integration(t *testing.T) {
	const externalID = "owner/repo"

	t.Run("seeded default credential lists branches via real provider client", func(t *testing.T) {
		db := testkit.SetupTestDB(t)
		srv := newGithubStubServer(t, externalID)

		svc := wireService(t, db)
		repoID := createTestRepo(t, svc, "github", srv.URL, externalID)

		const userID = int64(7)
		seedProviderCredential(t, db, userID, "github", srv.URL, "seeded-token")

		got, err := svc.ListBranchesForUser(context.Background(), repoID, userID, "")
		require.NoError(t, err)
		require.Equal(t, []string{"main"}, got)
	})

	t.Run("user with no credential returns ErrNoGitCredential", func(t *testing.T) {
		db := testkit.SetupTestDB(t)
		srv := newGithubStubServer(t, externalID)

		svc := wireService(t, db)
		repoID := createTestRepo(t, svc, "github", srv.URL, externalID)

		_, err := svc.ListBranchesForUser(context.Background(), repoID, 999, "")
		require.ErrorIs(t, err, ErrNoGitCredential)
	})

	t.Run("provider 5xx propagates as ErrListBranchesProvider sentinel", func(t *testing.T) {
		db := testkit.SetupTestDB(t)
		srv5xx := newGithubStubServer5xx(t)

		svc := wireService(t, db)
		repoID := createTestRepo(t, svc, "github", srv5xx.URL, externalID)

		const userID = int64(8)
		seedProviderCredential(t, db, userID, "github", srv5xx.URL, "any-token")

		_, err := svc.ListBranchesForUser(context.Background(), repoID, userID, "")
		require.ErrorIs(t, err, ErrListBranchesProvider)
	})
}
