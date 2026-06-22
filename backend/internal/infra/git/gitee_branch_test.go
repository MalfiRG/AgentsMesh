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

func TestGiteeBranchOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("list branches", func(t *testing.T) {
		callCount := 0
		server, provider := setupGiteeMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				// List branches
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
			} else {
				// Get project for default branch
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
	})

	t.Run("get branch", func(t *testing.T) {
		callCount := 0
		server, provider := setupGiteeMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"name": "main",
					"commit": map[string]interface{}{
						"sha": "abc123",
					},
					"protected": true,
				})
			} else {
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
	})

	t.Run("create branch", func(t *testing.T) {
		server, provider := setupGiteeMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "feature",
				"commit": map[string]interface{}{
					"sha": "abc123",
				},
			})
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

	t.Run("delete branch", func(t *testing.T) {
		server, provider := setupGiteeMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		defer server.Close()

		err := provider.DeleteBranch(ctx, "owner/repo", "feature")
		if err != nil {
			t.Fatalf("DeleteBranch failed: %v", err)
		}
	})
}

func TestGiteeListBranches_Cases(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantErr   error
		wantCount int
	}{
		{name: "empty", status: 200, body: `[]`, wantCount: 0},
		{name: "single", status: 200, body: `[{"name":"main","commit":{"sha":"a"},"protected":false}]`, wantCount: 1},
		{name: "many", status: 200, body: `[{"name":"main","commit":{"sha":"a"}},{"name":"dev","commit":{"sha":"b"}}]`, wantCount: 2},
		{name: "unauthorized_401", status: 401, body: `{}`, wantErr: ErrUnauthorized},
		{name: "notfound_404", status: 404, body: `{}`, wantErr: ErrNotFound},
		// Gitee maps 403 -> ErrRateLimited (gitee_client.go:67-69), same conflation pattern as GitHub
		{name: "forbidden_403_maps_ratelimit", status: 403, body: `{}`, wantErr: ErrRateLimited},
		// GAP lock: Gitee has no explicit 429 branch; falls through to opaque fmt.Errorf
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
				_, _ = w.Write([]byte(`{"default_branch":"main","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`))
			}))
			defer srv.Close()
			p, err := NewGiteeProvider(srv.URL, "tok")
			if err != nil {
				t.Fatalf("failed to create provider: %v", err)
			}
			branches, err := p.ListBranches(context.Background(), "owner/repo")
			switch {
			case tc.name == "ratelimit_429_is_opaque":
				if err == nil {
					t.Fatal("expected error for 429, got nil")
				}
				// GAP lock: Gitee has no 429 branch; 429 falls through to opaque error, NOT ErrRateLimited
				if errors.Is(err, ErrRateLimited) {
					t.Fatalf("GAP violated: Gitee 429 should not map to ErrRateLimited, got %v", err)
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
