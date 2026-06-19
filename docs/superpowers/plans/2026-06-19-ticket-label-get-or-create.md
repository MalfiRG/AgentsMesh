# Ticket Label Get-or-Create + Label Updates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ticket labels work across all write surfaces - create get-or-creates labels by name, and labels can be replaced on update via REST, Connect/UI, and the agent-facing MCP tools.

**Architecture:** A single `tx`-scoped `getOrCreateLabel` helper in the infra repo backs both the shared `CreateTicketAtomic` loop and a new `ReplaceLabels` repo primitive. The service exposes `ReplaceTicketLabels`; REST/MCP wire it with `*[]string` tri-state, Connect wires it with a `len>0` no-op guard (proto3 `repeated` has no presence). No proto change, no DB migration.

**Tech Stack:** Go, Gin (REST), Connect-RPC, GORM, PostgreSQL 16. Tests: white-box `package ticket`, in-memory SQLite via `testkit.SetupTestDB`.

## Global Constraints

- Every file must stay under 200 lines (test files under 400). If `tickets.go` crosses 200 after edits, extract the label-handling block to a sibling file.
- No code comments that restate what the code does (project rule); comment only non-obvious why.
- ASCII hyphen only; never U+2014/U+2013.
- Auto-created label color is set EXPLICITLY to `#6B7280` (the in-memory test schema defaults `labels.color` to `#808080`, so relying on the DB default fails the color assertion).
- Lookup AND create both at org level: `organization_id = ? AND name = ? AND repository_id IS NULL`.
- Name handling on ALL paths: trim; skip if empty after trim; `len > 100` after trim is `ErrInvalidLabelName`. Case preserved (`Bug` != `bug`).
- De-dup names (after trim) and resolved ids before linking, on create AND replace.
- Source of truth: `docs/superpowers/specs/2026-06-19-ticket-label-get-or-create-design.md` (Rev 2).

## Deviation from spec (decision #9 refinement)

Spec decision #9 says get-or-create handles a unique-violation via `ON CONFLICT`. Verified against PG16: the `UNIQUE(organization_id, repository_id, name)` constraint treats NULL `repository_id` as DISTINCT, so it does NOT enforce uniqueness for org-level labels and `ON CONFLICT` does not fire for them. Net: a concurrent insert of the same new org-level name produces a benign duplicate row, not an error/500. Resolution applied in this plan: keep `clause.OnConflict{DoNothing: true}` defensively (covers the repo-scoped path and a future partial index), AND make every lookup deterministic with `ORDER BY id ASC` so duplicate rows resolve to the same label. Strict org-level uniqueness (a partial unique index `WHERE repository_id IS NULL`) is a tracked follow-up, not in this fix.

---

## Task 1: `getOrCreateLabel` helper + name normalization + `ErrInvalidLabelName`

**Files:**
- Create: `backend/internal/domain/ticket/label_errors.go`
- Modify: `backend/internal/infra/ticket_repo_label.go`
- Test: `backend/internal/service/ticket/service_label_getorcreate_test.go`

**Interfaces:**
- Produces: `ticket.ErrInvalidLabelName` (domain sentinel); `getOrCreateLabel(tx *gorm.DB, orgID int64, name string) (int64, error)` (unexported, infra); `normalizeLabelNames(names []string) ([]string, error)` (unexported, infra).
- Consumes: `ticket.Label`, GORM `clause`, the existing `isNotFound(err)` helper in infra.

- [ ] **Step 1: Write the failing test**

In `backend/internal/service/ticket/service_label_getorcreate_test.go`:

```go
package ticket

import (
	"context"
	"testing"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bazel test //backend/internal/service/ticket:ticket_test --test_filter=TestCreateTicket_GetOrCreatesUnknownLabel`
(Fallback if the bazel target name differs: `cd backend && go test ./internal/service/ticket/ -run TestCreateTicket_GetOrCreatesUnknownLabel -v`)
Expected: FAIL - label is skipped (0 labels) because the current loop `continue`s on unknown.

- [ ] **Step 3: Add the domain error**

In `backend/internal/domain/ticket/label_errors.go`:

```go
package ticket

import "errors"

var ErrInvalidLabelName = errors.New("invalid label name")
```

- [ ] **Step 4: Add helpers to `ticket_repo_label.go`**

Add imports `strings` and `gorm.io/gorm/clause` as needed. Append:

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

(The loop wiring that consumes these lands in Task 2; this task only adds the helpers + error and is verified by Task 2's test going green. If you want Task 1 to compile-and-pass on its own, the test from Step 1 will still fail until Task 2 - acceptable: mark Step 2's failure as the gate and proceed; the helpers are dead code until Task 2 calls them.)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/domain/ticket/label_errors.go backend/internal/infra/ticket_repo_label.go backend/internal/service/ticket/service_label_getorcreate_test.go
git commit -m "feat(ticket): add getOrCreateLabel helper + label name normalization"
```

---

## Task 2: Wire get-or-create into `CreateTicketAtomic`

**Files:**
- Modify: `backend/internal/infra/ticket_repo.go` (the `LabelNames` loop in `CreateTicketAtomic`)
- Test: `backend/internal/service/ticket/service_label_getorcreate_test.go` (extend)

**Interfaces:**
- Consumes: `normalizeLabelNames`, `getOrCreateLabel` (Task 1).
- Produces: create path now get-or-creates + de-dups labels.

- [ ] **Step 1: Add failing tests for reuse + dedup + validation**

Append to `service_label_getorcreate_test.go`:

```go
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

Add imports `errors` and the domain `ticket` package alias to the test file as needed.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/service/ticket/ -run 'TestCreateTicket_(GetOrCreates|Reuses|Dedups|RejectsTooLong)' -v`
Expected: FAIL (labels still skipped; no dedup; no validation).

- [ ] **Step 3: Replace the LabelNames loop in `CreateTicketAtomic`**

In `backend/internal/infra/ticket_repo.go`, replace the existing loop:

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

Note: `err` may already be declared earlier in the function; if so use `=` not `:=` or rename to `nerr` to avoid shadowing - verify the surrounding scope.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ticket/ -run 'TestCreateTicket_(GetOrCreates|Reuses|Dedups|RejectsTooLong)' -v`
Expected: PASS (all four).

- [ ] **Step 5: Regression smoke check (not validation)**

Run: `cd backend && go test ./internal/service/ticket/ -run TestCreateTicket -v`
Expected: PASS, including the pre-existing `TestCreateTicket_TableDriven` "with label names" case. This is a SMOKE CHECK that the existing-label resolution path is preserved - it does not validate the new get-or-create behavior (that is covered by the new tests above).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/infra/ticket_repo.go backend/internal/service/ticket/service_label_getorcreate_test.go
git commit -m "fix(ticket): create path get-or-creates labels by name instead of skipping"
```

---

## Task 3: `ReplaceLabels` repo + `ReplaceTicketLabels` service (with tenant guard)

**Files:**
- Modify: `backend/internal/infra/ticket_repo_label.go` (add `ReplaceLabels`)
- Modify: `backend/internal/domain/ticket/repository.go` (add to interface)
- Modify: `backend/internal/service/ticket/label_service.go` (add `ReplaceTicketLabels`)
- Test: `backend/internal/service/ticket/service_label_replace_test.go`

**Interfaces:**
- Consumes: `getOrCreateLabel`, `normalizeLabelNames` (Task 1).
- Produces: repo `ReplaceLabels(ctx context.Context, ticketID, orgID int64, names []string) error`; service `(*Service).ReplaceTicketLabels(ctx context.Context, ticketID, orgID int64, names []string) error`. Reuse `ErrTicketNotFound` if it already exists in the service package; otherwise add it to `service_types.go`.

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

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/service/ticket/ -run TestReplaceTicketLabels -v`
Expected: FAIL - `service.ReplaceTicketLabels` undefined.

- [ ] **Step 3: Add `ReplaceLabels` to the repo**

Append to `backend/internal/infra/ticket_repo_label.go`:

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
			return ticket.ErrTicketNotFound
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

If `ticket.ErrTicketNotFound` does not exist in the domain package, add it to `label_errors.go` (or reuse the service-level sentinel and map there - grep first: `grep -rn "ErrTicketNotFound" backend/internal`). Keep the error referenced here consistent with what the test imports (`ErrTicketNotFound` in `package ticket` service scope). Simplest: define `ErrTicketNotFound` in `service_types.go` if absent, and have the repo return a domain sentinel that the service maps - but to keep the test (which references the service-package `ErrTicketNotFound`) green, the service method should translate the repo error to the service `ErrTicketNotFound`. See Step 5.

- [ ] **Step 4: Add `ReplaceLabels` to the repo interface**

In `backend/internal/domain/ticket/repository.go`, add to the `TicketRepository` interface near the other label methods:

```go
	ReplaceLabels(ctx context.Context, ticketID, orgID int64, names []string) error
```

- [ ] **Step 5: Add the service method**

In `backend/internal/service/ticket/label_service.go`:

```go
func (s *Service) ReplaceTicketLabels(ctx context.Context, ticketID, orgID int64, names []string) error {
	if err := s.repo.ReplaceLabels(ctx, ticketID, orgID, names); err != nil {
		slog.ErrorContext(ctx, "failed to replace ticket labels", "ticket_id", ticketID, "org_id", orgID, "error", err)
		return err
	}
	slog.InfoContext(ctx, "ticket labels replaced", "ticket_id", ticketID, "org_id", orgID, "count", len(names))
	return nil
}
```

Ensure the error the repo returns for a wrong-org ticket is the same sentinel the test asserts (`ErrTicketNotFound` in the service `package ticket`). If you defined the domain sentinel `ticket.ErrTicketNotFound`, alias or map it; if `service_types.go` already defines `ErrTicketNotFound`, return that from the repo path (translate in the service method via `errors.Is`).

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ticket/ -run TestReplaceTicketLabels -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/infra/ticket_repo_label.go backend/internal/domain/ticket/repository.go backend/internal/domain/ticket/label_errors.go backend/internal/service/ticket/label_service.go backend/internal/service/ticket/service_types.go backend/internal/service/ticket/service_label_replace_test.go
git commit -m "feat(ticket): ReplaceTicketLabels service + ReplaceLabels repo with tenant guard"
```

---

## Task 4: Ext REST UpdateTicket - labels field + wiring + re-fetch

**Files:**
- Modify: `backend/internal/api/rest/v1/tickets.go`
- Test: `backend/internal/api/rest/v1/tickets_labels_test.go` (or extend the nearest existing handler test; if no handler test harness exists, cover via a service-level test for the wiring and assert the handler logic by reading)

**Interfaces:**
- Consumes: `ticketService.ReplaceTicketLabels` (Task 3); `ticket.ErrInvalidLabelName`, `ErrTicketNotFound`.

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

- [ ] **Step 2: Wire into the `UpdateTicket` handler**

After the existing `t, err = h.ticketService.UpdateTicket(...)` call succeeds and BEFORE the JSON response, add:

```go
	if req.Labels != nil {
		if err := h.ticketService.ReplaceTicketLabels(c.Request.Context(), t.ID, tenant.OrganizationID, *req.Labels); err != nil {
			switch {
			case errors.Is(err, ticket.ErrInvalidLabelName):
				apierr.BadRequest(c, "Invalid label name")
			case errors.Is(err, ErrTicketNotFound):
				apierr.ResourceNotFound(c, "Ticket not found")
			default:
				apierr.InternalError(c, "Failed to update labels")
			}
			return
		}
		t, err = h.ticketService.GetTicketBySlug(c.Request.Context(), tenant.OrganizationID, t.Slug)
		if err != nil {
			apierr.ResourceNotFound(c, "Ticket not found")
			return
		}
	}
```

Use the real error-helper names from `apierr` (grep `backend/internal/api/rest/apierr` for `BadRequest`/`ResourceNotFound`/`InternalError`; match exact names). The post-mutation re-fetch is required - the earlier `UpdateTicket` re-fetch happened before labels changed (F-ADV-06).

- [ ] **Step 3: File-size check**

Run: `wc -l backend/internal/api/rest/v1/tickets.go`
If > 200, extract the label block into `backend/internal/api/rest/v1/tickets_labels.go` as a helper `func (h *TicketHandler) applyLabelUpdate(c *gin.Context, t *ticket.Ticket, labels *[]string) (*ticket.Ticket, bool)` returning the refreshed ticket and an ok flag; call it from the handler.

- [ ] **Step 4: Add a wiring test**

Minimal coverage (service-level proves the path; if a REST test harness exists, prefer it):

```go
// In service_label_replace_test.go - proves nil = untouched semantics the handler relies on.
func TestReplaceTicketLabels_NilVsEmptyContract(t *testing.T) {
	// nil is handled by the HANDLER (skips the call); empty clears.
	// This test documents that ReplaceTicketLabels([]) clears, so handler must
	// only call it when req.Labels != nil.
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

Run: `cd backend && go build ./... && go test ./internal/service/ticket/ -run TestReplaceTicketLabels -v`
Expected: build OK, tests PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api/rest/v1/
git commit -m "feat(api): ext REST UpdateTicket accepts labels (*[]string tri-state) with re-fetch"
```

---

## Task 5: Connect UpdateTicket - labels wiring (no-op on empty) + re-fetch

**Files:**
- Modify: `backend/internal/api/connect/ticket/ticket_update.go`

**Interfaces:**
- Consumes: `ReplaceTicketLabels`; proto `req.Msg.GetLabels()` (`[]string`).

- [ ] **Step 1: Wire labels after the `UpdateAssignees` block**

```go
	if len(req.Msg.GetLabels()) > 0 {
		if err := s.ticketSvc.ReplaceTicketLabels(ctx, t.ID, tenant.OrganizationID, req.Msg.GetLabels()); err != nil {
			switch {
			case errors.Is(err, ticket.ErrInvalidLabelName):
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			case errors.Is(err, ticketservice.ErrTicketNotFound):
				return nil, connect.NewError(connect.CodeNotFound, err)
			default:
				return nil, connect.NewError(connect.CodeInternal, err)
			}
		}
	}
```

Match the real tenant accessor and service alias used in this file (grep the file head for how `tenant`/`tc` and the ticket service are named). `len(...) > 0` is the deliberate no-op-on-empty guard (proto3 `repeated` has no presence; empty/absent both take the no-op branch). Clearing labels over Connect is via the existing `RemoveLabel` RPC.

- [ ] **Step 2: Re-fetch after the label mutation**

Ensure the ticket returned to `toProtoTicket(t)` is re-fetched AFTER `ReplaceTicketLabels` (the existing `GetTicket` re-fetch runs before labels change). Add a re-fetch when labels were applied:

```go
	if len(req.Msg.GetLabels()) > 0 {
		if refreshed, ferr := s.ticketSvc.GetTicket(ctx, t.ID); ferr == nil {
			t = refreshed
		}
	}
```

- [ ] **Step 3: Build + targeted test**

Run: `cd backend && go build ./... && go test ./internal/api/connect/ticket/... 2>/dev/null || echo "no connect ticket test pkg - covered by build + Task 9 smoke"`
Expected: build OK.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/api/connect/ticket/ticket_update.go
git commit -m "feat(api): Connect UpdateTicket replaces labels (empty=no-op), re-fetch after"
```

---

## Task 6: Create-path error mapping (REST + Connect)

**Files:**
- Modify: `backend/internal/api/rest/v1/tickets.go` (CreateTicket handler)
- Modify: `backend/internal/api/connect/ticket/ticket_crud.go` (CreateTicket handler)

**Interfaces:**
- Consumes: `ticket.ErrInvalidLabelName`.

- [ ] **Step 1: REST CreateTicket - map invalid label name to 400**

In the REST `CreateTicket` handler, where the service error is currently handled (generic 500), add:

```go
	if err != nil {
		if errors.Is(err, ticket.ErrInvalidLabelName) {
			apierr.BadRequest(c, "Invalid label name")
			return
		}
		apierr.InternalError(c, "Failed to create ticket")
		return
	}
```

(Preserve any existing specific error mappings; only add the `ErrInvalidLabelName` branch.)

- [ ] **Step 2: Connect CreateTicket - same mapping**

In `ticket_crud.go` CreateTicket, map `ticket.ErrInvalidLabelName` -> `connect.NewError(connect.CodeInvalidArgument, err)` before the generic internal-error return.

- [ ] **Step 3: Build**

Run: `cd backend && go build ./...`
Expected: OK.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/api/
git commit -m "feat(api): map ErrInvalidLabelName to 400/InvalidArgument on create paths"
```

---

## Task 7: MCP create_ticket - labels support

**Files:**
- Modify: `runner/internal/mcp/http_tools_ticket_write.go` (`createCreateTicketTool` schema + handler)
- Modify: `runner/internal/mcp/tools/types_client.go` (`CollaborationClient.CreateTicket` signature)
- Modify: `runner/internal/mcp/grpc_client_ticket.go` (`GRPCCollaborationClient.CreateTicket` params map)
- Modify: `backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go` (`mcpCreateTicket` params struct + `CreateTicketRequest` build)

**Interfaces:**
- Consumes: service `CreateTicketRequest.Labels []string` (already exists).
- Produces: `create_ticket` MCP tool accepts `labels: []string`.

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

In the handler, extract a `[]string` from `args["labels"]` (use the file's existing arg-parsing helper; if none, range over the `[]interface{}` and type-assert each to string), then pass it to `client.CreateTicket(...)`.

- [ ] **Step 3: Extend the client interface + impl**

In `tools/types_client.go`, add `labels []string` as the final param of `CreateTicket`. In `grpc_client_ticket.go`, add the param and:

```go
	if len(labels) > 0 {
		params["labels"] = labels
	}
```

- [ ] **Step 4: Extend the backend adapter**

In `runner_adapter_mcp_ticket_write.go` `mcpCreateTicket`, add to the params struct:

```go
		Labels []string `json:"labels"`
```

and in the `ticket.CreateTicketRequest{...}` build add:

```go
		Labels: params.Labels,
```

- [ ] **Step 5: Build both modules**

Run: `cd backend && go build ./... && cd ../runner && go build ./...`
Expected: OK (signature change ripples to any other `CreateTicket` callers/mocks - fix them; grep `CreateTicket(` in `runner/internal/mcp`).

- [ ] **Step 6: Add an MCP adapter test if a harness exists**

Grep `runner_adapter_mcp_ticket_write` test neighbors; if a table-test exists, add a case asserting `Labels` is threaded into the service request (mock service captures the request). Otherwise rely on the build + Task 9 smoke and note the gap.

- [ ] **Step 7: Commit**

```bash
git add runner/internal/mcp/ backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go
git commit -m "feat(mcp): create_ticket accepts labels (get-or-created backend-side)"
```

---

## Task 8: MCP update_ticket - labels support

**Files:**
- Modify: `runner/internal/mcp/http_tools_ticket_write.go` (`createUpdateTicketTool`)
- Modify: `runner/internal/mcp/tools/types_client.go` (`UpdateTicket` signature)
- Modify: `runner/internal/mcp/grpc_client_ticket.go` (`UpdateTicket` params)
- Modify: `backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go` (`mcpUpdateTicket`)

**Interfaces:**
- Consumes: `ReplaceTicketLabels` (Task 3). JSON `*[]string` carries presence so full tri-state.

- [ ] **Step 1: Tool schema + handler + client** - mirror Task 7 Steps 1-3 for the update tool, threading `labels []string` through `UpdateTicket`.

- [ ] **Step 2: Backend adapter wiring**

In `mcpUpdateTicket`, add `Labels *[]string `json:"labels"`` to the params struct. After the existing `ticketService.UpdateTicket(ctx, t.ID, updates)` call, when `params.Labels != nil`:

```go
	if params.Labels != nil {
		if err := a.ticketService.ReplaceTicketLabels(ctx, t.ID, tc.OrganizationID, *params.Labels); err != nil {
			return nil, mcpErrorFrom(err) // map ErrInvalidLabelName/ErrTicketNotFound; match existing mcpError helper
		}
		if refreshed, ferr := a.ticketService.GetTicket(ctx, t.ID); ferr == nil {
			t = refreshed
		}
	}
```

Match the file's existing `mcpError` construction pattern (grep `mcpError` in the adapter); there is no generic `mcpErrorFrom` - build the `*mcpError` inline as the neighboring code does.

- [ ] **Step 3: Build + commit**

```bash
cd backend && go build ./... && cd ../runner && go build ./...
git add runner/internal/mcp/ backend/internal/api/grpc/runner_adapter_mcp_ticket_write.go
git commit -m "feat(mcp): update_ticket accepts labels (replace-all via *[]string)"
```

---

## Task 9: Full verification smoke test

**Files:** none (verification only)

- [ ] **Step 1: Build everything**

Run: `cd backend && go build ./... && cd ../runner && go build ./...`
Expected: both succeed.

- [ ] **Step 2: Run the ticket service test suite**

Run: `cd backend && go test ./internal/service/ticket/... -v`
Expected: PASS, including all new tests (Tasks 1-4) and the pre-existing suite.

- [ ] **Step 3: Run the broader backend test suite**

Run: `cd backend && go test ./internal/... 2>&1 | tail -30`
Expected: PASS (no regressions). Investigate any failure before proceeding.

- [ ] **Step 4: Lint**

Run: `bazel run //backend:lint && bazel run //runner:lint`
(Fallback: `cd backend && golangci-lint run ./internal/...`)
Expected: clean.

- [ ] **Step 5: Confirm spec coverage**

Re-read the spec's Test plan (items 1-10) and confirm each maps to an implemented test or a build-covered wiring. Note any item covered only by build/smoke (Connect transport test item 9 and MCP item 10 may be build-covered if no transport test harness exists) - state this explicitly rather than claiming full test coverage.

- [ ] **Step 6: Final commit if any cleanup**

```bash
git add -A && git commit -m "test(ticket): full label fix verification pass" || echo "nothing to commit"
```

---

## Spec coverage map

| Spec deliverable | Task |
|---|---|
| get-or-create in shared loop | 1, 2 |
| org-level lookup+create scope, deterministic | 1 |
| explicit #6B7280 color | 1 |
| trim/skip-empty/len<=100 validation, all paths | 1, 2 |
| de-dup on create | 2 |
| `ReplaceLabels` repo + tenant guard | 3 |
| `ReplaceTicketLabels` service | 3 |
| de-dup on replace | 3 |
| REST UpdateTicket `*[]string` tri-state + re-fetch | 4 |
| Connect UpdateTicket no-op-on-empty + re-fetch | 5 |
| failure->status mapping (REST+Connect) | 4, 5, 6 |
| MCP create labels | 7 |
| MCP update labels | 8 |
| no proto / no migration change | (held throughout) |
| test plan items 1-10 | 1-5, 9 |
