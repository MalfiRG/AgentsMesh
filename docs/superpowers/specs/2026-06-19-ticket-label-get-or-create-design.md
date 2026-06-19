# Design: Ticket label get-or-create + label updates

Rev 1 - 2026-06-19

## Problem

Attaching labels to tickets silently fails across multiple surfaces. The root
cause is a shared "attach-if-exists-only" loop plus a missing update path.

1. **Create silently drops unknown labels.** `CreateTicketAtomic`
   (`backend/internal/infra/ticket_repo.go:137-145`) iterates `LabelNames`,
   looks each up by `(organization_id, name)`, and `continue // skip unknown
   labels` when not found. No error is returned; the ticket is created with
   fewer (or zero) labels than requested. This loop is shared by **both** the
   ext REST create path and the Connect/UI create path - every real caller
   sends label *names* (`LabelIDs` is dead code, populated by no caller).
2. **Labels cannot be updated via ext REST.** `UpdateTicketRequest`
   (`backend/internal/api/rest/v1/tickets.go:52-59`) has no `Labels` field, and
   the handler builds a generic `map[string]interface{}` that never touches
   labels.
3. **Labels cannot be updated via Connect/UI.** The Connect `UpdateTicket`
   handler's `buildUpdateMap` ignores the proto `labels` field, so the UI
   cannot change a ticket's labels either.

## Scope

In scope (locked with user 2026-06-19):
- Backend get-or-create in the shared create loop (fixes REST + Connect create).
- A shared `ReplaceTicketLabels` service+repo primitive.
- Wire label replacement into the ext REST `UpdateTicket` PUT.
- Wire label replacement into the Connect `UpdateTicket` handler.

Out of scope:
- `MetaOrchestrator/scripts/agentsmesh_backlog_sync.py` (talks to Postgres
  directly, bypasses the backend; separate follow-up).
- Label color selection on create (auto-created labels take the DB default).
- Any UI/proto schema change beyond consuming the existing `labels` field.
- A standalone ext label-CRUD route (Connect already has label CRUD; create
  get-or-create removes the need for it on the ext path).

## Locked decisions (DO-NOT-CHANGE)

1. **Get-or-create lives in the shared `CreateTicketAtomic` `LabelNames` loop**,
   not duplicated per transport. Both REST and Connect create benefit.
2. **Lookup semantics on existing labels are preserved.** Resolve an existing
   label first by `(organization_id, name)` matching ANY repository scope (the
   current behavior, via `First`). Only the not-found branch changes: from
   `continue` (skip) to create.
3. **Auto-created labels are org-level** (`repository_id = NULL`) with the
   **DB-default color `#6B7280`**. This matches how section labels and the
   backlog board already work.
4. **Update is replace-all (PUT semantics).** The label field is `*[]string`:
   `nil`/absent = labels untouched; `&[]` (empty) = clear all labels;
   `&[...]` = the ticket's label set becomes exactly that list (each name
   get-or-created). Mirrors the existing `ReplaceAssignees` pattern.
5. **Name validation:** empty/whitespace-only names are skipped; a name longer
   than 100 chars (the `labels.name` column limit) is a request error, not a
   silent truncation. No slugkit/identifier normalization (label `name` is a
   free-form presentation string with no existing validation).
6. **Replacement is atomic.** `ReplaceTicketLabels` runs delete-then-link inside
   one transaction (like `ReplaceAssignees`), and get-or-create of each label
   happens in the same transaction so a mid-operation failure leaves no
   orphaned labels-without-links visible to the request.
7. **No new event types.** Label links do not emit ticket events today (the
   2026-06-18 direct-SQL workaround noted this); we keep that. `UpdateTicket`'s
   existing single event is unchanged.

## Design

### Repo layer (`backend/internal/infra/`)

- **`ticket_repo.go` - get-or-create in the `LabelNames` loop.** Replace the
  `continue // skip unknown labels` branch: on not-found, create an org-level
  `Label{OrganizationID, Name, Color: "#6B7280"}` inside `tx`, then link it.
  Extract a small `tx`-scoped helper `getOrCreateLabel(tx, orgID, name)
  (int64, error)` so the same logic is reused by replace.
- **`ticket_repo_label.go` - new `ReplaceLabels(ctx, ticketID, orgID int64,
  names []string) error`.** Transactional: `DELETE FROM ticket_labels WHERE
  ticket_id = ?`, then for each name `getOrCreateLabel` + create `TicketLabel`.
  Skip empty names. Follows `ReplaceAssignees` shape.
- **`repository.go` interface** gains `ReplaceLabels(...)`.

### Service layer (`backend/internal/service/ticket/`)

- **`label_service.go` - new `ReplaceTicketLabels(ctx, ticketID, orgID int64,
  names []string) error`.** Validates names (`len <= 100`, else
  `ErrInvalidLabelName`); delegates to `repo.ReplaceLabels`. Logs at info.
- **`service_types.go`** gains `ErrInvalidLabelName`.
- Get-or-create on create needs no service change - it already passes
  `LabelNames` to the repo; behavior changes inside the repo loop only.

### Ext REST (`backend/internal/api/rest/v1/tickets.go`)

- `UpdateTicketRequest` gains `Labels *[]string \`json:"labels"\``.
- `UpdateTicket` handler: after the existing field-map update, if
  `req.Labels != nil`, call `ticketService.ReplaceTicketLabels(ctx, t.ID,
  tenant.OrganizationID, *req.Labels)`. Map `ErrInvalidLabelName` to 400.
- Re-fetch ticket so the response reflects new labels (handler already returns
  the updated ticket).

### Connect (`backend/internal/api/connect/ticket/ticket_update.go`)

- After the existing update + `UpdateAssignees` block, if `req.Msg.Labels !=
  nil` (proto `repeated string labels`), call `ReplaceTicketLabels` with the
  tenant org. Mirror the `AssigneeIds != nil` guard already present.

## Test plan

White-box `package ticket`, in-memory SQLite, `newTestService(db)` (per
`service_setup_test.go`). New/extended tests:

1. **Create get-or-create**: `CreateTicket` with `Labels: ["brand-new"]` where
   no such label exists -> ticket has 1 label; the label row now exists,
   org-level, color `#6B7280`.
2. **Create reuses existing**: seed label `bug`; create with `Labels: ["bug"]`
   -> reuses the seeded row (no duplicate; count of `labels` unchanged).
3. **Replace-all**: ticket with `[a, b]`; `ReplaceTicketLabels(ticket, ["b",
   "c"])` -> labels are exactly `{b, c}`; `c` auto-created; `a` link removed.
4. **Clear**: `ReplaceTicketLabels(ticket, [])` -> zero labels.
5. **Untouched**: ext `UpdateTicket` with `Labels == nil` -> labels unchanged.
6. **Validation**: name > 100 chars -> `ErrInvalidLabelName`; whitespace/empty
   name skipped.
7. **Atomicity / no orphan**: replace that links the same name twice or hits a
   forced error leaves a consistent set (no partial link state).

Existing `TestCreateTicket_TableDriven` "with label names" case must still pass
unchanged (existing-label resolution path is preserved).

## Risks

- **Blast radius on the UI create path.** Verified: Connect create passes raw
  names through the same loop, so it gains get-or-create too - this is
  desirable and matches the user's intent, but means a UI typo now creates a
  label rather than silently dropping it. Acceptable; labels are cheap and
  UI-deletable. Documented here so it is an explicit decision, not a surprise.
- **`ticket_labels` has no UNIQUE constraint in the test schema** (prod PK is
  `(ticket_id, label_id)`). Replace deletes first, so double-link within one
  replace is avoided by de-duping names before linking.
