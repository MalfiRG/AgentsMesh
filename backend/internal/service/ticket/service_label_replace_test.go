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
