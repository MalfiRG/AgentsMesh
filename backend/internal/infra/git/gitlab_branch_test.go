package git

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitLabBranchOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("list branches", func(t *testing.T) {
		server, provider := setupGitLabMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"name": "main",
					"commit": map[string]interface{}{
						"id": "abc123",
					},
					"protected": true,
					"default":   true,
				},
				{
					"name": "develop",
					"commit": map[string]interface{}{
						"id": "def456",
					},
					"protected": false,
					"default":   false,
				},
			})
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
		server, provider := setupGitLabMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "main",
				"commit": map[string]interface{}{
					"id": "abc123",
				},
				"protected": true,
				"default":   true,
			})
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

	t.Run("get branch not found", func(t *testing.T) {
		server, provider := setupGitLabMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		defer server.Close()

		_, err := provider.GetBranch(ctx, "owner/repo", "nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("create branch", func(t *testing.T) {
		server, provider := setupGitLabMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "feature",
				"commit": map[string]interface{}{
					"id": "abc123",
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
		server, provider := setupGitLabMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		defer server.Close()

		err := provider.DeleteBranch(ctx, "owner/repo", "feature")
		if err != nil {
			t.Fatalf("DeleteBranch failed: %v", err)
		}
	})
}

func TestGitLabListBranches_Cases(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantErr   error
		wantCount int
	}{
		{name: "empty", status: 200, body: `[]`, wantCount: 0},
		{name: "single", status: 200, body: `[{"name":"main","commit":{"id":"a"},"protected":false,"default":true}]`, wantCount: 1},
		{name: "many", status: 200, body: `[{"name":"main","commit":{"id":"a"},"default":true},{"name":"dev","commit":{"id":"b"},"default":false}]`, wantCount: 2},
		{name: "unauthorized_401", status: 401, body: `{}`, wantErr: ErrUnauthorized},
		{name: "notfound_404", status: 404, body: `{}`, wantErr: ErrNotFound},
		// GitLab maps 429 -> ErrRateLimited (gitlab_client.go:62-64)
		{name: "ratelimit_429_maps_ratelimited", status: 429, body: `{}`, wantErr: ErrRateLimited},
		// GAP lock: GitLab 403 falls through to opaque fmt.Errorf, NOT ErrRateLimited
		{name: "forbidden_403_is_opaque", status: 403, body: `{}`},
		{name: "malformed_json", status: 200, body: `not json`},
		// GitLab default flag comes from payload, not a separate GetProject call
		{name: "default_flag_honored", status: 200, body: `[{"name":"main","commit":{"id":"a"},"default":true},{"name":"dev","commit":{"id":"b"},"default":false}]`, wantCount: 2},
		{name: "slashy_unicode_names", status: 200, body: `[{"name":"feat/ünïcode/deep","commit":{"id":"c"}}]`, wantCount: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/branches") {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				w.WriteHeader(200)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			p, err := NewGitLabProvider(srv.URL, "tok")
			if err != nil {
				t.Fatalf("failed to create provider: %v", err)
			}
			branches, err := p.ListBranches(context.Background(), "owner/repo")
			switch {
			case tc.name == "forbidden_403_is_opaque":
				if err == nil {
					t.Fatal("expected error for 403, got nil")
				}
				// GAP lock: GitLab 403 falls through to opaque error, NOT ErrRateLimited
				if errors.Is(err, ErrRateLimited) {
					t.Fatalf("GAP violated: GitLab 403 should not map to ErrRateLimited, got %v", err)
				}
			case tc.name == "malformed_json":
				if err == nil {
					t.Fatal("expected error for malformed JSON, got nil")
				}
			case tc.name == "default_flag_honored":
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var defaultCount int
				for _, b := range branches {
					if b.Default {
						defaultCount++
					}
				}
				if defaultCount != 1 {
					t.Fatalf("expected exactly 1 default branch, got %d", defaultCount)
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
