# Design: Ticket label get-or-create + label updates

Rev 2 - 2026-06-19 - post-adversarial-review (13 findings + 2 user decisions applied: 3 critical, 5 high, 3 medium, 2 low)
Review: 3-agent adversarial team (adversarial-tl-reviewer, reviewer-coverage, socratic-challenger) + 2 user decisions

## Problem

Attaching labels to tickets silently fails across multiple surfaces. The root
cause is a shared "attach-if-exists-only" loop plus a missing update path.

1. **Create silently drops unknown labels.** `CreateTicketAtomic`
   (`backend/internal/infra/ticket_repo.go`, the `LabelNames` loop) iterates
   `LabelNames`, looks each up by `(organization_id, name)`, and
   `continue // skip unknown labels` when not found. No error is returned; the
   ticket is created with fewer (or zero) labels than requested. This loop is
   shared by **both** the ext REST create path and the Connect/UI create path -
   every real caller sends label *names* (`LabelIDs` is dead code, populated by
   no caller).
2. **Labels cannot be updated via ext REST.** `UpdateTicketRequest`
   (`backend/internal/api/rest/v1/tickets.go`) has no `Labels` field, and the
   handler builds a generic `map[string]interface{}` that never touches labels.
3. **Labels cannot be updated via Connect/UI.** The Connect `UpdateTicket`
   handler's `buildUpdateMap` ignores the proto `labels` field, so the UI cannot
   change a ticket's labels either.
4. **Labels cannot be set via the agent-facing MCP tools.** Neither
   `create_ticket` nor `update_ticket` MCP tools expose a `labels` argument, so
   agent pods cannot label tickets at all.

## Scope

In scope (locked with user 2026-06-19):
- Backend get-or-create in the shared create loop (fixes REST + Connect create).
- A shared `ReplaceTicketLabels` service + `ReplaceLabels` repo primitive.
- Wire label replacement into the ext REST `UpdateTicket` PUT.
- Wire label replacement into the Connect `UpdateTicket` handler.
- Wire labels into the agent-facing MCP create AND update tools (D-USER-2). The
  runner<->backend transport (`McpRequest.payload`) is JSON-generic, so NO
  runner proto change is needed; the service `CreateTicketRequest` already has
  `Labels []string`.

Out of scope (see "Out of scope / follow-ups" for the full list):
- `MetaOrchestrator/scripts/agentsmesh_backlog_sync.py` (direct-SQL, bypasses the
  backend).
- Any proto schema change. The Connect `labels` field stays `repeated string`;
  the runner `McpRequest.payload` stays JSON-generic.
- A standalone ext label-CRUD route (Connect already has label CRUD).

## Locked decisions (DO-NOT-CHANGE)

1. Get-or-create lives in the shared `CreateTicketAtomic` `LabelNames` loop, via
   a `tx`-scoped helper `getOrCreateLabel(tx, orgID, name) (int64, error)` reused
   by the replace path. Not duplicated per transport - both REST and Connect
   create benefit.
2. **Lookup AND create both operate at org level (`repository_id IS NULL`).**
   `getOrCreateLabel` resolves by
   `(organization_id = ? AND name = ? AND repository_id IS NULL)` and, on miss,
   creates with `repository_id = NULL`. Lookup scope and create scope MUST agree
   (mismatched scopes caused F-ADV-03's non-determinism). In practice every
   existing label is org-level, so this matches current data.
3. Auto-created labels are org-level with color set **explicitly** to `#6B7280`
   on the `Label{}` struct. Do NOT rely on the DB default: the in-memory test
   schema defaults `labels.color` to `#808080`, so a test asserting `#6B7280`
   fails unless the value is set explicitly (F-COV-01).
4. **Update is replace-all with transport-divergent presence (D-USER-1).** REST
   and MCP carry full tri-state via `*[]string`: `nil`/absent = labels
   untouched; `&[]` (empty) = clear all labels; `&[...]` (non-empty) = the
   ticket's label set becomes exactly that list. Connect uses proto3
   `repeated string` (no presence on the wire): non-empty = replace the set;
   empty/absent = **no-op (labels untouched)**. Clearing all labels over Connect
   uses the existing per-label `RemoveLabel` RPC. The transports diverge by
   design - see "Design patterns enforced" Pattern A.
5. **Name validation, all paths (create, replace, MCP).** Each name is trimmed
   before lookup/create; names empty after trim are skipped; a name longer than
   100 chars after trim is a request error `ErrInvalidLabelName`
   (HTTP 400 / Connect `CodeInvalidArgument`). No slugkit normalization beyond
   trim; case is preserved (`Bug` != `bug`). Case-fold is an optional follow-up
   (see Risks).
6. **Atomicity is per-transaction, NOT all-or-nothing across aspects.**
   `getOrCreateLabel` + links run inside one tx for create, and one tx for
   replace (delete-then-link). The "no orphan labels-without-links visible to
   the request" guarantee is scoped to single-tx rollback only; it does NOT
   cover auto-created labels that outlive their links across requests
   (F-ADV-10), nor cross-aspect partial state. **Observable contract:** field,
   assignee, and label updates are separate service calls / separate txs and are
   best-effort-per-aspect - a label-update failure leaves prior field/assignee
   updates committed (F-ADV-13).
7. No new event types. Label links emit no ticket events (preserve current
   behavior); `UpdateTicket`'s existing single event is unchanged. Implicit
   label creation is logged at info, distinguishable from explicit Connect label
   CRUD.
8. **`ReplaceLabels`/`ReplaceTicketLabels` verifies the ticket belongs to
   `orgID` inside the tx** (guard query; `ErrTicketNotFound` on mismatch).
   Defense-in-depth at the primitive, not just the current callers (F-ADV-05).
9. **`getOrCreateLabel` is concurrent-insert-safe** against
   `UNIQUE(organization_id, repository_id, name)`: use
   `INSERT ... ON CONFLICT (organization_id, repository_id, name) DO NOTHING`
   then re-`SELECT` (or catch the unique violation and re-run the lookup). Never
   bare find-then-create (F-ADV-01).
10. **De-dup label names before linking, on BOTH create and replace** - after
    trim AND after id resolution (two distinct names can resolve to one id). The
    prod child PK `(ticket_id, label_id)` would otherwise be violated; this fix
    removes the old `continue`-skip that masked the problem (F-ADV-04).

## Design

### Repo layer (`backend/internal/infra/`)

- **`ticket_repo.go` - get-or-create in the `LabelNames` loop.** Replace the
  `continue // skip unknown labels` branch with `getOrCreateLabel`. The helper:
  trims the name (skip if empty, error `ErrInvalidLabelName` if > 100 chars),
  runs the conflict-safe lookup-or-create
  (`SELECT ... WHERE organization_id = ? AND name = ? AND repository_id IS NULL`;
  on miss `INSERT ... ON CONFLICT (organization_id, repository_id, name) DO
  NOTHING` with `Color: "#6B7280"`, `repository_id = NULL`, then re-`SELECT`),
  and returns the label id. De-dup resolved ids before creating
  `TicketLabel` rows so a single tx never double-links the same id. The helper
  is `tx`-scoped - it does NOT call the non-tx `GetLabelByOrgNameRepo`
  (which uses `r.db`); it issues its own queries against `tx`.
- **`ticket_repo_label.go` - new `ReplaceLabels(ctx, ticketID, orgID int64,
  names []string) error`.** Transactional. First a tenant-guard query: confirm
  the ticket exists with `organization_id = orgID` (else `ErrTicketNotFound`).
  Then `DELETE FROM ticket_labels WHERE ticket_id = ?`; for each name call
  `getOrCreateLabel`; de-dup the resolved ids; create one `TicketLabel` per
  distinct id. Follows the `ReplaceAssignees` shape.
- **`repository.go` interface** gains `ReplaceLabels(...)`.

### Service layer (`backend/internal/service/ticket/`)

- **`label_service.go` - new `ReplaceTicketLabels(ctx, ticketID, orgID int64,
  names []string) error`.** Validates names (trim, skip-empty, `len <= 100` else
  `ErrInvalidLabelName`); delegates to `repo.ReplaceLabels`. Logs at info.
- **`service_types.go`** gains `ErrInvalidLabelName`.
- Get-or-create on create needs no service change - it already passes
  `LabelNames` to the repo; behavior changes inside the repo loop only. Trim,
  skip-empty, and length validation are pushed into the repo loop / helper so
  every caller (create, replace, MCP) is covered uniformly (F-ADV-07, F-ADV-08).

### Ext REST (`backend/internal/api/rest/v1/tickets.go`)

- `UpdateTicketRequest` gains `Labels *[]string \`json:"labels"\`` (full
  tri-state per decision #4).
- `UpdateTicket` handler: after the existing field-map update, if
  `req.Labels != nil`, call `ticketService.ReplaceTicketLabels(ctx, t.ID,
  tenant.OrganizationID, *req.Labels)`. Map `ErrInvalidLabelName` to 400,
  `ErrTicketNotFound` to 404.
- **Re-fetch the ticket AFTER the label mutation and return that.** The existing
  handler re-fetches BEFORE the label call, so the response shows stale labels
  (F-ADV-06). Add an explicit post-mutation re-fetch.

### Connect (`backend/internal/api/connect/ticket/ticket_update.go`)

- The proto field is `repeated string labels` with no presence (D-USER-1).
  After the existing update + `UpdateAssignees` block, if
  `len(req.Msg.Labels) > 0`, call `ReplaceTicketLabels` with the tenant org.
  An empty/absent list is a **no-op** - do NOT clear; clearing is via the
  existing `RemoveLabel` RPC.
- **Re-fetch the ticket AFTER the label mutation and return that** (same stale-
  response defect as REST; F-ADV-06).
- Map `ErrInvalidLabelName` to `CodeInvalidArgument`, `ErrTicketNotFound` to
  `CodeNotFound`.

### MCP (agent-facing)

The runner<->backend transport is JSON-generic (`McpRequest.payload`), so NO
runner proto change is needed; the backend service `CreateTicketRequest` already
has `Labels []string`.

- **Create** (`labels` = full replace; JSON list, no tri-state needed - absent
  list just means no labels):
  - Add a `labels` array to `createCreateTicketTool`'s InputSchema
    (`runner/internal/mcp/http_tools_ticket_write.go`).
  - Add `labels` to the `CreateTicket` method on the `tools.CollaborationClient`
    interface (`runner/internal/mcp/tools/types_client.go`) and thread it into
    the params map of `GRPCCollaborationClient.CreateTicket`
    (`runner/internal/mcp/grpc_client_ticket.go`).
  - Add `Labels []string \`json:"labels"\`` to the backend MCP adapter struct
    and set `Labels: params.Labels` in the `CreateTicketRequest` build
    (`backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go`).
- **Update** (`labels` = `*[]string`, JSON carries presence so full tri-state
  like REST):
  - Add a `labels` array to `createUpdateTicketTool`'s InputSchema and thread it
    through the `UpdateTicket` method on `tools.CollaborationClient` +
    `GRPCCollaborationClient.UpdateTicket` params map.
  - In the backend MCP update adapter, decode `Labels *[]string` and, when
    non-nil, call `ReplaceTicketLabels`. Re-fetch after the mutation (same
    F-ADV-06 fix as REST/Connect).

## Failure -> status mapping (F-ADV-09)

| Condition | REST | Connect |
|---|---|---|
| Invalid / too-long name (after trim) | 400 | `CodeInvalidArgument` |
| Ticket not in caller's org | 404 | `CodeNotFound` |
| Concurrent insert conflict (handled internally via retry) | 200 | `CodeOK` |
| Any other error | 500 | `CodeInternal` |

## Test plan

White-box `package ticket`, in-memory SQLite, `newTestService(db)` (per
`service_setup_test.go`). New/extended tests:

1. **Create get-or-create**: `CreateTicket` with `Labels: ["brand-new"]` where
   no such label exists -> ticket has 1 label; the label row now exists,
   org-level (`repository_id IS NULL`), color `#6B7280` (asserted explicitly -
   the test schema default is `#808080`).
2. **Create reuses existing**: seed label `bug`; create with `Labels: ["bug"]`
   -> reuses the seeded row (label count unchanged).
3. **Replace-all**: ticket with `[a, b]`; `ReplaceTicketLabels(ticket,
   ["b", "c"])` -> labels are exactly `{b, c}`; `c` auto-created; `a` link
   removed.
4. **Clear**: `ReplaceTicketLabels(ticket, [])` -> zero labels.
5. **Untouched**: ext `UpdateTicket` with `Labels == nil` -> labels unchanged.
6. **Validation**: name > 100 chars (after trim) -> `ErrInvalidLabelName`;
   whitespace/empty name skipped, and the resulting set is the requested set
   minus skipped entries (decision #5 caveat).
7. **De-dup + atomicity** (split from the old item 7):
   - (a) create with duplicate names (`["x", "x"]`, and `["x", " x "]` that trim
     to the same name) -> exactly one link;
   - (b) replace with duplicate names -> exactly one link;
   - (c) forced mid-tx error -> full rollback (no partial link state).
8. **Tenant guard**: `ReplaceTicketLabels` for a ticket in another org ->
   `ErrTicketNotFound`, no rows touched.
9. **Connect transport**: empty `labels` over the Connect update path -> no-op
   (labels unchanged).
10. **MCP path**: create via the MCP tool with `labels` -> ticket gets the
    labels (exercises the adapter wiring, not just the service).

**Test-harness note (load-bearing).** The in-memory SQLite `ticket_labels`
table has a surrogate `id` PK and NO composite `(ticket_id, label_id)` PK and NO
`labels` UNIQUE constraint (verified in
`backend/internal/testkit/schema_business.go`). So de-dup and concurrency
correctness MUST be enforced in CODE and asserted by link/label COUNT - the test
DB will NOT raise a constraint violation the way prod does.

Existing `TestCreateTicket_TableDriven` "with label names" case must still pass
unchanged (existing-label resolution path is preserved).

## Risks

- **Auto-created labels accumulate across requests.** A typo now creates a
  permanent org label rather than silently dropping it. Decision #6's no-orphan
  guarantee is single-tx only - it does not GC labels whose links are later
  removed. Orphan-label GC is a tracked follow-up. Acceptable: labels are cheap
  and UI-deletable.
- **Case-sensitivity proliferation.** `Bug` and `bug` produce two distinct
  labels (decision #5 preserves case). Accepted; optional case-fold is a tracked
  follow-up.
- **Implicit label creation is available to any `tickets:write` caller**,
  including low-privilege MCP agent pods. This is an accepted privilege,
  consistent with the fact that those callers already create ticket content;
  implicit creation is logged at info for traceability.
- **Cross-aspect updates are best-effort-per-aspect** (decision #6). A request
  that updates fields, assignees, and labels is three separate transactions; a
  label failure leaves the field/assignee changes committed. This is the
  observable contract, not a bug (F-ADV-13).
- **Blast radius on the UI create path.** Connect create passes raw names
  through the same loop, so it gains get-or-create too. Desirable and matches
  user intent; documented so it is an explicit decision.

## Design patterns enforced

- **Pattern A - presence-tri-state needs a presence-carrying transport.** Any
  nil/empty/set tri-state field must be backed by a type with presence on every
  transport it crosses. proto3 `repeated` and bare Go slices lack presence;
  `*[]string` (REST/MCP JSON) has it. This is why Connect diverges (D-USER-1):
  it cannot distinguish absent from `[]`, so empty = no-op and clearing routes
  through `RemoveLabel`.
- **Pattern B - get-or-create under a UNIQUE constraint needs conflict
  handling, scope agreement, and input de-dup.** Lookup scope must equal create
  scope (decision #2); the insert must be conflict-safe (decision #9); inputs
  must be de-duped after resolution (decision #10).

## Out of scope / follow-ups

- **`MetaOrchestrator/scripts/agentsmesh_backlog_sync.py`** writes directly to
  Postgres and bypasses the backend. End-state to be plain about: **this backend
  fix does NOT fix the visible backlog-board labels on its own** - the board
  stays stale until that script is separately fixed (Socratic Q9).
- **Backfill of labels dropped during the broken window: none.** Losses are not
  auto-recovered; the 2026-06-18 incident manually backfilled the affected
  tickets (Socratic Q7).
- **Orphan-label GC** and **optional case-folding**: tracked follow-ups, not in
  this fix.
- **Stale in-code line citations** (F-COV-04): refresh opportunistically during
  implementation; no spec change required here, which is why this spec cites
  files by symbol/function rather than by line number.

## Resolutions Applied in Rev 2

- **Rev 1 decision #2 withdrawn.** Rev 1 matched an existing label by
  `(organization_id, name)` against ANY repository scope via `First` and only
  changed the not-found branch. That left lookup scope (any) and create scope
  (org-level NULL) disagreeing, producing F-ADV-03 non-determinism. Rev 2 fixes
  both to org-level `repository_id IS NULL`. Verified the current loop uses no
  `repository_id` filter (`backend/internal/infra/ticket_repo.go` `LabelNames`
  loop) and that prod `labels` carries
  `UNIQUE(organization_id, repository_id, name)`
  (`backend/migrations/000001_init_schema.up.sql`).
- **Color set explicitly.** Verified prod default is `#6B7280` and the test
  schema default is `#808080` (`backend/internal/testkit/schema_business.go`),
  so the explicit set is required for the test assertion (decision #3).
- **MCP gRPC-client path corrected.** The curator brief cited
  `runner/internal/mcp/tools/grpc_client_ticket.go`; that path does not exist.
  The actual file is `runner/internal/mcp/grpc_client_ticket.go` (the
  `GRPCCollaborationClient.CreateTicket` params-map lives there). The
  `CollaborationClient` interface (with `CreateTicket`/`UpdateTicket`) is in
  `runner/internal/mcp/tools/types_client.go`, as the brief stated. The MCP
  subsection above uses the corrected paths.
- **Conservative interpretation, Connect non-op threshold.** D-USER-1 says "a
  non-empty labels list replaces the set; an empty/absent list is a no-op". The
  guard is written as `len(req.Msg.Labels) > 0` so that both absent and `[]`
  (indistinguishable on the proto3 wire) take the no-op branch - the most
  conservative reading that never silently clears labels.
