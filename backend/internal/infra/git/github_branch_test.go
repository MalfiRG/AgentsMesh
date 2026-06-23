package git

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGitHubListBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		callCount := 0
		server, provider := setupGitHubMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if r.URL.Path == "/repos/owner/repo/branches" {
				json.NewEncoder(w).Encode([]map[string]interface{}{
					{
						"name": "main",
						"commit": map[string]interface{}{
							"sha": "abc123",
						},
						"protected": true,
					},
					{
						"name": "develop",
						"commit": map[string]interface{}{
							"sha": "def456",
						},
						"protected": false,
					},
				})
			} else if r.URL.Path == "/repos/owner/repo" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"default_branch": "main",
					"created_at":     time.Now().Format(time.RFC3339),
					"updated_at":     time.Now().Format(time.RFC3339),
				})
			}
		})
		defer server.Close()

		branches, err := provider.ListBranches(ctx, "owner/repo")
		if err != nil {
			t.Fatalf("ListBranches failed: %v", err)
		}
		if len(branches) != 2 {
			t.Errorf("len(branches) = %d, want 2", len(branches))
		}
		// Check default branch
		for _, b := range branches {
			if b.Name == "main" && !b.Default {
				t.Error("main branch should be default")
			}
		}
	})
}

func TestGitHubGetBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		server, provider := setupGitHubMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/repos/owner/repo/branches/main" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"name": "main",
					"commit": map[string]interface{}{
						"sha": "abc123",
					},
					"protected": true,
				})
			} else if r.URL.Path == "/repos/owner/repo" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"default_branch": "main",
					"created_at":     time.Now().Format(time.RFC3339),
					"updated_at":     time.Now().Format(time.RFC3339),
				})
			}
		})
		defer server.Close()

		branch, err := provider.GetBranch(ctx, "owner/repo", "main")
		if err != nil {
			t.Fatalf("GetBranch failed: %v", err)
		}
		if branch.Name != "main" {
			t.Errorf("branch.Name = %s, want main", branch.Name)
		}
		if !branch.Protected {
			t.Error("branch should be protected")
		}
	})

	t.Run("not found", func(t *testing.T) {
		server, provider := setupGitHubMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		defer server.Close()

		_, err := provider.GetBranch(ctx, "owner/repo", "nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestGitHubCreateBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		server, provider := setupGitHubMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" && r.URL.Path == "/repos/owner/repo/git/refs/heads/main" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ref": "refs/heads/main",
					"object": map[string]interface{}{
						"sha": "abc123",
					},
				})
			} else if r.Method == "POST" && r.URL.Path == "/repos/owner/repo/git/refs" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ref": "refs/heads/feature",
					"object": map[string]interface{}{
						"sha": "abc123",
					},
				})
			}
		})
		defer server.Close()

		branch, err := provider.CreateBranch(ctx, "owner/repo", "feature", "main")
		if err != nil {
			t.Fatalf("CreateBranch failed: %v", err)
		}
		if branch.Name != "feature" {
			t.Errorf("branch.Name = %s, want feature", branch.Name)
		}
	})
}

func TestGitHubDeleteBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		server, provider := setupGitHubMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "DELETE" {
				t.Errorf("unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		})
		defer server.Close()

		err := provider.DeleteBranch(ctx, "owner/repo", "feature")
		if err != nil {
			t.Fatalf("DeleteBranch failed: %v", err)
		}
	})
}

func TestGitHubListBranches_Cases(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantErr     error
		wantCount   int
		wantDefault string
	}{
		{name: "empty", status: 200, body: `[]`, wantCount: 0},
		{name: "single", status: 200, body: `[{"name":"main","commit":{"sha":"a"},"protected":false}]`, wantCount: 1},
		{name: "many", status: 200, body: `[{"name":"main","commit":{"sha":"a"}},{"name":"dev","commit":{"sha":"b"}}]`, wantCount: 2},
		{name: "unauthorized_401", status: 401, body: `{}`, wantErr: ErrUnauthorized},
		{name: "notfound_404", status: 404, body: `{}`, wantErr: ErrNotFound},
		// GAP lock: github_client.go:70-72 maps every 403 to ErrRateLimited (scope 403 conflated with rate-limit)
		{name: "forbidden_403_maps_ratelimit", status: 403, body: `{}`, wantErr: ErrRateLimited},
		// GAP lock: GitHub has no explicit 429 branch; falls through to opaque fmt.Errorf
		{name: "ratelimit_429_is_opaque", status: 429, body: `{}`},
		{name: "malformed_json", status: 200, body: `not json`},
		{name: "slashy_unicode_names", status: 200, body: `[{"name":"feat/ünïcode/deep","commit":{"sha":"c"}}]`, wantCount: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/branches") {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				// GetProject (default-branch derivation) - return a minimal project
				w.WriteHeader(200)
				_, _ = w.Write([]byte(`{"default_branch":"main"}`))
			}))
			defer srv.Close()
			p, err := NewGitHubProvider(srv.URL, "tok")
			if err != nil {
				t.Fatalf("failed to create provider: %v", err)
			}
			branches, err := p.ListBranches(context.Background(), "owner/repo")
			switch {
			case tc.name == "ratelimit_429_is_opaque":
				if err == nil {
					t.Fatal("expected error for 429, got nil")
				}
				// GAP lock: GitHub has no 429 branch; 429 falls through to opaque error, NOT ErrRateLimited
				if errors.Is(err, ErrRateLimited) {
					t.Fatalf("GAP violated: GitHub 429 should not map to ErrRateLimited, got %v", err)
				}
			case tc.name == "malformed_json":
				if err == nil {
					t.Fatal("expected error for malformed JSON, got nil")
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err %v, want %v", err, tc.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(branches) != tc.wantCount {
					t.Fatalf("got %d branches, want %d", len(branches), tc.wantCount)
				}
			}
		})
	}
}
