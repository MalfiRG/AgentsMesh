# Ticket Label Get-or-Create + Label Updates Implementation Plan

> **Rev 2 - 2026-06-19 - post-plan-hardening (traceability + coverage + adversarial; sentinel/alias, single-module commands, MCP signature targets, test-coverage honesty)**

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ticket labels work across all write surfaces - create get-or-creates labels by name, and labels can be replaced on update via REST, Connect/UI, and the agent-facing MCP tools.

**Architecture:** A single `tx`-scoped `getOrCreateLabel` helper in the infra repo backs both the shared `CreateTicketAtomic` loop and a new `ReplaceLabels` repo primitive. The service exposes `ReplaceTicketLabels`; REST/MCP wire it with `*[]string` tri-state, Connect wires it with a `len>0` no-op guard (proto3 `repeated` has no presence). No proto change, no DB migration.

**Tech Stack:** Go, Gin (REST), Connect-RPC, GORM, PostgreSQL 16. Single Go module: `go.mod` at repo root, module `github.com/anthropics/agentsmesh` (there is NO `backend/go.mod` or `runner/go.mod`). Tests: white-box `package ticket`, in-memory SQLite via `testkit.SetupTestDB`.

## Global Constraints

- Every file must stay under 200 lines (test files under 400). If `tickets.go` crosses 200 after edits, extract the label-handling block to a sibling file.
- No code comments that restate what the code does (project rule); comment only non-obvious why.
- ASCII hyphen only; never U+2014/U+2013.
- Auto-created label color is set EXPLICITLY to `#6B7280` (the in-memory test schema defaults `labels.color` to `#808080`, so relying on the DB default fails the color assertion).
- Lookup AND create both at org level: `organization_id = ? AND name = ? AND repository_id IS NULL`.
- Name handling on ALL paths: trim; skip if empty after trim; `len > 100` after trim is `ErrInvalidLabelName`. Case preserved (`Bug` != `bug`).
- De-dup names (after trim) and resolved ids before linking, on create AND replace.
- Source of truth: `docs/superpowers/specs/2026-06-19-ticket-label-get-or-create-design.md` (Rev 2).

## Resolutions Applied in Rev 2

- **R0 (ground truth):** All symbol names, signatures, file paths, and import aliases below are verified against the codebase. Single Go module; all build/test commands run from repo root.
- **R1 (error-sentinel design):** `ErrInvalidLabelName` lives in the DOMAIN package (`backend/internal/domain/ticket/label_errors.go`), superseding the earlier "service_types.go" note. The tenant guard reuses `gorm.ErrRecordNotFound` from the repo and the service translates it to the existing service-package `ErrTicketNotFound` (`backend/internal/service/ticket/service_types.go:13`). No new tenant sentinel.
- **R2 (commands):** Every `cd backend`/`cd ../runner` pair removed; all builds/tests run from repo root.
- **R3 (task merge):** Old Task 1 and old Task 2 merged into one task. Tasks renumbered 9 -> 8. Coverage map updated.
- **R4 (MCP):** Signature edits target the `TicketClient` interface (`runner/internal/mcp/tools/types_client.go:46-47`), the test mock is updated explicitly, and `newMcpError(code int32, msg string)` is used everywhere (there is NO `mcpErrorFrom`).
- **R5 (re-fetch):** `GetTicket(ctx, t.ID)` used for post-mutation re-fetch on all surfaces; re-fetch failure after a successful mutation is an internal error, never a 404 or stale 200.
- **R6 (test honesty):** Coverage labeled build/review-covered vs test-covered truthfully (forced-rollback, concurrency, nil-untouched, transport tests).
- **R7 (apierr arity):** All `apierr.BadRequest(c, msg)` calls replaced with `apierr.ValidationError(c, msg)`. Grep hint corrected to `backend/pkg/apierr`.
- **R8 (polish):** Repo-scoped auto-create explicitly out of scope; the `gorm.io/gorm/clause` + `strings` import-add note retained; the PG-NULL deviation kept as a one-line load-bearing comment.

## Deviation from spec (decision #9 refinement)

Spec decision #9 says get-or-create handles a unique-violation via `ON CONFLICT`. Verified against PG16 (`pgvector/pgvector:pg16`): the `UNIQUE(organization_id, repository_id, name)` constraint treats NULL `repository_id` as DISTINCT, so it does NOT enforce uniqueness for org-level labels and `ON CONFLICT` does not fire for them.
`Verified: backend/migrations/000001_init_schema.up.sql:479` (the UNIQUE constraint includes the nullable `repository_id`; PG16 treats NULL as DISTINCT by default).
Net: a concurrent insert of the same new org-level name produces a benign duplicate row, not an error/500. Resolution applied in this plan: keep `clause.OnConflict{DoNothing: true}` defensively (covers the repo-scoped path and a future partial index), AND make every lookup deterministic with `ORDER BY id ASC` so duplicate rows resolve to the same label. The safety here is "benign duplicate row + deterministic `ORDER BY id ASC` resolution on subsequent reads," NOT `ON CONFLICT`. Strict org-level uniqueness (a partial unique index `WHERE repository_id IS NULL`) is a tracked follow-up, not in this fix.

## Scope boundary (R8 / F-ADV-14)

Repo-scoped label auto-creation is intentionally unsupported: the get-or-create path only creates org-level labels, so `labels.repository_id` stays NULL on the auto-create path. Attaching a repo-scoped label requires that label to already exist.

---

## Task 1: get-or-create helper + name normalization + `ErrInvalidLabelName` + wire into `CreateTicketAtomic`

(Merge of old Task 1 + Task 2 per R3: the domain error, both infra helpers, the loop replacement, and all four create tests land in ONE green commit. No standalone red-test commit, no dead-code-until-next-task.)

**Files:**
- Create: `backend/internal/domain/ticket/label_errors.go`
- Modify: `backend/internal/infra/ticket_repo_label.go` (add helpers; add imports `strings` and `gorm.io/gorm/clause`)
- Modify: `backend/internal/infra/ticket_repo.go` (replace the `LabelNames` loop in `CreateTicketAtomic`, `ticket_repo.go:137-145`)
- Test: `backend/internal/service/ticket/service_label_getorcreate_test.go`

**Interfaces:**
- Produces: `ticket.ErrInvalidLabelName` (domain sentinel); `getOrCreateLabel(tx *gorm.DB, orgID int64, name string) (int64, error)` (unexported, infra); `normalizeLabelNames(names []string) ([]string, error)` (unexported, infra); create path now get-or-creates + de-dups labels.
- Consumes: `ticket.Label`, GORM `clause`, the existing `isNotFound(err)` helper (`backend/internal/infra/repo_helpers.go:9`, same `infra` package - call unqualified).

- [ ] **Step 1: Write the failing tests**

In `backend/internal/service/ticket/service_label_getorcreate_test.go`:

```go
package ticket

import (
	"context"
	"errors"
	"testing"

	ticket "github.com/anthropics/agentsmesh/backend/internal/domain/ticket"
)

func TestCreateTicket_GetOrCreatesUnknownLabel(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()

	created, err := service.CreateTicket(ctx, &CreateTicketRequest{
		OrganizationID: 1,
		ReporterID:     1,
		Title:          "needs a brand new label",
		Labels:         []string{"brand-new"},
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if len(created.Labels) != 1 || created.Labels[0].Name != "brand-new" {
		t.Fatalf("want 1 label brand-new, got %+v", created.Labels)
	}
	if created.Labels[0].Color != "#6B7280" {
		t.Fatalf("want color #6B7280 (set explicitly, not DB default), got %q", created.Labels[0].Color)
	}
	if created.Labels[0].RepositoryID != nil {
		t.Fatalf("want org-level label (repository_id NULL), got %v", *created.Labels[0].RepositoryID)
	}
}

func TestCreateTicket_ReusesExistingLabel(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	db.Exec(`INSERT INTO labels (organization_id, name, color) VALUES (1, 'bug', '#FF0000')`)

	var before int64
	db.Raw(`SELECT COUNT(*) FROM labels WHERE organization_id = 1`).Scan(&before)

	created, err := service.CreateTicket(ctx, &CreateTicketRequest{
		OrganizationID: 1, ReporterID: 1, Title: "reuse", Labels: []string{"bug"},
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	var after int64
	db.Raw(`SELECT COUNT(*) FROM labels WHERE organization_id = 1`).Scan(&after)
	if after != before {
		t.Fatalf("expected no new label row, before=%d after=%d", before, after)
	}
	if len(created.Labels) != 1 {
		t.Fatalf("want 1 label, got %d", len(created.Labels))
	}
}

func TestCreateTicket_DedupsDuplicateNames(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	created, err := service.CreateTicket(ctx, &CreateTicketRequest{
		OrganizationID: 1, ReporterID: 1, Title: "dup",
		Labels: []string{"x", "x", " x ", "  "},
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if len(created.Labels) != 1 || created.Labels[0].Name != "x" {
		t.Fatalf("want exactly 1 label x, got %+v", created.Labels)
	}
}

func TestCreateTicket_RejectsTooLongName(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	_, err := service.CreateTicket(ctx, &CreateTicketRequest{
		OrganizationID: 1, ReporterID: 1, Title: "toolong", Labels: []string{string(long)},
	})
	if !errors.Is(err, ticket.ErrInvalidLabelName) {
		t.Fatalf("want ErrInvalidLabelName, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from repo root): `go test ./backend/internal/service/ticket/ -run 'TestCreateTicket_(GetOrCreates|Reuses|Dedups|RejectsTooLong)' -v`
Expected: FAIL - labels are skipped (0 labels) because the current loop `continue`s on unknown; no dedup; no validation.

- [ ] **Step 3: Add the domain error**

In `backend/internal/domain/ticket/label_errors.go`:

```go
package ticket

import "errors"

var ErrInvalidLabelName = errors.New("invalid label name")
```

- [ ] **Step 4: Add helpers to `ticket_repo_label.go`**

Add imports `strings` and `gorm.io/gorm/clause` (clause is used in sibling infra files but is NOT yet imported in `ticket_repo_label.go` - add it). Append:

```go
const maxLabelNameLen = 100

func normalizeLabelNames(names []string) ([]string, error) {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" {
			continue
		}
		if len(n) > maxLabelNameLen {
			return nil, ticket.ErrInvalidLabelName
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func getOrCreateLabel(tx *gorm.DB, orgID int64, name string) (int64, error) {
	var label ticket.Label
	err := tx.Where("organization_id = ? AND name = ? AND repository_id IS NULL", orgID, name).
		Order("id ASC").First(&label).Error
	if err == nil {
		return label.ID, nil
	}
	if !isNotFound(err) {
		return 0, err
	}
	newLabel := ticket.Label{OrganizationID: orgID, Name: name, Color: "#6B7280"}
	// PG NULLS-distinct: UNIQUE(...,repository_id,...) does not fire for org-level labels; OnConflict is defensive only
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&newLabel).Error; err != nil {
		return 0, err
	}
	if newLabel.ID != 0 {
		return newLabel.ID, nil
	}
	if err := tx.Where("organization_id = ? AND name = ? AND repository_id IS NULL", orgID, name).
		Order("id ASC").First(&label).Error; err != nil {
		return 0, err
	}
	return label.ID, nil
}
```

In infra, `ticket` is the domain import, so `ticket.ErrInvalidLabelName` resolves to the domain sentinel - correct.

- [ ] **Step 5: Replace the `LabelNames` loop in `CreateTicketAtomic`**

In `backend/internal/infra/ticket_repo.go` (the skip-loop at `ticket_repo.go:137-145`), replace:

```go
	for _, name := range p.LabelNames {
		var label ticket.Label
		if err := tx.Where("organization_id = ? AND name = ?", p.Ticket.OrganizationID, name).First(&label).Error; err != nil {
			continue // skip unknown labels
		}
		if err := tx.Create(&ticket.TicketLabel{TicketID: p.Ticket.ID, LabelID: label.ID}).Error; err != nil {
			return err
		}
	}
```

with:

```go
	names, err := normalizeLabelNames(p.LabelNames)
	if err != nil {
		return err
	}
	seenIDs := make(map[int64]struct{}, len(names))
	for _, name := range names {
		labelID, err := getOrCreateLabel(tx, p.Ticket.OrganizationID, name)
		if err != nil {
			return err
		}
		if _, dup := seenIDs[labelID]; dup {
			continue
		}
		seenIDs[labelID] = struct{}{}
		if err := tx.Create(&ticket.TicketLabel{TicketID: p.Ticket.ID, LabelID: labelID}).Error; err != nil {
			return err
		}
	}
```

Note: `err` may already be declared earlier in the function; if so use `=` not `:=`, or rename to `nerr` to avoid shadowing - verify the surrounding scope.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./backend/internal/service/ticket/ -run 'TestCreateTicket_(GetOrCreates|Reuses|Dedups|RejectsTooLong)' -v`
Expected: PASS (all four).

- [ ] **Step 7: Regression smoke check (not validation)**

Run: `go test ./backend/internal/service/ticket/ -run TestCreateTicket -v`
Expected: PASS, including the pre-existing `TestCreateTicket_TableDriven` "with label names" case. That case is a SMOKE CHECK only - it does not assert label count, so it does NOT prove reuse or dedup. The real (count-based) proofs are `TestCreateTicket_ReusesExistingLabel` and `TestCreateTicket_DedupsDuplicateNames` above.

- [ ] **Step 8: Build everything from repo root**

Run: `go build ./...`
Expected: OK (this is a single module - any cross-subtree breakage surfaces here).

- [ ] **Step 9: Commit**

```bash
git add backend/internal/domain/ticket/label_errors.go backend/internal/infra/ticket_repo_label.go backend/internal/infra/ticket_repo.go backend/internal/service/ticket/service_label_getorcreate_test.go
git commit -m "feat(ticket): create path get-or-creates labels by name (helper + normalization + wiring)"
```

---

## Task 2: `ReplaceLabels` repo + `ReplaceTicketLabels` service (with tenant guard)

**Files:**
- Modify: `backend/internal/infra/ticket_repo_label.go` (add `ReplaceLabels`)
- Modify: `backend/internal/domain/ticket/repository.go` (add to interface `TicketRepository`, near label methods at lines 56-64)
- Modify: `backend/internal/service/ticket/label_service.go` (add `ReplaceTicketLabels`)
- Test: `backend/internal/service/ticket/service_label_replace_test.go`

**Interfaces:**
- Consumes: `getOrCreateLabel`, `normalizeLabelNames` (Task 1).
- Produces: repo `ReplaceLabels(ctx context.Context, ticketID, orgID int64, names []string) error`; service `(*Service).ReplaceTicketLabels(ctx context.Context, ticketID, orgID int64, names []string) error`.
- Sentinel identity (R1): the repo returns `gorm.ErrRecordNotFound` when the org-scoped count is 0 (it CANNOT import the service package - import cycle). The service translates that to the existing service sentinel `ErrTicketNotFound` (`backend/internal/service/ticket/service_types.go:13`). No new sentinel.

- [ ] **Step 1: Write failing tests**

In `backend/internal/service/ticket/service_label_replace_test.go`:

```go
package ticket

import (
	"context"
	"errors"
	"testing"
)

func seedTicket(t *testing.T, service *Service, org int64, title string) int64 {
	t.Helper()
	tk, err := service.CreateTicket(context.Background(), &CreateTicketRequest{
		OrganizationID: org, ReporterID: 1, Title: title,
	})
	if err != nil {
		t.Fatalf("seed CreateTicket: %v", err)
	}
	return tk.ID
}

func TestReplaceTicketLabels_ReplacesSet(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	id := seedTicket(t, service, 1, "rep")

	if err := service.ReplaceTicketLabels(ctx, id, 1, []string{"a", "b"}); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	if err := service.ReplaceTicketLabels(ctx, id, 1, []string{"b", "c"}); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	got, err := service.GetTicketLabels(ctx, id)
	if err != nil {
		t.Fatalf("GetTicketLabels: %v", err)
	}
	names := map[string]bool{}
	for _, l := range got {
		names[l.Name] = true
	}
	if len(names) != 2 || !names["b"] || !names["c"] {
		t.Fatalf("want {b,c}, got %v", names)
	}
}

func TestReplaceTicketLabels_ClearsAll(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	id := seedTicket(t, service, 1, "clear")
	_ = service.ReplaceTicketLabels(ctx, id, 1, []string{"a", "b"})

	if err := service.ReplaceTicketLabels(ctx, id, 1, []string{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ := service.GetTicketLabels(ctx, id)
	if len(got) != 0 {
		t.Fatalf("want 0 labels, got %d", len(got))
	}
}

func TestReplaceTicketLabels_WrongOrgRejected(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	id := seedTicket(t, service, 1, "tenant")

	err := service.ReplaceTicketLabels(ctx, id, 999, []string{"x"})
	if !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("want ErrTicketNotFound for wrong org, got %v", err)
	}
	got, _ := service.GetTicketLabels(ctx, id)
	if len(got) != 0 {
		t.Fatalf("ticket labels must be untouched on rejected replace, got %d", len(got))
	}
}
```

`ErrTicketNotFound` is referenced bare here because the test is `package ticket` in the service scope, where the sentinel lives.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./backend/internal/service/ticket/ -run TestReplaceTicketLabels -v`
Expected: FAIL - `service.ReplaceTicketLabels` undefined.

- [ ] **Step 3: Add `ReplaceLabels` to the repo**

Append to `backend/internal/infra/ticket_repo_label.go`. The repo returns `gorm.ErrRecordNotFound` on the tenant-guard miss (it cannot reference the service sentinel):

```go
func (r *ticketRepository) ReplaceLabels(ctx context.Context, ticketID, orgID int64, names []string) error {
	cleaned, err := normalizeLabelNames(names)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&ticket.Ticket{}).
			Where("id = ? AND organization_id = ?", ticketID, orgID).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Where("ticket_id = ?", ticketID).Delete(&ticket.TicketLabel{}).Error; err != nil {
			return err
		}
		seen := make(map[int64]struct{}, len(cleaned))
		for _, name := range cleaned {
			labelID, err := getOrCreateLabel(tx, orgID, name)
			if err != nil {
				return err
			}
			if _, dup := seen[labelID]; dup {
				continue
			}
			seen[labelID] = struct{}{}
			if err := tx.Create(&ticket.TicketLabel{TicketID: ticketID, LabelID: labelID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
```

The `tx.Transaction(...)` wrapper provides atomicity: a delete-then-insert that fails mid-way rolls back the whole replace.

- [ ] **Step 4: Add `ReplaceLabels` to the repo interface**

In `backend/internal/domain/ticket/repository.go`, add to the `TicketRepository` interface (declared at line 33; label methods at 56-64):

```go
	ReplaceLabels(ctx context.Context, ticketID, orgID int64, names []string) error
```

- [ ] **Step 5: Add the service method**

In `backend/internal/service/ticket/label_service.go` (`slog` is already imported; add `gorm.io/gorm` and `errors` if missing):

```go
func (s *Service) ReplaceTicketLabels(ctx context.Context, ticketID, orgID int64, names []string) error {
	if err := s.repo.ReplaceLabels(ctx, ticketID, orgID, names); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTicketNotFound
		}
		slog.ErrorContext(ctx, "failed to replace ticket labels", "ticket_id", ticketID, "org_id", orgID, "error", err)
		return err
	}
	slog.InfoContext(ctx, "ticket labels replaced", "ticket_id", ticketID, "org_id", orgID, "count", len(names))
	return nil
}
```

The repo's `gorm.ErrRecordNotFound` is translated to the service sentinel `ErrTicketNotFound`, so `TestReplaceTicketLabels_WrongOrgRejected` (`errors.Is(err, ErrTicketNotFound)`) passes. (Pattern A - see Plan-review patterns.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./backend/internal/service/ticket/ -run TestReplaceTicketLabels -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Build + commit**

Run: `go build ./...`

```bash
git add backend/internal/infra/ticket_repo_label.go backend/internal/domain/ticket/repository.go backend/internal/service/ticket/label_service.go backend/internal/service/ticket/service_label_replace_test.go
git commit -m "feat(ticket): ReplaceTicketLabels service + ReplaceLabels repo with tenant guard"
```

---

## Task 3: Ext REST UpdateTicket - labels field + wiring + re-fetch

**Files:**
- Modify: `backend/internal/api/rest/v1/tickets.go`
- Test: `backend/internal/service/ticket/service_label_replace_test.go` (extend - the nil-vs-empty contract is service-level; the handler skip itself is review-covered)

**Interfaces:**
- Consumes: `h.ticketService.ReplaceTicketLabels` (Task 2); `ticketDomain.ErrInvalidLabelName` (domain alias in this file), `ticket.ErrTicketNotFound` (service alias in this file).
- File facts (R0): `tickets.go` imports service as `ticket`, domain as `ticketDomain`; `errors` is NOT imported - add it. `tenant := middleware.GetTenant(c)`; use `tenant.OrganizationID`; service handle is `h.ticketService`; `GetTicket(ctx, id)` exists.
- apierr is `github.com/anthropics/agentsmesh/backend/pkg/apierr`: `ValidationError(c, message string)` (2-arg), `ResourceNotFound(c, message string)` (2-arg), `InternalError(c, message string)` (2-arg).

- [ ] **Step 1: Add `Labels` to `UpdateTicketRequest`**

```go
type UpdateTicketRequest struct {
	Title        string    `json:"title"`
	Content      *string   `json:"content"`
	Status       string    `json:"status"`
	Priority     string    `json:"priority"`
	RepositoryID *int64    `json:"repository_id"`
	DueDate      *string   `json:"due_date"`
	Labels       *[]string `json:"labels"`
}
```

- [ ] **Step 2: Wire into the `UpdateTicket` handler (add `"errors"` import)**

After the existing `t, err = h.ticketService.UpdateTicket(...)` call succeeds and BEFORE the JSON response, add:

```go
	if req.Labels != nil {
		if err := h.ticketService.ReplaceTicketLabels(c.Request.Context(), t.ID, tenant.OrganizationID, *req.Labels); err != nil {
			switch {
			case errors.Is(err, ticketDomain.ErrInvalidLabelName):
				apierr.ValidationError(c, "Invalid label name")
			case errors.Is(err, ticket.ErrTicketNotFound):
				apierr.ResourceNotFound(c, "Ticket not found")
			default:
				apierr.InternalError(c, "Failed to update labels")
			}
			return
		}
		t, err = h.ticketService.GetTicket(c.Request.Context(), t.ID)
		if err != nil {
			apierr.InternalError(c, "Failed to load updated ticket")
			return
		}
	}
```

`ticketDomain` is the domain alias, `ticket` is the service alias in this file. The post-mutation re-fetch is required - the earlier `UpdateTicket` re-fetch happened before labels changed (F-ADV-06). On re-fetch failure the ticket still exists, so this is `InternalError`, NOT a 404 (R5).

- [ ] **Step 3: File-size check**

Run: `wc -l backend/internal/api/rest/v1/tickets.go`
If > 200, extract the label block into `backend/internal/api/rest/v1/tickets_labels.go` as a helper `func (h *TicketHandler) applyLabelUpdate(c *gin.Context, t *ticketDomain.Ticket, labels *[]string) (*ticketDomain.Ticket, bool)` returning the refreshed ticket and an ok flag; call it from the handler.

- [ ] **Step 4: Add a service-level contract test (nil-vs-empty)**

This proves the empty-clears semantics the handler relies on. It does NOT prove the handler's `if req.Labels != nil` skip - that branch is review-covered (no httptest harness here).

```go
// In service_label_replace_test.go - proves [] clears; the nil-skip is in the handler (review-covered).
func TestReplaceTicketLabels_EmptyClears(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)
	ctx := context.Background()
	id := seedTicket(t, service, 1, "contract")
	_ = service.ReplaceTicketLabels(ctx, id, 1, []string{"keep"})
	if err := service.ReplaceTicketLabels(ctx, id, 1, []string{}); err != nil {
		t.Fatal(err)
	}
	got, _ := service.GetTicketLabels(ctx, id)
	if len(got) != 0 {
		t.Fatalf("empty slice must clear, got %d", len(got))
	}
}
```

- [ ] **Step 5: Run + build**

Run: `go build ./... && go test ./backend/internal/service/ticket/ -run TestReplaceTicketLabels -v`
Expected: build OK, tests PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api/rest/v1/ backend/internal/service/ticket/service_label_replace_test.go
git commit -m "feat(api): ext REST UpdateTicket accepts labels (*[]string tri-state) with re-fetch"
```

---

## Task 4: Connect UpdateTicket - labels wiring (no-op on empty) + re-fetch

**Files:**
- Modify: `backend/internal/api/connect/ticket/ticket_update.go`
- Modify: `backend/internal/api/connect/ticket/ticket_mount.go` (extend `mapServiceError`, defined at `ticket_mount.go:15`)

**Interfaces:**
- Consumes: `s.ticketSvc.ReplaceTicketLabels`; proto `req.Msg.GetLabels()` (`[]string`, proto3 repeated, no presence).
- File facts (R0): service field is `s.ticketSvc`; `tenant := middleware.GetTenant(ctx)`; `toProtoTicket` at `ticket_convert.go:13`; `errors` is NOT imported in `ticket_update.go` and stays unimported - the handler delegates ALL error mapping to `mapServiceError`. `mapServiceError` already maps service `ErrTicketNotFound` -> `CodeNotFound`, plus `ErrLabelNotFound`, `ErrInvalidTransition`; it does NOT yet handle the domain `ErrInvalidLabelName`.

- [ ] **Step 1: Extend `mapServiceError` to handle `ErrInvalidLabelName` (R1)**

In `backend/internal/api/connect/ticket/ticket_mount.go`, add a case to `mapServiceError` for the domain `ErrInvalidLabelName` (grep the file's imports for its domain alias and use it):

```go
	case errors.Is(err, <domainAlias>.ErrInvalidLabelName):
		return connect.NewError(connect.CodeInvalidArgument, err)
```

`ErrTicketNotFound` -> `CodeNotFound` is already mapped. Both Connect create (Task 5) and update handlers then call `mapServiceError(err)` only - no inline switch, no `errors` import added to the handler files.

- [ ] **Step 2: Wire labels after the `UpdateAssignees` block in `ticket_update.go`**

```go
	if len(req.Msg.GetLabels()) > 0 {
		if err := s.ticketSvc.ReplaceTicketLabels(ctx, t.ID, tenant.OrganizationID, req.Msg.GetLabels()); err != nil {
			return nil, mapServiceError(err)
		}
		refreshed, ferr := s.ticketSvc.GetTicket(ctx, t.ID)
		if ferr != nil {
			return nil, connect.NewError(connect.CodeInternal, ferr)
		}
		t = refreshed
	}
```

`len(...) > 0` is the deliberate no-op-on-empty guard (proto3 `repeated` has no presence; empty/absent both take the no-op branch). Clearing labels over Connect is via the existing `RemoveLabel` RPC. The re-fetch runs AFTER `ReplaceTicketLabels` (the existing `GetTicket` re-fetch in this handler runs before labels change). On re-fetch failure the call returns `CodeInternal` - it does NOT silently return the stale `t` with a 200 (R5).

- [ ] **Step 3: Build + targeted check**

Run: `go build ./... && go test ./backend/internal/api/connect/ticket/... 2>/dev/null || echo "no connect ticket test pkg - build-covered (see Test plan)"`
Expected: build OK. (Transport-level behavior is build-covered unless a Connect transport test harness exists - see Test plan.)

- [ ] **Step 4: Commit**

```bash
git add backend/internal/api/connect/ticket/ticket_update.go backend/internal/api/connect/ticket/ticket_mount.go
git commit -m "feat(api): Connect UpdateTicket replaces labels (empty=no-op), re-fetch after; map ErrInvalidLabelName"
```

---

## Task 5: Create-path error mapping (REST + Connect)

**Files:**
- Modify: `backend/internal/api/rest/v1/tickets.go` (CreateTicket handler)
- Modify: `backend/internal/api/connect/ticket/ticket_crud.go` (CreateTicket handler, `ticket_crud.go:124-127`)

**Interfaces:**
- Consumes: `ticketDomain.ErrInvalidLabelName` (REST); domain `ErrInvalidLabelName` via `mapServiceError` (Connect).

- [ ] **Step 1: REST CreateTicket - map invalid label name to a validation error**

In the REST `CreateTicket` handler, where the service error is currently handled (generic 500), add the branch (uses `ticketDomain` domain alias and the already-added `"errors"` import from Task 3):

```go
	if err != nil {
		if errors.Is(err, ticketDomain.ErrInvalidLabelName) {
			apierr.ValidationError(c, "Invalid label name")
			return
		}
		apierr.InternalError(c, "Failed to create ticket")
		return
	}
```

(Preserve any existing specific error mappings; only add the `ErrInvalidLabelName` branch.)

- [ ] **Step 2: Connect CreateTicket - covered by extended `mapServiceError`**

`ticket_crud.go:124-127` already routes through `mapServiceError(err)`:

```go
	t, err := s.ticketSvc.CreateTicket(ctx, create)
	if err != nil {
		return nil, mapServiceError(err)
	}
```

Because Task 4 Step 1 extended `mapServiceError` with the `ErrInvalidLabelName -> CodeInvalidArgument` case, the Connect create path maps it automatically. No edit to `ticket_crud.go` is required beyond confirming it calls `mapServiceError`. Do NOT add an inline switch or an `errors` import here.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: OK.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/api/rest/v1/tickets.go
git commit -m "feat(api): map ErrInvalidLabelName to validation/InvalidArgument on create paths"
```

---

## Task 6: MCP create_ticket - labels support

**Files:**
- Modify: `runner/internal/mcp/http_tools_ticket_write.go` (`createCreateTicketTool` schema + handler; caller at `:58`)
- Modify: `runner/internal/mcp/tools/types_client.go` (`TicketClient.CreateTicket` signature, `types_client.go:46`)
- Modify: `runner/internal/mcp/grpc_client_ticket.go` (`GRPCCollaborationClient.CreateTicket` params map, `grpc_client_ticket.go:65`)
- Modify: `runner/internal/mcp/http_tools_format_helpers_test.go` (`mockFormatClient.CreateTicket`, `:94`)
- Modify: `backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go` (`mcpCreateTicket`, `:12`)

**Interfaces:**
- Consumes: service `CreateTicketRequest.Labels []string` (already exists).
- Produces: `create_ticket` MCP tool accepts `labels: []string`.
- R0/R4: signature edits target the `TicketClient` interface (`types_client.go:46-47`), which `CollaborationClient` embeds. Current sig: `CreateTicket(ctx, repositoryID *int64, title, content string, priority TicketPriority, parentTicketSlug *string) (*Ticket, error)`. Keep `content string`; append `labels []string` as the LAST param. The backend adapter builds errors via `newMcpError(code int32, msg string)` (`runner_adapter_mcp.go:42`) - there is NO `mcpErrorFrom`. The create path currently returns `newMcpError(500, "failed to create ticket")` on any error (`:60`).

- [ ] **Step 1: Add `labels` to the tool InputSchema**

In `createCreateTicketTool`, add to `properties`:

```go
				"labels": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional label names to attach. Created if they do not exist.",
				},
```

- [ ] **Step 2: Parse labels in the tool handler and pass to the client**

In the handler (caller `http_tools_ticket_write.go:58`), extract a `[]string` from `args["labels"]` (use the file's existing arg-parsing helper; if none, range over the `[]interface{}` and type-assert each to string), then pass it as the last argument to `client.CreateTicket(...)`.

- [ ] **Step 3: Extend the interface, impl, and test mock**

- `tools/types_client.go:46`: append `labels []string` as the final param of `TicketClient.CreateTicket`.
- `grpc_client_ticket.go:65`: add the param and:

```go
	if len(labels) > 0 {
		params["labels"] = labels
	}
```

- `http_tools_format_helpers_test.go:94`: update `mockFormatClient.CreateTicket` to the new signature (append a `_ []string` param). Without this the `runner` tests fail to compile.

- [ ] **Step 4: Extend the backend adapter + map `ErrInvalidLabelName` to 400**

In `runner_adapter_mcp_ticket_write.go` `mcpCreateTicket` (`:12`), add to the params struct:

```go
		Labels []string `json:"labels"`
```

and in the `ticket.CreateTicketRequest{...}` build add:

```go
		Labels: params.Labels,
```

Before the generic `newMcpError(500, "failed to create ticket")` (`:60`), add the invalid-label-name branch (grep this file's import block for its domain alias - shown here as `ticket`; add an `errors` import if missing):

```go
	if errors.Is(err, ticket.ErrInvalidLabelName) {
		return nil, newMcpError(400, "invalid label name")
	}
```

`newMcpError` takes an `int32` code. This is what stops the invalid-name case from being swallowed to a 500.

- [ ] **Step 5: Build everything from repo root**

Run: `go build ./...`
Expected: OK. (Single module - the signature change ripples to all `CreateTicket` callers/mocks in one build; grep `CreateTicket(` in `runner/internal/mcp` to confirm none missed.)

- [ ] **Step 6: Add an MCP adapter test if a harness exists**

Grep `runner_adapter_mcp_ticket_write` test neighbors; if a table-test exists, add a case asserting `Labels` is threaded into the service request (mock service captures the request). Otherwise this is build-covered - state the gap, do not claim white-box coverage.

- [ ] **Step 7: Commit**

```bash
git add runner/internal/mcp/ backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go
git commit -m "feat(mcp): create_ticket accepts labels (get-or-created backend-side)"
```

---

## Task 7: MCP update_ticket - labels support

**Files:**
- Modify: `runner/internal/mcp/http_tools_ticket_write.go` (`createUpdateTicketTool`; caller at `:123`)
- Modify: `runner/internal/mcp/tools/types_client.go` (`TicketClient.UpdateTicket` signature, `types_client.go:47`)
- Modify: `runner/internal/mcp/grpc_client_ticket.go` (`UpdateTicket` params, `grpc_client_ticket.go:89`)
- Modify: `runner/internal/mcp/http_tools_format_helpers_test.go` (`mockFormatClient.UpdateTicket`, `:102`)
- Modify: `backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go` (`mcpUpdateTicket`, `:66`)

**Interfaces:**
- Consumes: `ReplaceTicketLabels` (Task 2). JSON `*[]string` carries presence so full tri-state.
- R0/R4: current `TicketClient.UpdateTicket(ctx, ticketSlug string, title, content *string, status *TicketStatus, priority *TicketPriority) (*Ticket, error)`. Keep `content *string`; append `labels []string` as the LAST param. `mcpUpdateTicket` returns via `enrichTicketForMCP(ctx, tc.OrganizationID, t, nil)` (`:106`); `tc.OrganizationID` is the org accessor.

- [ ] **Step 1: Tool schema + handler + client + mock**

Mirror Task 6 Steps 1-3 for the update tool, threading `labels []string` through `TicketClient.UpdateTicket` (`types_client.go:47`), `grpc_client_ticket.go:89`, and the caller at `http_tools_ticket_write.go:123`. Update `mockFormatClient.UpdateTicket` (`http_tools_format_helpers_test.go:102`) to the new signature (append `_ []string`).

- [ ] **Step 2: Backend adapter wiring (use `newMcpError`, not `mcpErrorFrom`)**

In `mcpUpdateTicket` (`:66`), integrate `Labels *[]string` into the EXISTING anonymous params struct literal:

```go
		Labels *[]string `json:"labels"`
```

After the existing `ticketService.UpdateTicket(ctx, t.ID, updates)` call, when `params.Labels != nil`, replace labels then refresh `t` before the existing `enrichTicketForMCP(...)` return. Grep this file's import block for its domain + service aliases (shown here as `ticket` for domain and `ticketservice` for the service sentinel):

```go
	if params.Labels != nil {
		if err := a.ticketService.ReplaceTicketLabels(ctx, t.ID, tc.OrganizationID, *params.Labels); err != nil {
			if errors.Is(err, ticket.ErrInvalidLabelName) {
				return nil, newMcpError(400, "invalid label name")
			}
			if errors.Is(err, ticketservice.ErrTicketNotFound) {
				return nil, newMcpError(404, "ticket not found")
			}
			return nil, newMcpError(500, "failed to update ticket labels")
		}
		refreshed, ferr := a.ticketService.GetTicket(ctx, t.ID)
		if ferr != nil {
			return nil, newMcpError(500, "failed to load updated ticket")
		}
		t = refreshed
	}
```

Keep the existing `enrichTicketForMCP(ctx, tc.OrganizationID, t, nil)` return after this block. `newMcpError` takes an `int32` code. On re-fetch failure return a 500, not the stale `t` (R5). Add an `errors` import if missing.

- [ ] **Step 3: Build everything from repo root + commit**

Run: `go build ./...`

```bash
git add runner/internal/mcp/ backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go
git commit -m "feat(mcp): update_ticket accepts labels (replace-all via *[]string)"
```

---

## Task 8: Full verification smoke test

**Files:** none (verification only)

- [ ] **Step 1: Build everything (single module, from repo root)**

Run: `go build ./...`
Expected: succeeds. (One build covers backend + runner - this is a single module; a split-cd build would hide cross-subtree breakage.)

- [ ] **Step 2: Run the ticket service test suite**

Run: `go test ./backend/internal/service/ticket/... -v`
Expected: PASS, including all new tests (Tasks 1-3) and the pre-existing suite.

- [ ] **Step 3: Run the broader backend test suite**

Run: `go test ./backend/... 2>&1 | tail -40`
Expected: PASS (no regressions). Investigate any failure before proceeding.

- [ ] **Step 4: Lint**

Run: `bazel run //backend:lint && bazel run //runner:lint`
(Fallback: `golangci-lint run ./backend/... ./runner/...`. Bazel test alt: `bazel test //backend/internal/service/ticket/...`.)
Expected: clean.

- [ ] **Step 5: Confirm spec coverage**

Re-read the spec's Test plan (items 1-10) and confirm each maps to an implemented test, a build-covered wiring, or a review-covered branch. Use the honest labels in the Test plan below - do not claim full white-box coverage for build/review-covered items.

- [ ] **Step 6: Final commit if any cleanup**

```bash
git add -A && git commit -m "test(ticket): full label fix verification pass" || echo "nothing to commit"
```

---

## Test plan (honest coverage labels - R6)

| # | Spec item | Coverage | Notes |
|---|---|---|---|
| 1 | get-or-create unknown label on create | TEST | `TestCreateTicket_GetOrCreatesUnknownLabel` (asserts name, explicit `#6B7280`, NULL repository_id) |
| 2 | reuse existing label (no new row) | TEST | `TestCreateTicket_ReusesExistingLabel` - count-based proof (before == after). The pre-existing `TestCreateTicket_TableDriven` "with label names" case is a SMOKE check only; it does not assert count and does NOT prove reuse. |
| 3 | de-dup names on create | TEST | `TestCreateTicket_DedupsDuplicateNames` - count-based proof (`{x,x," x ","  "}` -> 1 label) |
| 4 | reject name len > 100 | TEST | `TestCreateTicket_RejectsTooLongName` |
| 5 | replace set / clear all | TEST | `TestReplaceTicketLabels_ReplacesSet`, `TestReplaceTicketLabels_ClearsAll`, `TestReplaceTicketLabels_EmptyClears` |
| 6 | tenant guard (wrong org rejected, labels untouched) | TEST | `TestReplaceTicketLabels_WrongOrgRejected` (asserts `ErrTicketNotFound` + untouched) |
| 7a | nil-labels = untouched (REST tri-state) | REVIEW | The service test only proves `[]` clears. The handler `if req.Labels != nil` skip is review-covered (no httptest harness). Do not claim the service test proves this. |
| 7b | atomic rollback on mid-replace failure | REVIEW | NOT a unit test. The constraint-free single-conn SQLite schema (`testkit/db.go:24` `SetMaxOpenConns(1)`; `testkit/schema_business.go:94-103` has surrogate `id` PK only, no unique/composite-PK) has nothing to trip mid-tx and cannot reproduce a race. Atomicity comes from the `tx.Transaction(...)` wrapper in `ReplaceLabels`/`CreateTicketAtomic` and is REVIEW-covered. No deterministic `TestRollback` is written. |
| 7c | concurrency / get-or-create race (decision #9) | BUILD/REVIEW | Single-conn `:memory:` serializes writers, so no race is reproducible. `clause.OnConflict{DoNothing:true}` is kept defensively, but for org-level (`repository_id IS NULL`) labels the conflict does NOT fire on PG (NULLS DISTINCT). The safety is "benign duplicate row + deterministic `ORDER BY id ASC` resolution on subsequent reads," NOT `ON CONFLICT`. `Verified: backend/migrations/000001_init_schema.up.sql:479` (UNIQUE includes nullable `repository_id`) on PG16. |
| 8 | REST UpdateTicket `*[]string` + re-fetch | TEST (service) + BUILD (handler) | Service path proven by replace tests; handler wiring + re-fetch is build-covered unless an httptest harness is added. |
| 9 | Connect UpdateTicket no-op-on-empty + re-fetch + error mapping | BUILD | Build-covered unless a Connect transport test harness exists; add explicit transport tests if one does. No white-box coverage claimed. |
| 10 | MCP create/update labels | BUILD | Build-covered unless an MCP adapter/transport test harness exists; add explicit tests if one does. No white-box coverage claimed. |

---

## Spec coverage map

| Spec deliverable | Task | Coverage |
|---|---|---|
| get-or-create in shared loop | 1 | TEST |
| org-level lookup+create scope, deterministic | 1 | TEST (NULL repo_id) + REVIEW (determinism on PG dup) |
| explicit #6B7280 color | 1 | TEST |
| trim/skip-empty/len<=100 validation, all paths | 1 | TEST |
| de-dup on create | 1 | TEST (count-based) |
| `ReplaceLabels` repo + tenant guard | 2 | TEST |
| `ReplaceTicketLabels` service + sentinel translation | 2 | TEST |
| de-dup on replace | 2 | TEST (via ReplacesSet) |
| REST UpdateTicket `*[]string` tri-state + re-fetch | 3 | TEST (service) + BUILD/REVIEW (handler/nil-skip) |
| Connect UpdateTicket no-op-on-empty + re-fetch | 4 | BUILD |
| failure->status mapping (REST+Connect) | 3, 4, 5 | TEST (REST sentinel via service) + BUILD (transport) |
| MCP create labels | 6 | BUILD |
| MCP update labels | 7 | BUILD |
| atomicity (tx wrapper) | 2 | REVIEW |
| concurrency safety (decision #9) | 1 | BUILD/REVIEW |
| no proto / no migration change | (held throughout) | - |
| full verification | 8 | build + suite + lint |

---

## Plan-review patterns

- **Pattern A - error-sentinel cross-package identity.** Every `errors.Is(e, X)` must use the consuming file's actual import alias for X's package; a sentinel returned by one layer and asserted in another must be EXPLICITLY mapped, never assumed equal. (Why Task 2 maps the repo's `gorm.ErrRecordNotFound` to the service `ErrTicketNotFound`: the infra repo cannot import the service package - import cycle - so it returns gorm's sentinel and the service translates it.)
- **Pattern B - constraint-free test harness.** Behavior that depends on a DB constraint (unique / PK / FK / mid-tx trip) cannot be unit-tested on the in-memory SQLite schema, which has none; such deliverables are build/review-covered and must be labeled so, never claimed as test-covered. (Why items 7b and 7c are REVIEW / BUILD-REVIEW, not TEST.)
