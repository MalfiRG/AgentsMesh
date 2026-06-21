# Design: Lazy Branch Dropdown + Ticket-View Pod Refresh

Date: 2026-06-21
Status: Approved (pending spec review)

Two features in AgentsMesh:

1. Replace the free-text branch field in the Create Pod form with an editable,
   lazily-fetched branch combobox.
2. Fix pod state not refreshing inside the ticket view when a pod is
   created/run from within a ticket.

The companion test plan (state-transition tables, git-provider failure-mode
audit, file-by-file test inventory) lives at
`.codex-reviews/branch-dropdown-and-pod-refresh-test-plan.md` and is the
authoritative source for test naming and coverage.

---

## Feature 1 - Lazy branch dropdown (editable combobox)

### Decisions

- Editable combobox: branches are fetched and listed, but a typed value not in
  the list is still accepted (targeting a not-yet-existing branch stays
  possible, preserving today's free-text capability).
- On any fetch failure (no credential, provider error, SSH provider with no
  API), the control silently degrades to today's plain free-text input with a
  one-line hint. Pod creation is never blocked by a branch-listing failure.
- Fetch is lazy and opportunistic: nothing happens until the dropdown is
  opened; the result is cached per `repoId`; a manual refresh affordance
  re-fetches on demand. No auto-polling.

### Current state (discovered)

- The branch field is the free-text `BranchInput` in the Create Pod form's
  Advanced section (`clients/web/src/components/pod/CreatePodForm/RepositorySelect.tsx:45-85`,
  wired at `AdvancedFormSection.tsx:122`). Form state is `selectedBranch: string`
  (`clients/web/src/components/pod/hooks/useCreatePodFormTypes.ts:20,41`).
- The branch-listing data path exists end to end but is unused by any UI:
  web facade `listRepositoryBranches(orgSlug, id, accessToken)` ->
  `{ items: string[], ... }` (`clients/web/src/lib/api/connect/repositoryConnect.ts:172`,
  re-exported `facade/repositoryConnect.ts:11`); Rust core
  `list_repository_branches` (`clients/core/crates/ffi/src/services/repository.rs:55`);
  Connect handler `ListRepositoryBranches`
  (`backend/internal/api/connect/repository/repository_branches.go:18`); REST
  `ListBranches` (`backend/internal/api/rest/v1/repositories_branches.go:15`);
  service `ListBranches` (`backend/internal/service/repository/service_sync.go:46`),
  which calls `git.NewProvider(...).ListBranches(...)` live.

### Prerequisite: the path is non-functional today (S5)

Rust core sends `access_token: ""` and both server entry points reject an empty
token before the service runs. So the backend MUST replace the empty-token
hard-reject with resolve-then-validate. Rust core continues to send an empty
token (it never holds one); no Rust/WASM rebuild is required.

### Backend changes

- **S5 - resolve-then-validate.** In both the Connect handler
  (`repository_branches.go:28-30`) and REST handler
  (`repositories_branches.go:41-44`): when `access_token` is empty, resolve it
  server-side from the caller's default git credential. An explicitly-passed
  token still wins (back-compat). The handler already has `tenant.UserID`
  (`repository_branches.go:115`).
- **S2 - token resolution wiring.** Give `Service.ListBranches` access to
  `userService.GetDecryptedProviderTokenByTypeAndURL(ctx, userID, repo.ProviderType,
  repo.ProviderBaseURL)`, mirroring the precedent at
  `backend/internal/service/repository/webhook_registration.go:148`. Inject
  `userService` into `Service` (or relocate resolution). Add a typed sentinel
  `ErrNoGitCredential`; the handler maps it to a distinct code the frontend
  treats as "use free-text" (extend `mapServiceError`).
- **S1 - provider injection seam.** Add an injectable `git.Provider` factory to
  `repository.Service` instead of calling package-level `git.NewProvider` in
  `service_sync.go:52`, so the service happy-path is unit-testable with a fake
  provider.
- No proto change for Feature 1 - `access_token` stays optional on the wire.

### Frontend changes

- **New hook `useRepositoryBranches(repoId)`** (`clients/web/src/hooks/`).
  Lazy: no fetch until `load()` is called. Caches `{ items, status }` per
  `repoId` in a module-level inflight/listener map modeled on
  `clients/web/src/hooks/useTicketPods.ts`. Exposes `branches`, `loading`,
  `error`/`fallbackToFreeText`, `load()`, `refresh()`. **S4 - stale-response
  guard:** a per-request token keyed by `repoId` so a late response for repo A
  cannot overwrite repo B after a switch.
- **New `BranchCombobox`** replacing `BranchInput` usage in
  `AdvancedFormSection.tsx:122`. Editable input + dropdown list; opening triggers
  `load()`. Filters by typed text; a typed value not in the list is accepted.
  Default branch surfaced first with a "(default)" marker. On `error` it renders
  the existing `BranchInput` plain text fallback. Writes a plain string to
  `selectedBranch` - form-state type unchanged, so submit/validation
  (`useCreatePodFormSubmit`) need no change.

### Known git-provider gaps (locked by tests, flagged for follow-up)

GitHub has no 429 branch (opaque error) and conflates 403-scope with rate-limit;
none of the three providers paginate `ListBranches` (truncates at provider
default page). The editable combobox mitigates truncation (users can type an
unlisted branch). See the test plan's provider audit (section 4).

---

## Feature 2 - Ticket-view pod refresh

### Root cause (discovered)

`TicketDetailSidebar.tsx:60` renders `SpawnPodButton` with no `onPodCreated`
callback, and `realtimePodHandlers.ts` `handlePodEvent` never calls
`invalidateTicketPods()`. The correct shape already exists at
`SidebarPodSection.tsx:34-38` (`invalidateTicketPods(ticketSlug); void refresh()`).

The `pod:created` realtime event carries no ticket linkage - it is emitted as
`PodStatusChangedEventData{PodKey, Status, AgentStatus}`
(`backend/cmd/server/eventbus_pod.go:43-46`), so the handler has no slug to
invalidate with. Multi-window / other-user refresh therefore needs a wire change.

### Decision: full fix (same-tab + realtime)

- **Same-tab (works immediately):** pass `onPodCreated` into `SpawnPodButton`
  at `TicketDetailSidebar.tsx:60` calling `invalidateTicketPods(ticketSlug)` +
  refetch, matching `SidebarPodSection.tsx:34-38`. `SpawnPodButton` already
  forwards `onPodCreated` from its modal.
- **S3 - realtime (multi-window / other users):** add `ticket_slug` to the pod
  realtime event (`event_data.proto` + `eventbus_pod.go:43-46`). In
  `realtimePodHandlers.ts:25-29`, call `invalidateTicketPods(data.ticketSlug)`
  when present, scoped to the matching ticket only (no spurious refetch for
  unrelated tickets). Pods already carry the ticket FK
  (`agentpod/pod.go:58 TicketID`, `pod_commands.go:13 TicketSlug`) - the change
  is putting it on the wire event.

---

## Testing

Authoritative plan: `.codex-reviews/branch-dropdown-and-pod-refresh-test-plan.md`.

Layers and headline coverage:

- **Backend unit** - provider layer (`infra/git`, extend existing httptest
  tables): empty/single/many/default-branch/401/403/429-gap/malformed per
  provider. Service layer (needs S1/S2): success variants, provider-error
  propagation, no-credential typed error, explicit-token-wins.
- **Backend integration** - `service_branches_integration_test.go`: real
  `Service` + real DB + real `userService` (seeded encrypted credential) +
  provider faked at the HTTP boundary via httptest `ProviderBaseURL`; asserts
  resolved-token path, no-credential error, 5xx propagation. Connect-handler
  tests: guard tests, empty-token-resolves-server-side, `mapServiceError` row.
- **Frontend Vitest** - `useRepositoryBranches` (lazy fetch, inflight dedupe,
  stale-response guard on repo A->B switch, error->fallback for no-credential /
  provider-error / logout-mid-fetch); `BranchCombobox` (typed non-listed value
  accepted, filtering, free-text fallback, empty-list editable, slashy/unicode
  names); Feature-2 `TicketDetailSidebar` invalidation + `realtimePodHandlers`
  per-ticket scoping.
- **Playwright E2E** (`clients/web/e2e-playwright/`; providers faked at backend
  boundary, no `page.route`): lazy load on dropdown open, 5xx->free-text
  fallback, pick-branch->create-pod; ticket-pod-refresh without reload (same-tab)
  and realtime update (S3); multitab concurrency modeled on
  `channel-members-multitab.spec.ts`.

### Requested scenario coverage (positive + negative)

1. No auth token / no git credential -> typed fallback error -> UI free-text.
2. Token present, provider 5xx / connection error / timeout -> wrapped error ->
   fallback.
3. Token present, provider OK -> empty / single / many branches, default
   surfaced.
4. Repo added while panel open -> documented no-auto-refresh (cache is
   per-repoId; repo list has no live subscription in this panel).
5. Logout mid-fetch -> in-flight fetch rejects with auth error, no stale list.
6. Concurrency: per-tab inflight dedupe; independent across tabs; rapid repo
   switch dropped via stale-guard (S4).
7. Provider-direct failure modes audited: 401/404/403/429, pagination
   truncation, malformed JSON, network partition, default-branch best-effort.

---

## Seams summary (all implemented as part of this work)

| ID | Change |
|----|--------|
| S1 | Injectable `git.Provider` factory on `repository.Service`. |
| S2 | `userService` on `Service` + `ErrNoGitCredential` sentinel. |
| S3 | `ticket_slug` on the pod realtime event; consumed in `realtimePodHandlers`. |
| S4 | Per-`repoId` stale-response guard in `useRepositoryBranches`. |
| S5 | Replace empty-token hard-reject with resolve-then-validate (Connect + REST). |

## Scope guard (YAGNI)

No proto change for Feature 1. No DB branch caching. No auto-polling of
branches. No change to `selectedBranch` form-state type. Provider pagination and
GitHub 429 handling are flagged and locked by tests but fixing them is
out of scope (mitigated by the editable combobox).
