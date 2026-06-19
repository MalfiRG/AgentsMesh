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
