# Branch Dropdown + Ticket-View Pod Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the free-text branch field in the Create Pod form with a lazily-fetched editable branch combobox, and make the ticket-view pod list refresh when a pod is created (same-tab and across windows).

**Architecture:** Backend resolves the git access token server-side from the caller's default credential (the branch path is non-functional today because the token is empty and the handlers reject empty tokens). A new lazy React hook caches branches per repository with a stale-response guard. Feature 2 reuses the existing `invalidateTicketPods` cache primitive and the already-existing `PodCreatedEventData.ticket_slug` wire field.

**Tech Stack:** Go (Gin + Connect-RPC + GORM + testify), git provider HTTP clients, Next.js + React + TypeScript + Vitest, Playwright (`e2e-playwright`).

## Global Constraints

- Every non-test file must stay under 200 lines; test files under 400. Split by SRP before committing if a file you touch crosses the line.
- No comments that restate what code does; only business-constraint / cross-module-contract / non-obvious-workaround comments.
- File names must be specific (no `utils`/`helpers`/`common`).
- ASCII hyphen only in all output; never the long-dash codepoints.
- Backend test command: `bazel test //backend/internal/...` (or a specific package target). Frontend unit: `bazel test //clients/web:unit`; a single Vitest file runs via `pnpm --filter @agentsmesh/web exec vitest run <path>` from repo root (confirm the exact filter name from root `package.json` before first use). E2E: the `e2e-playwright` Playwright project.
- `selectedBranch` form-state stays `string` (`clients/web/src/components/pod/hooks/useCreatePodFormTypes.ts:20,41`). Do not change its type.

---

## Phase A - Backend: branch listing made functional

### Task 1: Lock git-provider branch failure modes (provider layer)

No product change; extends the existing httptest-backed provider tests so the
known gaps (GitHub 429, 403 conflation, no pagination, best-effort default
branch) are regression-locked before the feature relies on them.

**Files:**
- Modify/Test: `backend/internal/infra/git/github_branch_test.go`
- Modify/Test: `backend/internal/infra/git/gitlab_branch_test.go` (create if absent, model on github)
- Modify/Test: `backend/internal/infra/git/gitee_branch_test.go` / `gitee_error_branch_test.go`

**Interfaces:**
- Consumes: `GitHubProvider.ListBranches(ctx, projectID) ([]*git.Branch, error)` (`github_branch.go:11`); `git.Branch{Name, CommitSHA, Protected, Default}` (`provider.go:44`); error sentinels `git.ErrUnauthorized`, `git.ErrNotFound`, `git.ErrRateLimited` (`provider.go:16-19`).
- Produces: nothing consumed downstream; pure regression coverage.

- [ ] **Step 1: Write the failing tests (GitHub)**

In `github_branch_test.go`, add table-driven sub-tests modeled on the existing
`setupGitHubMockServer` + `httptest` pattern already in that file:

```go
func TestGitHubListBranches_Cases(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantErr    error // nil means success
		wantCount  int
		wantDefault string // branch name expected to have Default==true
	}{
		{name: "empty", status: 200, body: `[]`, wantCount: 0},
		{name: "single", status: 200, body: `[{"name":"main","commit":{"sha":"a"},"protected":false}]`, wantCount: 1},
		{name: "many", status: 200, body: `[{"name":"main","commit":{"sha":"a"}},{"name":"dev","commit":{"sha":"b"}}]`, wantCount: 2},
		{name: "unauthorized_401", status: 401, body: `{}`, wantErr: git.ErrUnauthorized},
		{name: "notfound_404", status: 404, body: `{}`, wantErr: git.ErrNotFound},
		{name: "forbidden_403_maps_ratelimit", status: 403, body: `{}`, wantErr: git.ErrRateLimited},
		{name: "ratelimit_429_is_opaque", status: 429, body: `{}`, wantErr: nil /* GAP: see assertion */},
		{name: "malformed_json", status: 200, body: `not json`, wantErr: nil /* decode error: assert err != nil */},
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
			p, err := git.NewProvider(git.ProviderTypeGitHub, srv.URL, "tok")
			require.NoError(t, err)
			branches, err := p.ListBranches(context.Background(), "owner/repo")
			switch {
			case tc.name == "ratelimit_429_is_opaque":
				require.Error(t, err)
				require.NotErrorIs(t, err, git.ErrRateLimited) // GAP lock: GitHub has no 429 branch (github_client.go has no 429 case)
			case tc.name == "malformed_json":
				require.Error(t, err)
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			default:
				require.NoError(t, err)
				require.Len(t, branches, tc.wantCount)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bazel test //backend/internal/infra/git:git_test --test_filter=TestGitHubListBranches_Cases`
Expected: FAIL (new test names absent / a gap assertion not yet matching the client's actual mapping).

- [ ] **Step 3: Adjust assertions to the client's real behavior**

Read `backend/internal/infra/git/github_client.go:62-79` to confirm the exact
status->error mapping; tune each `wantErr` so the test asserts the CURRENT
behavior (locking the gap), not the desired one. Do NOT change product code in
this task.

- [ ] **Step 4: Add GitLab + Gitee parity tests**

In `gitlab_branch_test.go`: same table, but `429 -> git.ErrRateLimited`
(`gitlab_client.go:62-64`) and assert GitLab's `default` flag from the payload
is honored (`gitlab_branch.go`). In `gitee_branch_test.go`: `403 -> git.ErrRateLimited`
(`gitee_client.go:67-69`). Reuse the existing `gitee_error_branch_test.go`
malformed-JSON case.

- [ ] **Step 5: Run all three to verify pass**

Run: `bazel test //backend/internal/infra/git:git_test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/infra/git/*branch_test.go
git commit -m "test(git): lock ListBranches failure modes across github/gitlab/gitee"
```

---

### Task 2: Service-layer token resolution + provider seam (S1 + S2)

**Files:**
- Modify: `backend/internal/service/repository/service.go` (add provider factory field + `userService`-backed resolution entry)
- Modify: `backend/internal/service/repository/service_sync.go:46` (`ListBranches` -> use factory) and add `ListBranchesForUser`
- Modify: `backend/internal/service/repository/webhook_registration.go` (extract `ResolveAccessToken`)
- Modify: `backend/internal/service/repository/interfaces.go:9` (add `ListBranchesForUser` to `RepositoryServiceInterface`)
- Create/Test: `backend/internal/service/repository/service_branches_test.go`

**Interfaces:**
- Consumes: `WebhookService` (already held on `Service` as `s.webhookService`, `service.go:18-21`); `git.NewProvider(providerType, baseURL, token) (git.Provider, error)` (`provider.go:193`); `userService.GetDecryptedProviderTokenByTypeAndURL(ctx, userID, providerType, baseURL) (string, error)` (`repository_provider_token.go:36`).
- Produces:
  - `var ErrNoGitCredential = errors.New("no git credential available to list branches")`
  - `func (s *WebhookService) ResolveAccessToken(ctx context.Context, repo *gitprovider.Repository, userID int64) (string, error)`
  - `func (s *Service) ListBranchesForUser(ctx context.Context, repoID, userID int64, explicitToken string) ([]string, error)`
  - settable seam: `func (s *Service) SetProviderFactory(f func(providerType, baseURL, token string) (git.Provider, error))`

- [ ] **Step 1: Write the failing test**

`service_branches_test.go`:

```go
func TestListBranchesForUser(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit token bypasses resolution and lists", func(t *testing.T) {
		s := setupTestService(t) // existing helper, service_setup_test.go:11
		seedRepo(t, s, "github", "https://github.com", "owner/repo")
		var factoryToken string
		s.SetProviderFactory(func(_, _, token string) (git.Provider, error) {
			factoryToken = token
			return &fakeProvider{branches: []*git.Branch{{Name: "main", Default: true}}}, nil
		})
		got, err := s.ListBranchesForUser(ctx, 1, 42, "explicit-tok")
		require.NoError(t, err)
		require.Equal(t, []string{"main"}, got)
		require.Equal(t, "explicit-tok", factoryToken) // resolution not consulted
	})

	t.Run("empty token with no credential returns ErrNoGitCredential", func(t *testing.T) {
		s := setupTestService(t) // webhookService nil or userService returns empty
		seedRepo(t, s, "github", "https://github.com", "owner/repo")
		_, err := s.ListBranchesForUser(ctx, 1, 42, "")
		require.ErrorIs(t, err, repository.ErrNoGitCredential)
	})

	t.Run("provider error propagates", func(t *testing.T) {
		s := setupTestService(t)
		seedRepo(t, s, "github", "https://github.com", "owner/repo")
		s.SetProviderFactory(func(_, _, _ string) (git.Provider, error) {
			return &fakeProvider{err: errors.New("provider 5xx")}, nil
		})
		_, err := s.ListBranchesForUser(ctx, 1, 42, "tok")
		require.ErrorContains(t, err, "provider 5xx")
	})
}

type fakeProvider struct {
	git.Provider
	branches []*git.Branch
	err      error
}
func (f *fakeProvider) ListBranches(context.Context, string) ([]*git.Branch, error) {
	return f.branches, f.err
}
```

`seedRepo` is a small local helper inserting a `gitprovider.Repository` row via
the same DB the service uses (model on existing `service_setup_test.go` /
`repository_integration_test.go` seeding). If a suitable helper already exists,
reuse it.

- [ ] **Step 2: Run to verify it fails**

Run: `bazel test //backend/internal/service/repository:repository_test --test_filter=TestListBranchesForUser`
Expected: FAIL (`SetProviderFactory`, `ListBranchesForUser`, `ErrNoGitCredential` undefined).

- [ ] **Step 3: Implement the seam + resolution**

In `service.go`: add the sentinel and fields.

```go
var ErrNoGitCredential = errors.New("no git credential available to list branches")

type providerFactory func(providerType, baseURL, token string) (git.Provider, error)

// add to Service struct:
//   providerFactory providerFactory
// In NewService default it:
//   s.providerFactory = git.NewProvider

func (s *Service) SetProviderFactory(f providerFactory) { s.providerFactory = f }
```

In `webhook_registration.go`, extract the token-resolution head of
`getGitProviderForUser` into a reusable method, and have the original call it:

```go
func (s *WebhookService) ResolveAccessToken(ctx context.Context, repo *gitprovider.Repository, userID int64) (string, error) {
	if s.userService == nil {
		return "", ErrNoAccessToken
	}
	if tok, err := s.userService.GetDecryptedProviderTokenByTypeAndURL(ctx, userID, repo.ProviderType, repo.ProviderBaseURL); err == nil && tok != "" {
		return tok, nil
	}
	tokens, err := s.userService.GetDecryptedTokens(ctx, userID, repo.ProviderType)
	if err != nil || tokens.AccessToken == "" {
		return "", ErrNoAccessToken
	}
	return tokens.AccessToken, nil
}
```

In `service_sync.go`, add `ListBranchesForUser` and route `ListBranches`
through the factory:

```go
func (s *Service) ListBranchesForUser(ctx context.Context, repoID, userID int64, explicitToken string) ([]string, error) {
	repo, err := s.GetByID(ctx, repoID)
	if err != nil {
		return nil, err
	}
	token := explicitToken
	if token == "" {
		if s.webhookService == nil {
			return nil, ErrNoGitCredential
		}
		token, err = s.webhookService.ResolveAccessToken(ctx, repo, userID)
		if err != nil || token == "" {
			return nil, ErrNoGitCredential
		}
	}
	client, err := s.providerFactory(repo.ProviderType, repo.ProviderBaseURL, token)
	if err != nil {
		return nil, err
	}
	branches, err := client.ListBranches(ctx, repo.ExternalID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	return names, nil
}
```

Update the existing `ListBranches` (`service_sync.go:52`) to call
`s.providerFactory(...)` instead of package-level `git.NewProvider(...)` so the
seam covers both. Add `ListBranchesForUser` to `RepositoryServiceInterface`
(`interfaces.go`).

- [ ] **Step 4: Run to verify pass**

Run: `bazel test //backend/internal/service/repository:repository_test --test_filter=TestListBranchesForUser`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/repository/
git commit -m "feat(repository): resolve git token server-side for ListBranchesForUser (S1+S2)"
```

---

### Task 3: Handlers resolve-then-validate (S5) - Connect + REST

**Files:**
- Modify: `backend/internal/api/connect/repository/repository_branches.go:18-44` (drop empty-token reject; call `ListBranchesForUser`)
- Modify: `backend/internal/api/connect/repository/repository_branches.go:49-75` (`SyncRepositoryBranches` same)
- Modify: `backend/internal/api/connect/repository/repository_mount.go:16` (`mapServiceError`: map `ErrNoGitCredential`)
- Modify: `backend/internal/api/rest/v1/repositories_branches.go:37-50`
- Modify/Test: `backend/internal/api/connect/repository/repository_test.go`

**Interfaces:**
- Consumes: `repoSvc.ListBranchesForUser(ctx, id, userID, token)` (Task 2); `middleware.GetTenant(ctx).UserID` (`repository_branches.go:115`); `repository.ErrNoGitCredential`.
- Produces: empty-token Connect call now reaches the service; `ErrNoGitCredential -> connect.CodeFailedPrecondition` (the frontend treats this code as "use free-text fallback").

- [ ] **Step 1: Write the failing handler test**

Extend `repository_test.go` (copy the guard-test + `fakeRepoService` patterns at
`repository_test.go:19-156`):

```go
func TestListRepositoryBranches_EmptyToken_ResolvesServerSide(t *testing.T) {
	svc := &fakeRepoService{branches: []string{"main", "dev"}}
	srv := NewServer(svc, fakeOrgSvc())
	ctx := ctxAsUser(t, /*userID*/ 42, /*orgMember*/ true)
	resp, err := srv.ListRepositoryBranches(ctx, connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{
		OrgSlug: "acme", Id: 1, AccessToken: "", // empty - must NOT be rejected anymore
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"main", "dev"}, namesOf(resp.Msg.Items))
	require.Equal(t, int64(42), svc.lastUserID) // handler threaded tenant.UserID
}

func TestListRepositoryBranches_NoCredential_FailedPrecondition(t *testing.T) {
	svc := &fakeRepoService{err: repository.ErrNoGitCredential}
	srv := NewServer(svc, fakeOrgSvc())
	ctx := ctxAsUser(t, 42, true)
	_, err := srv.ListRepositoryBranches(ctx, connect.NewRequest(&repositoryv1.ListRepositoryBranchesRequest{OrgSlug: "acme", Id: 1}))
	require.Equal(t, connect.CodeFailedPrecondition, connectCodeOf(err))
}
```

Extend `fakeRepoService` with `ListBranchesForUser` recording `lastUserID` and
returning `branches`/`err`.

- [ ] **Step 2: Run to verify it fails**

Run: `bazel test //backend/internal/api/connect/repository:repository_test --test_filter=TestListRepositoryBranches_`
Expected: FAIL (handler still rejects empty token at `repository_branches.go:28-30`).

- [ ] **Step 3: Implement resolve-then-validate**

In `repository_branches.go`, remove the empty-token early return; replace the
`s.repoSvc.ListBranches(...)` call with:

```go
tenant := middleware.GetTenant(ctx)
branches, err := s.repoSvc.ListBranchesForUser(ctx, req.Msg.GetId(), tenant.UserID, req.Msg.GetAccessToken())
if err != nil {
	return nil, mapServiceError(err)
}
```

Apply the same to `SyncRepositoryBranches`. In `repository_mount.go`
`mapServiceError`, add:

```go
case errors.Is(err, repositoryservice.ErrNoGitCredential):
	return connect.NewError(connect.CodeFailedPrecondition, err)
```

In REST `repositories_branches.go`, drop the `Access token required` 400; when
the query/header token is empty, pass it through to `ListBranchesForUser(c.Request.Context(), repoID, tenant.UserID, accessToken)`; map `ErrNoGitCredential` to a 412/409 JSON error (use the existing `apierr` helper closest to FailedPrecondition).

- [ ] **Step 4: Run to verify pass**

Run: `bazel test //backend/internal/api/connect/repository:repository_test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/connect/repository/ backend/internal/api/rest/v1/repositories_branches.go
git commit -m "feat(repository): resolve-then-validate branch token in Connect+REST handlers (S5)"
```

---

### Task 4: Backend integration test (real service + DB + faked provider HTTP)

**Files:**
- Create/Test: `backend/internal/service/repository/service_branches_integration_test.go`

**Interfaces:**
- Consumes: `testkit.SetupTestDB`, `setupTestService` (existing); a seeded encrypted git credential via the real `user.Service`; an `httptest` server whose URL is stored as the repo's `ProviderBaseURL` (so real `git.NewProvider` hits it).
- Produces: end-to-end assertion that empty-token + seeded credential lists branches; no-credential returns `ErrNoGitCredential`; provider 5xx propagates.

- [ ] **Step 1: Write the failing integration test**

```go
//go:build integration || !unit
func TestListBranchesForUser_Integration(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/branches") {
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"a"}}]`))
			return
		}
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	}))
	defer provider.Close()

	db := testkit.SetupTestDB(t)
	repoSvc, userSvc := wireRealServices(t, db) // real WebhookService(userSvc) + SetWebhookService
	repoID := seedRepoRow(t, db, "github", provider.URL, "owner/repo")
	seedDefaultGitCredential(t, userSvc, /*userID*/ 7, "github", provider.URL, "seeded-token")

	got, err := repoSvc.ListBranchesForUser(context.Background(), repoID, 7, "")
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, got)

	// no-credential user
	_, err = repoSvc.ListBranchesForUser(context.Background(), repoID, 999, "")
	require.ErrorIs(t, err, repository.ErrNoGitCredential)
}
```

Use the real provider factory here (do NOT call `SetProviderFactory`) so the
httptest boundary exercises `git.NewProvider` end to end. Model
`seedDefaultGitCredential` on how `user.Service.CreateGitCredential` +
provider-token storage is tested elsewhere; if encrypted-token seeding is
non-trivial, reuse the closest existing integration seeding helper.

- [ ] **Step 2: Run to verify it fails, then passes after Task 2/3 are in**

Run: `bazel test //backend/internal/service/repository:repository_test --test_filter=TestListBranchesForUser_Integration`
Expected: PASS (Task 2/3 already landed). If the build tag excludes it, run with the integration config the repo uses for `*_integration_test.go`.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/repository/service_branches_integration_test.go
git commit -m "test(repository): integration coverage for server-side branch token resolution"
```

---

## Phase B - Frontend: lazy branch combobox

### Task 5: `useRepositoryBranches` hook (lazy fetch, dedupe, stale-guard S4, fallback)

**Files:**
- Create: `clients/web/src/hooks/useRepositoryBranches.ts`
- Create/Test: `clients/web/src/hooks/__tests__/useRepositoryBranches.test.ts`

**Interfaces:**
- Consumes: `listRepositoryBranches(orgSlug, id, accessToken="") -> Promise<{ items: string[]; total; limit; offset }>` (`@/lib/api/facade/repositoryConnect`); current org slug source used elsewhere in the form (reuse whatever `CreatePodForm` already uses to get `orgSlug`).
- Produces:
  ```ts
  interface UseRepositoryBranchesResult {
    branches: string[];
    loading: boolean;
    fallbackToFreeText: boolean; // true once a fetch has failed for this repoId
    load: () => void;            // idempotent; triggers a fetch if not cached/inflight
    refresh: () => void;         // force re-fetch
  }
  function useRepositoryBranches(repoId: number | null): UseRepositoryBranchesResult
  function __resetRepositoryBranchesCacheForTests(): void
  ```
  Cache keyed by `repoId`; a monotonically increasing per-`repoId` request token drops stale responses (S4).

- [ ] **Step 1: Write the failing tests**

```ts
import { renderHook, act, waitFor } from "@testing-library/react";
import { useRepositoryBranches, __resetRepositoryBranchesCacheForTests } from "../useRepositoryBranches";

vi.mock("@/lib/api/facade/repositoryConnect", () => ({
  listRepositoryBranches: vi.fn(),
}));
import { listRepositoryBranches } from "@/lib/api/facade/repositoryConnect";

beforeEach(() => { __resetRepositoryBranchesCacheForTests(); vi.mocked(listRepositoryBranches).mockReset(); });

test("does not fetch until load() is called (lazy)", () => {
  renderHook(() => useRepositoryBranches(1));
  expect(listRepositoryBranches).not.toHaveBeenCalled();
});

test("load() fetches once and caches per repoId", async () => {
  vi.mocked(listRepositoryBranches).mockResolvedValue({ items: ["main"], total: 1, limit: 100, offset: 0 });
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.branches).toEqual(["main"]));
  act(() => result.current.load());
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
});

test("concurrent load() for same repoId dedupes to one call", async () => {
  let resolve!: (v: unknown) => void;
  vi.mocked(listRepositoryBranches).mockReturnValue(new Promise((r) => { resolve = r; }) as never);
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => { result.current.load(); result.current.load(); });
  resolve({ items: ["main"], total: 1, limit: 100, offset: 0 });
  await waitFor(() => expect(result.current.branches).toEqual(["main"]));
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
});

test("stale response for repo A is dropped after switching to repo B (S4)", async () => {
  const deferred: Record<number, (v: unknown) => void> = {};
  vi.mocked(listRepositoryBranches).mockImplementation((_org, id) =>
    new Promise((r) => { deferred[id as number] = r; }) as never);
  const { result, rerender } = renderHook(({ id }) => useRepositoryBranches(id), { initialProps: { id: 1 } });
  act(() => result.current.load());
  rerender({ id: 2 });
  act(() => result.current.load());
  deferred[2]({ items: ["b-branch"], total: 1, limit: 100, offset: 0 });
  await waitFor(() => expect(result.current.branches).toEqual(["b-branch"]));
  deferred[1]({ items: ["a-branch"], total: 1, limit: 100, offset: 0 }); // late A
  await Promise.resolve();
  expect(result.current.branches).toEqual(["b-branch"]); // A did not overwrite B
});

test("fetch error sets fallbackToFreeText", async () => {
  vi.mocked(listRepositoryBranches).mockRejectedValue(new Error("FailedPrecondition: no git credential"));
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.fallbackToFreeText).toBe(true));
  expect(result.current.branches).toEqual([]);
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/hooks/__tests__/useRepositoryBranches.test.ts`
Expected: FAIL (module not found).

- [ ] **Step 3: Implement the hook**

Model the inflight/listener cache on `useTicketPods.ts:17-118`. Key the cache by
`repoId`; store `{ branches, status: "idle"|"loading"|"done"|"error", reqToken }`.
`load()` no-ops if status is `loading`/`done`. On resolve, only commit if the
entry's `reqToken` still matches the token captured at fetch start (S4). On
reject, set `status="error"` (drives `fallbackToFreeText`). `refresh()` bumps the
token and forces a fetch. Keep the file under 200 lines.

- [ ] **Step 4: Run to verify pass**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/hooks/__tests__/useRepositoryBranches.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add clients/web/src/hooks/useRepositoryBranches.ts clients/web/src/hooks/__tests__/useRepositoryBranches.test.ts
git commit -m "feat(web): lazy useRepositoryBranches hook with stale-response guard"
```

---

### Task 6: `BranchCombobox` component

**Files:**
- Create: `clients/web/src/components/pod/CreatePodForm/BranchCombobox.tsx`
- Create/Test: `clients/web/src/components/pod/CreatePodForm/__tests__/BranchCombobox.test.tsx`

**Interfaces:**
- Consumes: `useRepositoryBranches(repoId)` (Task 5); existing `BranchInput` (`RepositorySelect.tsx:52-85`) for the fallback render; `selectedBranch: string` value + `onChange(branch: string)`.
- Produces:
  ```ts
  interface BranchComboboxProps {
    repoId: number;
    value: string;
    onChange: (value: string) => void;
    error?: string;
    t: (key: string) => string;
  }
  function BranchCombobox(props: BranchComboboxProps): JSX.Element
  ```

- [ ] **Step 1: Write the failing tests**

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { BranchCombobox } from "../BranchCombobox";

const hook = { branches: ["main", "develop"], loading: false, fallbackToFreeText: false, load: vi.fn(), refresh: vi.fn() };
vi.mock("@/hooks/useRepositoryBranches", () => ({ useRepositoryBranches: () => hook }));
const t = (k: string) => k;

test("opening the dropdown calls load()", () => {
  render(<BranchCombobox repoId={1} value="" onChange={() => {}} t={t} />);
  fireEvent.focus(screen.getByRole("combobox"));
  expect(hook.load).toHaveBeenCalled();
});

test("lists fetched branches and selecting one fires onChange", () => {
  const onChange = vi.fn();
  render(<BranchCombobox repoId={1} value="" onChange={onChange} t={t} />);
  fireEvent.focus(screen.getByRole("combobox"));
  fireEvent.click(screen.getByText("develop"));
  expect(onChange).toHaveBeenCalledWith("develop");
});

test("typed non-listed value is accepted (editable combobox)", () => {
  const onChange = vi.fn();
  render(<BranchCombobox repoId={1} value="" onChange={onChange} t={t} />);
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "feature/new" } });
  expect(onChange).toHaveBeenCalledWith("feature/new");
});

test("falls back to plain text input when hook signals error", () => {
  hook.fallbackToFreeText = true;
  render(<BranchCombobox repoId={1} value="x" onChange={() => {}} t={t} />);
  expect(screen.getByLabelText(/branch/i)).toBeInTheDocument(); // BranchInput rendered
  hook.fallbackToFreeText = false; // reset for other tests
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/components/pod/CreatePodForm/__tests__/BranchCombobox.test.tsx`
Expected: FAIL (module not found).

- [ ] **Step 3: Implement the component**

Editable `<input role="combobox">` bound to `value`; `onChange` writes the raw
typed string (preserves new-branch capability). On focus/open call `load()`.
Render a filtered list (`branches.filter(b => b.includes(typed))`) as clickable
options; clicking calls `onChange(branch)`. Surface the default branch first if
present (the hook returns plain strings; if a default marker is needed later it
is out of scope - keep to strings). When `fallbackToFreeText` is true, render the
existing `BranchInput`. Keep under 200 lines; if it approaches the limit, extract
the option-list into a sibling `BranchOptionList.tsx`.

- [ ] **Step 4: Run to verify pass**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/components/pod/CreatePodForm/__tests__/BranchCombobox.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add clients/web/src/components/pod/CreatePodForm/BranchCombobox.tsx clients/web/src/components/pod/CreatePodForm/__tests__/BranchCombobox.test.tsx
git commit -m "feat(web): editable BranchCombobox with free-text fallback"
```

---

### Task 7: Wire `BranchCombobox` into the form

**Files:**
- Modify: `clients/web/src/components/pod/CreatePodForm/AdvancedFormSection.tsx:120-128`

**Interfaces:**
- Consumes: `BranchCombobox` (Task 6); `form.selectedRepository` (number), `form.selectedBranch` (string), `form.setSelectedBranch`, `form.validationErrors.branch`.

- [ ] **Step 1: Replace BranchInput usage**

In `AdvancedFormSection.tsx`, swap the `BranchInput` block (lines 120-128) for:

```tsx
{form.selectedRepository && (
  <BranchCombobox
    repoId={form.selectedRepository}
    value={form.selectedBranch}
    onChange={form.setSelectedBranch}
    error={form.validationErrors.branch}
    t={t}
  />
)}
```

Update the import on line 12: drop `BranchInput`, add
`import { BranchCombobox } from "./BranchCombobox";`. Keep `RepositorySelect`.
(`BranchInput` stays exported from `RepositorySelect.tsx` because the combobox
fallback renders it.)

- [ ] **Step 2: Type-check + existing form tests**

Run: `bazel build //clients/web:src && pnpm --filter @agentsmesh/web exec vitest run clients/web/src/components/pod/CreatePodForm/__tests__/CreatePodForm.test.tsx`
Expected: PASS (form-state contract unchanged).

- [ ] **Step 3: Commit**

```bash
git add clients/web/src/components/pod/CreatePodForm/AdvancedFormSection.tsx
git commit -m "feat(web): use BranchCombobox in create-pod advanced section"
```

---

## Phase C - Feature 2: ticket-view pod refresh

### Task 8: Same-tab refresh on spawn (`TicketDetailSidebar`)

**Files:**
- Modify: `clients/web/src/components/tickets/TicketDetailSidebar.tsx:58-65`
- Create/Test: `clients/web/src/components/tickets/__tests__/TicketDetailSidebar.podRefresh.test.tsx`

**Interfaces:**
- Consumes: `SpawnPodButton` already forwards `onPodCreated` from its modal `onCreated` (`SpawnPodButton.tsx:46-49`); `invalidateTicketPods(ticketSlug)` (`useTicketPods.ts:110`).

- [ ] **Step 1: Write the failing test**

```tsx
const invalidate = vi.fn();
vi.mock("@/hooks/useTicketPods", () => ({
  useTicketPods: () => ({ pods: [], loading: false, ready: true, error: null, refresh: vi.fn() }),
  invalidateTicketPods: (slug: string) => invalidate(slug),
}));
// Stub SpawnPodButton to expose its onPodCreated synchronously
vi.mock("@/components/tickets/SpawnPodButton", () => ({
  SpawnPodButton: ({ onPodCreated }: { onPodCreated?: () => void }) => (
    <button onClick={onPodCreated}>spawn</button>
  ),
}));

test("spawning a pod invalidates this ticket's pods", () => {
  render(<TicketDetailSidebar {...baseProps} ticketSlug="AM-1" />);
  fireEvent.click(screen.getByText("spawn"));
  expect(invalidate).toHaveBeenCalledWith("AM-1");
});
```

`baseProps` provides the minimal `ticket`, `t`, etc. the sidebar needs (model on
any existing `TicketDetailSidebar` test or construct a minimal `Ticket`).

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/components/tickets/__tests__/TicketDetailSidebar.podRefresh.test.tsx`
Expected: FAIL (no `onPodCreated` wired).

- [ ] **Step 3: Wire the callback**

In `TicketDetailSidebar.tsx`, import `invalidateTicketPods` from
`@/hooks/useTicketPods` and pass:

```tsx
<SpawnPodButton
  ticket={ticket}
  ticketSlug={ticketSlug}
  onPodCreated={() => invalidateTicketPods(ticketSlug)}
  size="lg"
  className="h-11 w-full gap-2 text-sm font-semibold shadow-sm"
/>
```

- [ ] **Step 4: Run to verify pass**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/components/tickets/__tests__/TicketDetailSidebar.podRefresh.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add clients/web/src/components/tickets/TicketDetailSidebar.tsx clients/web/src/components/tickets/__tests__/TicketDetailSidebar.podRefresh.test.tsx
git commit -m "fix(web): refresh ticket pods when spawning a pod in the ticket view"
```

---

### Task 9: Populate `ticket_slug` on `pod:created`, consistently (S3 backend)

The `pod:created` wire event must always carry a `PodCreatedEventData` (with
`ticket_slug`). Today the deterministic emitter (`mutations.go:84`) sends
`PodCreatedEventData` but leaves `ticket_slug` empty; the race-prone status
callback (`eventbus_pod.go:38-46`) sends a `PodStatusChangedEventData` under the
same event type. Make both emit `PodCreatedEventData` with the slug populated.

**Files:**
- Modify: `backend/internal/api/connect/pod/mutations.go:79-103` (`publishPodCreated` - set `TicketSlug`)
- Modify: `backend/cmd/server/eventbus_pod.go:20-62` (EventPodCreated branch -> emit `PodCreatedEventData` incl. resolved `ticket_slug`)
- Modify/Test: `backend/internal/api/connect/pod/mutations_test.go` (or nearest existing pod-mutation test)

**Interfaces:**
- Consumes: `req.Msg.TicketSlug` already available in `CreatePod` (`mutations.go:48`); `eventsv1.PodCreatedEventData{..., TicketSlug}` (`proto/events/v1/event_data.proto:20-29`).
- Produces: every `pod:created` event decodes as `PodCreatedEventData` with `ticket_slug` set when the pod belongs to a ticket.

- [ ] **Step 1: Write the failing test (deterministic emitter)**

In the pod-mutations test, capture the published event and assert the slug:

```go
func TestPublishPodCreated_SetsTicketSlug(t *testing.T) {
	bus := newCapturingEventBus(t) // records published events
	srv := newPodServerWithBus(t, bus)
	srv.publishPodCreatedWithSlug(context.Background(), &podDomain.Pod{PodKey: "p1", OrganizationID: 9, TicketID: ptr(int64(3))}, "AM-3")
	ev := bus.last()
	data := decodePodCreated(t, ev) // unmarshal PodCreatedEventData
	require.Equal(t, "AM-3", data.TicketSlug)
}
```

(Adjust to the real helper names; if no capturing bus helper exists, add a
minimal one in the test file.)

- [ ] **Step 2: Run to verify it fails**

Run: `bazel test //backend/internal/api/connect/pod:pod_test --test_filter=TestPublishPodCreated_SetsTicketSlug`
Expected: FAIL.

- [ ] **Step 3: Thread the slug through the deterministic emitter**

Change `publishPodCreated` to accept the slug (rename to keep one caller):

```go
func (s *Server) publishPodCreated(ctx context.Context, pod *podDomain.Pod, ticketSlug string) {
	// ... existing nil guards ...
	data := &eventsv1.PodCreatedEventData{
		PodKey: pod.PodKey, Status: pod.Status, AgentStatus: pod.AgentStatus,
		RunnerId: pod.RunnerID, CreatedById: pod.CreatedByID, TicketSlug: ticketSlug,
	}
	if pod.TicketID != nil { data.TicketId = pod.TicketID }
	// ... existing NewEntityEvent + Publish ...
}
```

Update the `CreatePod` call site to `s.publishPodCreated(ctx, result.Pod, optionalString(req.Msg.TicketSlug))`.

- [ ] **Step 4: Make the status-callback emitter consistent**

In `eventbus_pod.go`, extend the existing pod lookup query to also select the
ticket slug (LEFT JOIN tickets on `pods.ticket_id = tickets.id`), and in the
`EventPodCreated` branch build a `PodCreatedEventData` (not
`PodStatusChangedEventData`) with `TicketSlug` set. Leave the other branches
(`EventPodStatusChanged`, `EventPodTerminated`, `EventPodAgentChanged`) using
`PodStatusChangedEventData` unchanged.

- [ ] **Step 5: Run to verify pass + no regressions**

Run: `bazel test //backend/internal/api/connect/pod/... //backend/cmd/server/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api/connect/pod/ backend/cmd/server/eventbus_pod.go
git commit -m "feat(pod): carry ticket_slug on pod:created from both emitters (S3 backend)"
```

---

### Task 10: Realtime handler invalidates the ticket (S3 frontend)

**Files:**
- Modify: `clients/web/src/providers/realtimePodHandlers.ts:24-30`
- Create/Test: `clients/web/src/providers/__tests__/realtimePodHandlers.test.ts`

**Interfaces:**
- Consumes: `decodeEventData(PodCreatedEventDataSchema, event.data)` (schema already imported via `@/lib/realtime`, `types.ts:109`); `invalidateTicketPods(slug)` (`useTicketPods.ts:110`).

- [ ] **Step 1: Write the failing tests**

```ts
const invalidate = vi.fn();
vi.mock("@/hooks/useTicketPods", () => ({ invalidateTicketPods: (s: string) => invalidate(s) }));
// plus the existing pod/mesh store mocks modeled on realtimeEventHandlers.test.ts

test("pod:created with ticketSlug invalidates that ticket", () => {
  handlePodEvent(makePodCreatedEvent({ podKey: "p1", ticketSlug: "AM-7" }));
  expect(invalidate).toHaveBeenCalledWith("AM-7");
});

test("pod:created without ticketSlug does not invalidate", () => {
  handlePodEvent(makePodCreatedEvent({ podKey: "p1", ticketSlug: "" }));
  expect(invalidate).not.toHaveBeenCalled();
});
```

`makePodCreatedEvent` builds a `RealtimeEvent` of type `pod:created` whose
`data` is a binary-encoded `PodCreatedEventData` (use `toBinary` +
`PodCreatedEventDataSchema`, mirroring how `realtimeEventHandlers.test.ts`
constructs event data).

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/providers/__tests__/realtimePodHandlers.test.ts`
Expected: FAIL (handler ignores payload).

- [ ] **Step 3: Decode + invalidate in the handler**

In `realtimePodHandlers.ts`, change the `pod:created` case to decode the payload
and invalidate when a slug is present (keep `refreshSidebar()` +
`refreshMeshTopology()` for the workspace view):

```ts
case "pod:created":
case "pod:restarting": {
  refreshSidebar();
  refreshMeshTopology();
  if (event.type === "pod:created") {
    const data = decodeEventData(PodCreatedEventDataSchema, event.data);
    if (data.ticketSlug) invalidateTicketPods(data.ticketSlug);
  }
  break;
}
```

Add the `PodCreatedEventDataSchema` + `invalidateTicketPods` imports. Note: this
is safe only because Task 9 guarantees every `pod:created` carries a
`PodCreatedEventData` (the `pod:restarting` branch is NOT decoded).

- [ ] **Step 4: Run to verify pass**

Run: `pnpm --filter @agentsmesh/web exec vitest run clients/web/src/providers/__tests__/realtimePodHandlers.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add clients/web/src/providers/realtimePodHandlers.ts clients/web/src/providers/__tests__/realtimePodHandlers.test.ts
git commit -m "feat(web): invalidate ticket pods on realtime pod:created (S3 frontend)"
```

---

## Phase D - End-to-end (Playwright)

> Harness: `clients/web/e2e-playwright/`. Auth via `tests/global.setup.ts`
> (storageState). External git providers are faked at the BACKEND boundary
> (seed a repo whose `ProviderBaseURL` points at a fake provider), NOT via
> `page.route`. Model new specs on existing ones in `tests/pod/`,
> `tests/tickets/`, `tests/mesh/` and page objects in `pages/`.

### Task 11: Branch dropdown E2E

**Files:**
- Create: `clients/web/e2e-playwright/pages/create-pod.page.ts`
- Create: `clients/web/e2e-playwright/tests/pod/branch-dropdown.spec.ts`

- [ ] **Step 1: Write the page object**

A `CreatePodPage` exposing role-based locators: `openAdvanced()`,
`branchCombobox()` (`getByRole("combobox")` scoped to the branch field),
`branchOption(name)` (`getByRole("option", { name })` or `getByText`),
`submit()`. Follow `workspace.page.ts` conventions; explicit return types.

- [ ] **Step 2: Write the specs**

```ts
test("branch dropdown loads branches lazily on open", async ({ page, seedRepoWithFakeProvider }) => {
  await seedRepoWithFakeProvider({ branches: ["main", "develop"] });
  const createPod = new CreatePodPage(page);
  await createPod.gotoWorkspaceCreate();
  await createPod.openAdvanced();
  await createPod.selectRepository();
  await expect(createPod.branchOption("develop")).toHaveCount(0); // not fetched yet
  await createPod.branchCombobox().focus();
  await expect(createPod.branchOption("develop")).toBeVisible(); // fetched on open
});

test("branch fetch failure falls back to free-text", async ({ page, seedRepoWithFakeProvider }) => {
  await seedRepoWithFakeProvider({ status: 500 });
  const createPod = new CreatePodPage(page);
  await createPod.gotoWorkspaceCreate();
  await createPod.openAdvanced();
  await createPod.selectRepository();
  await createPod.branchCombobox().focus();
  await createPod.branchCombobox().fill("feature/typed");
  await expect(createPod.branchCombobox()).toHaveValue("feature/typed"); // editable, accepted
});
```

`seedRepoWithFakeProvider` is a fixture extension (add to `fixtures/`) that
inserts a repo row pointing `ProviderBaseURL` at a fake provider responding with
the given branch set or status. Reuse the existing DB-seeding fixture
(`db.fixture.ts`) plus a small fake-provider helper (model on `helpers/`).

- [ ] **Step 3: Run**

Run the `e2e-playwright` project for `tests/pod/branch-dropdown.spec.ts`.
Expected: PASS (requires the dev stack / CI harness the other e2e specs use).

- [ ] **Step 4: Commit**

```bash
git add clients/web/e2e-playwright/pages/create-pod.page.ts clients/web/e2e-playwright/tests/pod/branch-dropdown.spec.ts clients/web/e2e-playwright/fixtures/
git commit -m "test(e2e): branch dropdown lazy load + free-text fallback"
```

---

### Task 12: Ticket-view pod refresh E2E

**Files:**
- Create: `clients/web/e2e-playwright/tests/tickets/ticket-pod-refresh.spec.ts`

- [ ] **Step 1: Write the specs**

```ts
test("creating a pod inside a ticket shows it in the sidebar without reload", async ({ page, seedTicket, mockAgent }) => {
  const ticket = await seedTicket();
  await page.goto(ticketUrl(ticket));
  // spawn a pod from the ticket detail sidebar
  await page.getByRole("button", { name: /spawn|start pod/i }).click();
  await new CreatePodPage(page).submit();
  // no page.reload(): the working-pods rail must show the pod
  await expect(page.getByText(/working pods/i)).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: ticket.podAlias })).toBeVisible();
});

test("realtime pod:created updates the ticket sidebar in a second tab", async ({ context, seedTicket, pushPodCreated }) => {
  const ticket = await seedTicket();
  const a = await context.newPage(); await a.goto(ticketUrl(ticket));
  await pushPodCreated({ ticketSlug: ticket.slug, podKey: "p-xyz" }); // via eventbus-stream helper
  await expect(a.getByRole("listitem").filter({ hasText: "p-xyz" })).toBeVisible();
});
```

`pushPodCreated` drives a `pod:created` event carrying `ticket_slug` through the
existing realtime test seam (`helpers/eventbus-stream.ts`). `mockAgent` is the
existing fake-pod helper (`helpers/mock-agent.ts`).

- [ ] **Step 2: Run**

Run the `e2e-playwright` project for `tests/tickets/ticket-pod-refresh.spec.ts`.
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add clients/web/e2e-playwright/tests/tickets/ticket-pod-refresh.spec.ts
git commit -m "test(e2e): ticket-view pod refresh same-tab and realtime"
```

---

## Self-review notes (coverage map)

- Spec Feature 1 backend (S1/S2/S5) -> Tasks 2, 3, 4. Provider failure audit -> Task 1.
- Spec Feature 1 frontend (lazy hook S4, combobox, fallback) -> Tasks 5, 6, 7.
- Spec Feature 2 same-tab -> Task 8. S3 realtime -> Tasks 9, 10.
- E2E -> Tasks 11, 12.
- Concurrency 6a (per-tab dedupe) -> Task 5 test 3; 6b (stale guard) -> Task 5 test 4; multitab 6a-across-tabs -> Task 12 second spec.
- Scenarios: no-credential -> Tasks 2/3/5; provider-error -> Tasks 1/2/5; success variants -> Tasks 1/11; logout-mid-fetch -> Task 5 error path (auth rejection is the same error->fallback branch); repo-added-while-open -> documented no-auto-refresh (per-repoId cache, Task 5; no task needed).

**Supersedes test plan:** S3 needs NO new proto field - `PodCreatedEventData.ticket_slug` already exists (`event_data.proto:20-29`) and is already decoded frontend-side. The realtime work is populating the slug from both emitters (Task 9) and decoding it in the handler (Task 10), not a wire-format addition.
