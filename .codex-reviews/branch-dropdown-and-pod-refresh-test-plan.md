# Test Plan: Lazy Branch Dropdown + Ticket-View Pod Refresh

Scope: two features in AgentsMesh. No product/test code here - this is the plan.
Every test below is grounded in a real file + an existing pattern to copy.

## 0. Pre-flight findings that change the plan (read first)

These are code-grounded facts discovered during planning. Several make a
requested scenario UNtestable without a product-code change. They are called
out inline in each section and consolidated in section 5.

- **F1 - End-to-end path is currently broken for the new feature.** The Rust
  core sends `access_token: String::new()` (`clients/core/crates/ffi/src/services/repository.rs:55-63`).
  BOTH server entry points reject an empty token *before* reaching the service:
  Connect `ListRepositoryBranches` returns `CodeInvalidArgument "access token
  required"` (`backend/internal/api/connect/repository/repository_branches.go:28-30`);
  REST `ListBranches` returns `BadRequest MISSING_REQUIRED`
  (`backend/internal/api/rest/v1/repositories_branches.go:41-44`). The Feature-1
  backend change (server-side token resolution) is therefore a *prerequisite*,
  not an optional enhancement - the dropdown cannot load branches at all until
  the empty-token guard is replaced by a resolve-then-validate path.

- **F2 - No injection seam for the git provider in the service.** `repository.Service`
  holds only `repo` + `webhookService` (`backend/internal/service/repository/service.go:18-21`).
  `ListBranches` calls the package-level `git.NewProvider(...)`
  (`backend/internal/service/repository/service_sync.go:52`) which hits the live
  provider. There is no way to inject a fake `git.Provider`. The existing test
  `TestListBranchesNotFound` (`service_query_test.go:193`) only exercises the
  `GetByID` -> not-found short-circuit; it never reaches `NewProvider`. **A seam
  is required** to unit-test the happy path + token resolution at the service
  layer (see section 5, S1).

- **F3 - No `userService` on the bare `Service`.** Token resolution must mirror
  `webhook_registration.go:148` (`GetDecryptedProviderTokenByTypeAndURL`), but
  that dependency lives on `WebhookService` (`webhook_service.go:26`), not on
  `Service`. `ListBranches` is a `Service` method. The wiring decision (inject
  `userService` into `Service`, or move `ListBranches` token-resolution into
  `WebhookService`) determines which mock the tests use (see section 5, S2).

- **F4 - The `pod:created` realtime event carries NO ticket linkage.** It is
  emitted as `PodStatusChangedEventData{PodKey, Status, AgentStatus}`
  (`backend/cmd/server/eventbus_pod.go:43-46`, type chosen at line 39). The
  frontend handler `handlePodEvent` `case "pod:created"`
  (`clients/web/src/providers/realtimePodHandlers.ts:25-29`) has only a pod key,
  so it *cannot* call `invalidateTicketPods(ticketSlug)` - there is no slug to
  pass. Pods do carry a ticket FK (`agentpod/pod.go:58 TicketID`,
  `pod_commands.go:13 TicketSlug`), but it is not on the wire event. **Feature 2's
  realtime half requires a product-code change**: either add `ticket_slug` to the
  pod event payload, or have `useTicketPods` reconcile via a pod-key->ticket map.
  Until that lands, only the SpawnPodButton `onPodCreated` half of Feature 2 is
  testable (see section 5, S3).

- **F5 - GitHub maps 429 to nothing special; GitLab/Gitee do.** `github_client.go`
  `doRequest` maps 401->`ErrUnauthorized`, 404->`ErrNotFound`,
  **403->`ErrRateLimited`** (`github_client.go:70-73`), and any other >=400 to a
  generic `fmt.Errorf("GitHub API error %d...")`. There is **no `429` branch** for
  GitHub (`grep 429 github_client.go` -> empty). GitLab maps **429**->`ErrRateLimited`
  (`gitlab_client.go:62-64`); Gitee maps **403**->`ErrRateLimited`
  (`gitee_client.go:67-69`). See the provider audit (section 4) for the
  handled/unhandled matrix.

---

## 1. Feature 1 - Lazy branch dropdown

### 1.1 State-transition table

State -> Trigger -> Expected outcome -> Layer -> Test name

| # | State | Trigger | Expected outcome | Layer | Test name |
|---|---|---|---|---|---|
| 1a | Repo selected, user has NO git credential | Dropdown opened -> server cannot resolve token | Service/handler returns typed "no credential" error; UI swaps to free-text `BranchInput` | BE unit + FE component | `TestListBranches_NoCredential_TypedFallbackError`; `useRepositoryBranches falls back on typed-error` |
| 1b | Same | Same | Combobox renders plain free-text input; typed value still accepted | FE component | `BranchCombobox renders free-text fallback on fetch error` |
| 2a | Token present, provider 5xx / connection error / timeout | Dropdown opened | Service returns wrapped provider error; handler maps to `CodeUnavailable`/`CodeInternal`; UI free-text fallback | BE unit + FE | `TestListBranches_ProviderError_Propagates`; `useRepositoryBranches falls back on provider error` |
| 3a | Token resolvable, provider OK, many branches | Dropdown opened | `items: string[]` returned; combobox lists branches; default branch surfaced first/marked | BE unit + FE + E2E | `TestListBranches_Success_ManyBranches`; `BranchCombobox lists fetched branches`; e2e `branch dropdown loads lazily` |
| 3b | Same, empty branch list | Dropdown opened | `items: []`, `total: 0`; combobox shows empty state but stays editable | BE unit + FE | `TestListBranches_Success_EmptyList`; `BranchCombobox empty list stays editable` |
| 3c | Same, single branch | Dropdown opened | one item; that branch selectable; default flag honored | BE unit + FE | `TestListBranches_Success_SingleBranch` |
| 3d | Same, default-branch surfacing | Dropdown opened | the repo's default branch is identifiable (GitHub derives via `GetProject`; GitLab/Gitee return `default` flag) | BE provider unit | `TestGitHubListBranches/default-branch-surfaced` (extend existing `github_branch_test.go:11`) |
| 4 | Create-pod panel open, dropdown open | A new repo is added in another tab/session | Branch cache is per-repoId; the *repo list* does not auto-refresh while open (no live repo subscription in this panel). Expected: NO new repo appears until panel re-open/refetch. Assert documented behavior, not magic refresh | FE component | `useRepositoryBranches cache is per-repoId, no cross-repo bleed`; documented in plan |
| 5 | Panel open / mid-fetch | User logs out (token invalidated) | In-flight fetch rejects with auth error; combobox falls to free-text; no stale branch list rendered from a prior repo | FE component | `useRepositoryBranches clears + falls back on auth error mid-fetch` |
| 6a | Two tabs, same repo | Both open dropdown | Per-tab inflight cache dedupes within a tab; tabs are independent processes (no shared JS cache) -> 1 fetch per tab, no torn state | FE component (per-tab) + E2E (multitab) | `useRepositoryBranches dedupes concurrent opens same repo`; e2e multitab |
| 6b | One tab, repo A fetch in flight | User switches to repo B before A resolves | A's late response must NOT overwrite B's selection/cache (stale-response guard keyed by repoId / request token) | FE component | `useRepositoryBranches stale response for repo A is dropped after switch to B` |
| 7 | Token present | Provider returns 403/429 (rate limit) | Provider returns `ErrRateLimited`; handler maps to a retryable/typed code; UI free-text fallback (do not hard-fail) | BE provider unit + handler unit | `TestListBranches_RateLimited_TypedError` |

### 1.2 Backend UNIT tests (testify / std-lib `t.Run`)

Pattern sources:
- Provider-level HTTP mock: `setupGitHubMockServer` + `httptest` already used in
  `backend/internal/infra/git/github_branch_test.go:11-59` (success) and the
  error-table style in `gitee_error_branch_test.go:9-40` (invalid JSON / status).
- Connect-handler fakes: `fakeRepoService` / `fakeOrgService` / `ctxAsUser` /
  `connectCodeOf` in `backend/internal/api/connect/repository/repository_test.go:19-156`.
- Service-with-DB: `setupTestService` / `setupTestDB` in
  `service_setup_test.go:11-30`, `testkit.SetupTestDB`.

Provider layer (`backend/internal/infra/git/`) - extend existing files, no new seam needed:
- `github_branch_test.go`: add `TestGitHubListBranches/empty`, `/single`,
  `/default-branch-surfaced` (assert `Default==true` for the project default,
  exercising the `GetProject` call at `github_branch.go:30-34`), `/401_unauthorized`
  (`->ErrUnauthorized`), `/403_rate_limited` (`->ErrRateLimited`,
  `github_client.go:70-73`), `/malformed_json`. There is no GitHub 429 branch
  (F5) - assert that a 429 currently falls through to the generic
  `"GitHub API error 429"` error (regression-lock the gap, or drive a code fix).
- `gitlab_branch_test.go`: add `/429_rate_limited` (`gitlab_client.go:62-64`),
  `/empty`, `/single`, `/default-flag-honored` (GitLab returns `default` in the
  payload, `gitlab_branch.go:24,37`).
- `gitee_branch_test.go` / `gitee_error_branch_test.go`: add `/403_rate_limited`
  (`gitee_client.go:67-69`), parity with the above.

Service layer (`backend/internal/service/repository/`) - REQUIRES seam S1+S2:
- New file `service_branches_test.go`:
  - `TestListBranches_NotFound` (already exists at `service_query_test.go:193` -
    keep / move).
  - `TestListBranches_Success_*` (Empty/Single/Many) - needs an injectable
    `git.Provider` factory (S1) so the test can return a canned `[]*Branch`.
  - `TestListBranches_ProviderError_Propagates` - factory returns an error.
  - `TestListBranches_NoCredential_TypedFallbackError` - needs `userService`
    seam (S2); mock returns empty token / `ErrNoAccessToken`; assert a NEW typed
    sentinel error (e.g. `ErrNoGitCredential`) the handler can translate. Model
    the resolution on `webhook_registration.go:140-169`.
  - `TestListBranches_ExplicitTokenWins` - when a non-empty token is passed,
    assert `userService` resolution is NOT called.

Connect handler (`backend/internal/api/connect/repository/`) - extend
`repository_test.go`:
- `TestListRepositoryBranches_MissingOrgSlug_InvalidArgument`,
  `_NoAuth_Unauthenticated`, `_NonMember_PermissionDenied` (copy the existing
  guard tests at `repository_test.go:91-133`, swap the method).
- `TestListRepositoryBranches_EmptyToken_ResolvesServerSide` - the key
  behavioral change vs. `repository_branches.go:28-30`. After the feature, an
  empty token must NOT short-circuit to `CodeInvalidArgument`; it must call the
  service path. Requires the handler to have `tenant.UserID` (already available,
  `repository_branches.go:115`).
- `TestListRepositoryBranches_NoCredential_TypedCode` - service returns
  `ErrNoGitCredential`; assert handler maps it to a distinct code the frontend
  recognizes as "use free-text" (extend the `mapServiceError` table test at
  `repository_test.go:137-156`).

### 1.3 Backend INTEGRATION tests

Harness to reuse: the `*_integration_test.go` + `testkit.SetupTestDB` pattern.
Closest existing model: `backend/internal/service/repository/repository_integration_test.go`
(service+DB) and the runner/agentpod lifecycle integration tests
(`pod_lifecycle_integration_test.go`). There is **no httptest-wired Connect
server harness** for repository in-repo today (the Connect tests use direct
handler calls with fakes, `repository_test.go`). Two options:

- **Integration-lite (recommended, matches repo convention):** new
  `service_branches_integration_test.go` - real `Service` + real DB
  (`setupTestService`), real `userService` (decrypt a seeded provider credential),
  and a **provider faked at the HTTP boundary** via `httptest` server whose URL is
  stored as the repo's `ProviderBaseURL`. This exercises real
  `git.NewProvider(...).ListBranches` end-to-end without a live GitHub. Assert:
  resolved-token path, no-credential typed error, provider 5xx propagation.
  This works WITHOUT seam S1 because the httptest URL is injected through the
  repo row, not through a factory.
- Full Connect-over-HTTP (`httptest` + generated connect client) is NOT an
  established pattern here; do not introduce it just for this feature.

### 1.4 Frontend Vitest tests

Conventions (verified):
- Setup: `clients/web/src/test/setup.ts` provides the wasm-core mock harness
  (`createAcpManager`, hoisted store stubs). Rust-core wasm is mocked by
  `vi.mock("@/lib/wasm-core", ...)` returning fake services - see
  `useTicketPods.test.ts:25-46` (mocks `getTicketService`/`getTicketState`) and
  `realtimeEventHandlers.test.ts:24-60`.
- Connect facade mapping is tested against the proto directly:
  `clients/web/src/lib/api/__tests__/repositoryConnect.test.ts:13-45` (note its
  bazel-ignore caveat in the header - runs under `pnpm test:run`).
- Component tests mock `next-intl` `useTranslations` to identity and mock child
  components: `CreatePodForm/__tests__/CreatePodForm.test.tsx:14-40`.

New hook `useRepositoryBranches(repoId)` - new file
`clients/web/src/hooks/__tests__/useRepositoryBranches.test.ts`. Model the cache
mechanics directly on `useTicketPods.test.ts` (inflight Map + listeners +
`__resetCacheForTests`). Mock `listRepositoryBranches` from the facade
(`@/lib/api/facade/repositoryConnect` re-export, or `@/lib/wasm-core`
`getRepositoryService().listRepositoryBranchesConnect`). Cases:
- `fetches once and shares result per repoId` (copy `useTicketPods.test.ts:66-79`).
- `dedupes concurrent opens for the same repoId` (copy `:81-92`, the
  `pendingFetch` gate).
- `independent caches for different repoIds`.
- `stale response for repo A dropped after switch to repo B` (state-table 6b) -
  hold A's promise, mount/fetch B, resolve A late, assert B's data wins. The hook
  needs a per-request guard token; if absent it is UNtestable -> S4.
- `error -> fallback flag` for: typed no-credential error, generic provider error,
  auth error mid-fetch (state-table 1a/2a/5). Assert the hook exposes a
  `fallbackToFreeText` (or `error`) signal the component consumes.

New `BranchCombobox` component - new file
`clients/web/src/components/pod/CreatePodForm/__tests__/BranchCombobox.test.tsx`:
- `renders fetched branches when open` (mock hook returns items).
- `typed non-listed value is accepted` (type `feature/new`, assert
  `onChange("feature/new")` fires - editable combobox contract).
- `filters list as user types`.
- `default branch is surfaced` (marked/first).
- `falls back to plain BranchInput when hook signals error` (state 1b) - assert
  the existing `BranchInput` (`RepositorySelect.tsx:52-85`) renders and still
  drives `onChange`.
- `empty list keeps input editable` (state 3b).
- Form-state invariant: `selectedBranch: string` unchanged
  (`useCreatePodFormTypes.ts:20,41`) - assert the combobox writes a string, not
  an object.

### 1.5 Playwright E2E

Harness (verified): `clients/web/e2e-playwright/playwright.config.ts`, fixtures in
`e2e-playwright/fixtures/` (`api.fixture.ts`, `db.fixture.ts`,
`blockstore.fixture.ts`, `index.ts`), page objects in `e2e-playwright/pages/`
(`workspace.page.ts`, `login.page.ts`, `sidebar.page.ts`), helpers
(`mock-agent.ts`, `eventbus-stream.ts`, `connect-stream.ts`,
`test-data.ts`). Auth is set up via `tests/global.setup.ts` / `admin.setup.ts`
(storageState). **External git-provider APIs are mocked at the BACKEND boundary
in these e2e (real backend + faked provider via httptest-style `ProviderBaseURL`
or `mock-agent`), NOT via Playwright `page.route`** - there is no `page.route`
or `Cdat` usage anywhere in `e2e-playwright` (`grep` -> empty). The CDAT
`features/<feature>/{components,data,actions}` layout referenced by the global
rule does NOT exist in this repo; the real convention is
`tests/<area>/<name>.spec.ts` + `pages/*.page.ts` + `fixtures/*`.

New spec `clients/web/e2e-playwright/tests/pods/branch-dropdown.spec.ts`:
- `branch dropdown loads branches lazily` - seed a repo whose `ProviderBaseURL`
  points at a fake provider returning a known branch set; open create-pod ->
  advanced settings -> open dropdown; assert branches appear only after open
  (no eager fetch on form mount).
- `branch fetch failure falls back to free-text` - fake provider returns 5xx;
  assert a plain text input is rendered and a typed branch is accepted.
- `pick a branch then create pod` - select default branch, submit, assert pod
  created against that branch.
Add a `pages/create-pod.page.ts` page object (follow `workspace.page.ts`).

---

## 2. Feature 2 - Ticket-view pod refresh

### 2.1 State-transition table

| # | State | Trigger | Expected outcome | Layer | Test name |
|---|---|---|---|---|---|
| A | Ticket detail open, `TicketDetailSidebar` shown | User spawns a pod via `SpawnPodButton` | `onPodCreated` fires -> `invalidateTicketPods(ticketSlug)` + refetch -> new pod appears in "working pods" rail without reload | FE component | `TicketDetailSidebar invalidates ticket pods on spawn` |
| B | Ticket detail open | Backend emits `pod:created` for a pod on THIS ticket | Sidebar pod list refreshes for the matching ticket (requires ticket linkage on the event) | FE handler + E2E | `handlePodEvent invalidates ticket pods on pod:created` |
| C | Ticket detail open | `pod:created` for a pod on a DIFFERENT ticket | This ticket's cache is NOT invalidated (no spurious refetch) | FE handler | `handlePodEvent does not invalidate unrelated ticket` |
| D | Ticket detail open | `pod:status_changed` to running | Pod row updates in place (already handled, regression-lock) | FE handler | existing pattern, add ticket-scope assertion |

### 2.2 The wiring gap (decides what is testable now)

- **State A is testable today** with a one-line product fix: pass
  `onPodCreated={() => { invalidateTicketPods(ticketSlug); }}` (or a refetch
  callback) into `SpawnPodButton` at `TicketDetailSidebar.tsx:60`. The correct
  shape already exists in `SidebarPodSection.tsx:34-38`
  (`invalidateTicketPods(ticketSlug); void refresh();`). `SpawnPodButton` already
  forwards `onPodCreated` from its modal (`SpawnPodButton.tsx:onCreated`).
- **States B/C are NOT testable today** - the `pod:created` event lacks a ticket
  slug (F4). Required product change before writing B/C tests (S3):
  add `ticket_slug` to `PodStatusChangedEventData` (backend
  `eventbus_pod.go:43-46` + proto `event_data.proto`), then in
  `realtimePodHandlers.ts:25-29` call `invalidateTicketPods(data.ticketSlug)`
  when present. Only after that can the handler test assert per-ticket scoping.

### 2.3 Frontend Vitest tests

`SpawnPodButton` / `TicketDetailSidebar` - new file
`clients/web/src/components/tickets/__tests__/TicketDetailSidebar.podRefresh.test.tsx`:
- Mock `@/hooks/useTicketPods` exposing a spied `invalidateTicketPods` and a
  controllable `pods` array (copy the `vi.mock("@/lib/wasm-core")` + seed pattern
  from `useTicketPods.test.ts:25-50`).
- `spawning a pod invalidates this ticket's pods` (state A): render sidebar,
  fire the `SpawnPodButton` -> simulate modal `onCreated`, assert
  `invalidateTicketPods` called with the ticket slug.
- `new pod appears in working-pods rail after invalidate` - flip the mocked
  `pods` to include a running pod, assert it renders in the rail
  (`TicketDetailSidebar.tsx:71-104`).

`handlePodEvent` - extend
`clients/web/src/providers/__tests__/` (new `realtimePodHandlers.test.ts`, model
on `realtimeEventHandlers.test.ts:1-60`). **Only after S3:**
- `pod:created with ticketSlug invalidates that ticket` (state B).
- `pod:created for other ticket does not invalidate` (state C).
- Keep existing `refreshSidebar`/`refreshMeshTopology` assertions
  (`realtimePodHandlers.ts:25-29`) as regression locks.

### 2.4 Playwright E2E

New spec `clients/web/e2e-playwright/tests/tickets/ticket-pod-refresh.spec.ts`
(model on `tests/mesh/channel-realtime.spec.ts` + `helpers/eventbus-stream.ts`
for driving realtime events, and `helpers/mock-agent.ts` for a fake pod):
- `creating a pod inside a ticket shows it in the sidebar without reload`
  (state A end-to-end): open a ticket, spawn a pod, assert the working-pods rail
  shows the running pod with no `page.reload()`.
- `realtime pod:created updates the ticket sidebar` (state B) - **gated on S3**;
  push a `pod:created` via `eventbus-stream.ts` and assert the rail updates.

---

## 3. Concurrency / race matrix (consolidated)

| Race | Where | Guard under test | Layer | Test |
|---|---|---|---|---|
| Two opens, same repo, one tab | `useRepositoryBranches` inflight Map | dedupe -> 1 network call | FE | `dedupes concurrent opens same repo` |
| Two tabs, same repo | independent JS heaps | no shared cache, each tab fetches once, no torn render | E2E multitab | model on `channel-members-multitab.spec.ts` |
| Repo A in flight, switch to B | stale-response guard (S4) | A's late resolve dropped | FE | `stale response for repo A dropped after switch to B` |
| Logout mid-fetch | auth rejection path | no stale list, fallback shown | FE | `clears + falls back on auth error mid-fetch` |
| Repo added while panel open | per-repoId cache scope | documented no-auto-refresh | FE | `cache is per-repoId, no cross-repo bleed` |

---

## 4. Git-provider failure-mode audit (handled vs unhandled)

Surface called by the feature: `Provider.ListBranches`
(`backend/internal/infra/git/provider.go:157`) -> `doRequest` per provider.

| Failure mode | GitHub | GitLab | Gitee | file:line | Test should assert |
|---|---|---|---|---|---|
| 401 unauthorized (revoked/invalid token) | `ErrUnauthorized` | mapped (`gitlab_client.go` 401 branch) | mapped (`gitee_client.go` 401 branch) | `github_client.go:62-65` | typed auth error -> UI fallback |
| 404 not found (repo gone / wrong externalID) | `ErrNotFound` | mapped | mapped | `github_client.go:66-69` | `ErrNotFound` -> handler `CodeNotFound` |
| 403 forbidden | **`ErrRateLimited`** (GitHub conflates 403 w/ rate limit) | passes through to generic? (no 403 branch -> generic >=400) | **`ErrRateLimited`** | `github_client.go:70-73`; `gitee_client.go:67-69` | GitHub: scope-insufficient 403 is MISreported as rate-limit - test locks this known conflation |
| 429 too many requests | **UNHANDLED** -> generic `"GitHub API error 429"` | **`ErrRateLimited`** | unhandled? (403-based) -> generic | `gitlab_client.go:62-64`; GitHub `grep 429`->none | GitHub 429 yields opaque error; test asserts current gap, flag for fix |
| 5xx / connection error / timeout | generic `fmt.Errorf("GitHub API error %d")` or transport err | generic | generic | `github_client.go:75-79` | non-typed error propagates; handler -> `CodeInternal`/`CodeUnavailable`; UI fallback |
| Malformed / invalid JSON body | decode error returned | decode error | decode error (test exists `gitee_error_branch_test.go:11`) | `github_branch.go:26-28` | error surfaced, not panic |
| Pagination (repo with > default page of branches) | **NOT paginated** - `ListBranches` issues a single `GET /repos/{id}/branches` with no `per_page`/page loop | **NOT paginated** - single `GET .../branches` | **NOT paginated** | `github_branch.go:11-12`; `gitlab_branch.go:10-12` | KNOWN LIMITATION: repos with >30 (GitHub default) / >20 (GitLab) branches return a truncated list. Test asserts the truncation (lock the gap) and flags that the combobox must remain editable so users can type un-listed branches |
| Huge / unusual branch names (slashes, unicode) | passed through verbatim in list; `GetBranch` path-escapes (`github_branch.go:50`) but `ListBranches` does not need to | same | same | - | combobox must render long/slashy names; FE test with `feat/very/deep/name` |
| default_branch info present? | GitHub: derived via extra `GetProject` call (`github_branch.go:30-34`); if `GetProject` fails it is silently `""` (error ignored, `_`) | GitLab/Gitee: `default` bool in payload | - | `github_branch.go:30` | GitHub default-branch is best-effort; test the `GetProject`-fails-> no default-marked case |
| Network partition (DNS / dial fail) | transport error from `httpClient.Do` | same | same | `github_client.go:57-60` | error propagates (not a typed sentinel); handler maps to internal/unavailable |

Net: **handled** = 401/404 (all three), 403/429 as rate-limit (provider-specific,
inconsistent). **Unhandled / gaps to flag** = GitHub 429 (opaque), GitHub 403
scope-vs-ratelimit conflation, NO pagination on any provider, GitHub default-branch
silently empty on `GetProject` failure.

---

## 5. Product-code changes required to make scenarios testable (seams)

| ID | Change | Why | Unblocks |
|---|---|---|---|
| **S1** | Add an injectable `git.Provider` factory to `repository.Service` (constructor param / settable field) instead of calling package-level `git.NewProvider` in `service_sync.go:52`. | No seam today (F2) - service unit tests cannot fake the provider; only the GetByID short-circuit is reachable. | All `TestListBranches_Success_*` / `_ProviderError_*` at the service layer (1.2). Integration-lite (1.3) can avoid this via httptest `ProviderBaseURL`. |
| **S2** | Give `Service.ListBranches` access to `userService.GetDecryptedProviderTokenByTypeAndURL` (inject `userService` into `Service`, or relocate resolution). Add typed sentinel `ErrNoGitCredential`. | Token resolution lives on `WebhookService` not `Service` (F3); the typed fallback error (state 1a) needs a sentinel the handler can map. | `TestListBranches_NoCredential_TypedFallbackError`, handler `_NoCredential_TypedCode`, `_EmptyToken_ResolvesServerSide`. |
| **S3** | Add `ticket_slug` to the pod realtime event (`event_data.proto` + `eventbus_pod.go:43-46`); consume it in `realtimePodHandlers.ts:25-29` -> `invalidateTicketPods`. | The event has no ticket linkage (F4); the realtime half of Feature 2 cannot scope an invalidation. | Feature 2 states B/C (handler + E2E). |
| **S4** | Add a per-request / per-repoId stale-guard token in `useRepositoryBranches` so a late response for repo A cannot overwrite repo B. | The `useTicketPods` inflight pattern dedupes but has no ordering guard across *different* keys mid-switch. | Concurrency 6b (`stale response ... dropped after switch`). |
| **S5** (prereq, not a test seam) | Replace the empty-token hard-reject (`repository_branches.go:28-30`, `repositories_branches.go:41-44`) with resolve-then-validate; have Rust core stop forcing `access_token: ""` or accept server resolution. | F1 - the feature is non-functional end-to-end until this lands; E2E specs in 1.5 will fail without it. | All Feature-1 E2E + integration happy paths. |

Without S1/S2 the backend Feature-1 unit coverage is limited to the provider
layer (`infra/git`) and the integration-lite httptest path. Without S3 only the
SpawnPodButton half of Feature 2 is covered. These should be implemented as part
of the feature, not deferred - flag in the PR.

---

## 6. New/changed test file inventory

Backend:
- `backend/internal/infra/git/github_branch_test.go` (extend: empty/single/default/401/403/malformed/429-gap)
- `backend/internal/infra/git/gitlab_branch_test.go` (extend: 429/empty/single/default-flag)
- `backend/internal/infra/git/gitee_branch_test.go` + `gitee_error_branch_test.go` (extend: 403/parity)
- `backend/internal/service/repository/service_branches_test.go` (new; needs S1/S2)
- `backend/internal/service/repository/service_branches_integration_test.go` (new; httptest provider)
- `backend/internal/api/connect/repository/repository_test.go` (extend: branch-handler guards + token resolution + mapServiceError row)

Frontend (Vitest):
- `clients/web/src/hooks/__tests__/useRepositoryBranches.test.ts` (new)
- `clients/web/src/components/pod/CreatePodForm/__tests__/BranchCombobox.test.tsx` (new)
- `clients/web/src/components/tickets/__tests__/TicketDetailSidebar.podRefresh.test.tsx` (new)
- `clients/web/src/providers/__tests__/realtimePodHandlers.test.ts` (new; B/C gated on S3)

E2E (Playwright):
- `clients/web/e2e-playwright/tests/pods/branch-dropdown.spec.ts` (new; gated on S5)
- `clients/web/e2e-playwright/tests/tickets/ticket-pod-refresh.spec.ts` (new; state A now, B gated on S3)
- `clients/web/e2e-playwright/pages/create-pod.page.ts` (new page object)
