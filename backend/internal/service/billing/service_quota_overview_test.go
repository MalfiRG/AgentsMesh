package billing

import (
	"context"
	"testing"
	"time"

	"github.com/anthropics/agentsmesh/backend/internal/domain/billing"
)

// ===========================================
// Billing Overview Tests
// ===========================================

func TestGetBillingOverview(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(newTestRepo(db), "")
	ctx := context.Background()

	seedTestPlan(t, db)
	service.CreateSubscription(ctx, 1, "based")

	// Add some resources
	db.Exec("INSERT INTO organization_members (organization_id, user_id, role) VALUES (1, 1, 'owner')")
	db.Exec("INSERT INTO runners (organization_id, node_id) VALUES (1, 'runner1')")
	db.Exec("INSERT INTO repositories (organization_id, name, slug) VALUES (1, 'repo1', 'repo1')")
	db.Exec("INSERT INTO pods (organization_id, pod_key, status) VALUES (1, 'pod1', 'running')")

	overview, err := service.GetBillingOverview(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get billing overview: %v", err)
	}
	if overview.Plan.Name != "based" {
		t.Errorf("expected plan 'free', got %s", overview.Plan.Name)
	}
	if overview.Usage.Users != 1 {
		t.Errorf("expected 1 user, got %d", overview.Usage.Users)
	}
	if overview.Usage.Runners != 1 {
		t.Errorf("expected 1 runner, got %d", overview.Usage.Runners)
	}
}

func TestGetBillingOverviewNoSubscription(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(newTestRepo(db), "")
	ctx := context.Background()

	seedTestPlan(t, db)

	// No subscription - should return error
	_, err := service.GetBillingOverview(ctx, 999)
	if err != ErrSubscriptionNotFound {
		t.Errorf("expected ErrSubscriptionNotFound, got %v", err)
	}
}

func TestGetBillingOverviewWithNilPlan(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(newTestRepo(db), "")
	ctx := context.Background()

	plan := seedTestPlan(t, db)

	// Create subscription without preloading plan
	now := time.Now()
	sub := &billing.Subscription{
		OrganizationID:     1,
		PlanID:             plan.ID,
		Status:             billing.SubscriptionStatusActive,
		BillingCycle:       billing.BillingCycleMonthly,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}
	db.Create(sub)

	// GetBillingOverview should still work by fetching plan by ID
	overview, err := service.GetBillingOverview(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get billing overview: %v", err)
	}
	if overview.Plan == nil {
		t.Error("expected plan to be loaded")
	}
}

func TestGetBillingOverviewReflectsCustomQuotas(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(newTestRepo(db), "")
	ctx := context.Background()

	seedTestPlan(t, db) // max_concurrent_pods = 5, max_runners = 1, max_users = 1
	service.CreateSubscription(ctx, 1, "based")
	service.SetCustomQuota(ctx, 1, "concurrent_pods", 50)
	service.SetCustomQuota(ctx, 1, "runners", 2)

	overview, err := service.GetBillingOverview(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get billing overview: %v", err)
	}
	if overview.Usage.MaxConcurrentPods != 50 {
		t.Errorf("expected custom concurrent_pods limit 50, got %d", overview.Usage.MaxConcurrentPods)
	}
	if overview.Usage.MaxRunners != 2 {
		t.Errorf("expected custom runners limit 2, got %d", overview.Usage.MaxRunners)
	}
	if overview.Usage.MaxUsers != 1 {
		t.Errorf("expected plan users limit 1, got %d", overview.Usage.MaxUsers)
	}
}
