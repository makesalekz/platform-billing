package service

import (
	"context"
	"testing"
	"time"

	v1 "github.com/makesalekz/platform-billing/api/platform_billing/v1"
	"github.com/makesalekz/platform-billing/ent/enum"
	"github.com/makesalekz/platform-billing/internal/biz"
	"github.com/makesalekz/platform-billing/internal/data"
	"github.com/makesalekz/platform-billing/messages"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================
// Story 7.1: GMV & Commission
// ========================================

func TestHandleSaleCompleted_StoresSaleEvents(t *testing.T) {
	_, uc, _, _, saleEventsRepo := setupService()
	ctx := context.Background()

	event := messages.SaleCompletedEvent{
		TenantID:  1,
		SaleID:    100,
		Total:     "500",
		Timestamp: "2026-05-01T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "200"},
			{ProductID: 20, Total: "300"},
		},
	}

	err := uc.HandleSaleCompleted(ctx, event)
	require.NoError(t, err)

	assert.Equal(t, 2, len(saleEventsRepo.events))
}

func TestHandleSaleCompleted_EnsuresCommissionExists(t *testing.T) {
	_, uc, commissionsRepo, _, _ := setupService()
	ctx := context.Background()

	completedAt := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	uc.EnsureCommissionExists(ctx, 1, completedAt)

	// Should create a commission for May 2026
	assert.Equal(t, 1, len(commissionsRepo.commissions))
	for _, c := range commissionsRepo.commissions {
		assert.Equal(t, int64(1), c.TenantID)
		assert.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), c.PeriodStart)
		assert.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), c.PeriodEnd)
		assert.Equal(t, enum.Pending, c.Status)
	}
}

func TestHandleSaleCompleted_IdempotentUpsert(t *testing.T) {
	_, uc, _, _, saleEventsRepo := setupService()
	ctx := context.Background()

	event := messages.SaleCompletedEvent{
		TenantID:  1,
		SaleID:    100,
		Total:     "500",
		Timestamp: "2026-05-01T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "200"},
		},
	}

	uc.HandleSaleCompleted(ctx, event)
	uc.HandleSaleCompleted(ctx, event) // same event again

	assert.Equal(t, 1, len(saleEventsRepo.events)) // should still be 1
}

func TestHandleReturnCompleted_StoresNegativeAmount(t *testing.T) {
	_, uc, _, _, saleEventsRepo := setupService()
	ctx := context.Background()

	event := messages.ReturnCompletedEvent{
		TenantID:  1,
		SaleID:    100,
		ReturnID:  50,
		Total:     "200",
		Timestamp: "2026-05-02T10:00:00Z",
		Items: []messages.ReturnCompletedEventItem{
			{ProductID: 10, Total: "200"},
		},
	}

	err := uc.HandleReturnCompleted(ctx, event)
	require.NoError(t, err)

	assert.Equal(t, 1, len(saleEventsRepo.events))
	for _, ev := range saleEventsRepo.events {
		assert.True(t, ev.Amount.IsNegative(), "return amount should be negative")
		assert.True(t, ev.IsReturn)
	}
}

func TestMonthPeriod(t *testing.T) {
	t1 := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	start, end := biz.MonthPeriod(t1)
	assert.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), start)
	assert.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), end)

	// December -> January
	t2 := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	start2, end2 := biz.MonthPeriod(t2)
	assert.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), start2)
	assert.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), end2)
}

func TestHandleSaleCompleted_CreatesCommissionAutomatically(t *testing.T) {
	_, uc, commissionsRepo, _, _ := setupService()
	ctx := context.Background()

	event := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "500", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 10, Total: "500"}},
	}

	// HandleSaleCompleted should auto-create commission (no manual EnsureCommissionExists)
	err := uc.HandleSaleCompleted(ctx, event)
	require.NoError(t, err)

	assert.Equal(t, 1, len(commissionsRepo.commissions))
}

func TestProductionPath_HandleSaleThenClosePeriod(t *testing.T) {
	svc, uc, commissionsRepo, _, _ := setupService()
	ctx := context.Background()

	// Production path: only HandleSaleCompleted, no manual EnsureCommissionExists
	event := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "600"},
			{ProductID: 20, Total: "400"},
		},
	}
	uc.HandleSaleCompleted(ctx, event)

	// ClosePeriod should find the auto-created commission and charge it
	resp, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.ClosedCount)

	for _, c := range commissionsRepo.commissions {
		assert.Equal(t, enum.Charged, c.Status)
		assert.True(t, decimal.NewFromInt(1000).Equal(c.Gmv))
	}
}

func TestHandleSaleCompleted_MultiTenant(t *testing.T) {
	_, uc, _, _, saleEventsRepo := setupService()
	ctx := context.Background()

	event1 := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "500", Timestamp: "2026-05-01T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 10, Total: "500"}},
	}
	event2 := messages.SaleCompletedEvent{
		TenantID: 2, SaleID: 200, Total: "300", Timestamp: "2026-05-01T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 20, Total: "300"}},
	}

	uc.HandleSaleCompleted(ctx, event1)
	uc.HandleSaleCompleted(ctx, event2)

	assert.Equal(t, 2, len(saleEventsRepo.events))
}

// ========================================
// Story 7.2: Exclusions (Brand Sponsorship)
// ========================================

func TestCreateExclusion(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola",
		SkuIds:    []int64{10, 20, 30},
		StartDate: "2026-05-01",
		EndDate:   "2026-05-31",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Exclusion)

	assert.Equal(t, "Coca-Cola", resp.Exclusion.BrandName)
	assert.Equal(t, []int64{10, 20, 30}, resp.Exclusion.SkuIds)
	assert.Equal(t, "2026-05-01", resp.Exclusion.StartDate)
	assert.Equal(t, "2026-05-31", resp.Exclusion.EndDate)
	assert.Equal(t, int64(42), resp.Exclusion.CreatedBy)
	assert.Equal(t, int64(1), resp.Exclusion.TenantId)
}

func TestCreateExclusion_NoTenant(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola",
		SkuIds:    []int64{10},
		StartDate: "2026-05-01",
		EndDate:   "2026-05-31",
	})
	require.Error(t, err)
}

func TestCreateExclusion_EmptyBrandName(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "",
		SkuIds:    []int64{10},
		StartDate: "2026-05-01",
		EndDate:   "2026-05-31",
	})
	require.Error(t, err)
}

func TestCreateExclusion_EmptySkuIds(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola",
		SkuIds:    nil,
		StartDate: "2026-05-01",
		EndDate:   "2026-05-31",
	})
	require.Error(t, err)
}

func TestCreateExclusion_InvalidDates(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Bad format
	_, err := svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola",
		SkuIds:    []int64{10},
		StartDate: "invalid",
		EndDate:   "2026-05-31",
	})
	require.Error(t, err)

	// End before start
	_, err = svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola",
		SkuIds:    []int64{10},
		StartDate: "2026-06-01",
		EndDate:   "2026-05-31",
	})
	require.Error(t, err)
}

func TestListExclusions(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola", SkuIds: []int64{10}, StartDate: "2026-05-01", EndDate: "2026-05-31",
	})
	svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Pepsi", SkuIds: []int64{20}, StartDate: "2026-05-01", EndDate: "2026-05-31",
	})

	resp, err := svc.ListExclusions(ctx, &v1.ListExclusionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestListExclusions_TenantIsolation(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(2, 43)

	svc.CreateExclusion(ctx1, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola", SkuIds: []int64{10}, StartDate: "2026-05-01", EndDate: "2026-05-31",
	})
	svc.CreateExclusion(ctx2, &v1.CreateExclusionRequest{
		BrandName: "Pepsi", SkuIds: []int64{20}, StartDate: "2026-05-01", EndDate: "2026-05-31",
	})

	resp, err := svc.ListExclusions(ctx1, &v1.ListExclusionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "Coca-Cola", resp.Items[0].BrandName)
}

func TestDeleteExclusion(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	created, _ := svc.CreateExclusion(ctx, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola", SkuIds: []int64{10}, StartDate: "2026-05-01", EndDate: "2026-05-31",
	})

	_, err := svc.DeleteExclusion(ctx, &v1.DeleteExclusionRequest{Id: created.Exclusion.Id})
	require.NoError(t, err)

	resp, _ := svc.ListExclusions(ctx, &v1.ListExclusionsRequest{})
	assert.Len(t, resp.Items, 0)
}

func TestDeleteExclusion_TenantIsolation(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(2, 43)

	created, _ := svc.CreateExclusion(ctx1, &v1.CreateExclusionRequest{
		BrandName: "Coca-Cola", SkuIds: []int64{10}, StartDate: "2026-05-01", EndDate: "2026-05-31",
	})

	// Tenant 2 tries to delete tenant 1's exclusion -- should not affect
	_, err := svc.DeleteExclusion(ctx2, &v1.DeleteExclusionRequest{Id: created.Exclusion.Id})
	require.NoError(t, err)

	// Should still exist for tenant 1
	resp, _ := svc.ListExclusions(ctx1, &v1.ListExclusionsRequest{})
	assert.Len(t, resp.Items, 1)
}

func TestListExclusions_NoTenant(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.ListExclusions(ctx, &v1.ListExclusionsRequest{})
	require.Error(t, err)
}

// ========================================
// Story 7.3: Auto Charge (ClosePeriod)
// ========================================

func TestClosePeriod_CalculatesAndCharges(t *testing.T) {
	svc, uc, commissionsRepo, _, _ := setupService()
	ctx := context.Background()

	// Simulate sale events
	event := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "600"},
			{ProductID: 20, Total: "400"},
		},
	}
	uc.HandleSaleCompleted(ctx, event)
	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	resp, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.ClosedCount)
	assert.Equal(t, int32(0), resp.FailedCount)

	// Verify commission was updated
	for _, c := range commissionsRepo.commissions {
		assert.Equal(t, enum.Charged, c.Status)
		assert.True(t, decimal.NewFromInt(1000).Equal(c.Gmv), "GMV should be 1000, got %s", c.Gmv)
		assert.True(t, decimal.Zero.Equal(c.ExcludedAmount))
		assert.True(t, decimal.NewFromInt(1000).Equal(c.TaxableAmount))
		// commission = 1000 * 0.03 = 30
		expectedCommission := decimal.NewFromInt(1000).Mul(biz.DefaultRate)
		assert.True(t, expectedCommission.Equal(c.Commission), "commission should be %s, got %s", expectedCommission, c.Commission)
	}
}

func TestClosePeriod_WithExclusions(t *testing.T) {
	svc, uc, commissionsRepo, exclusionsRepo, _ := setupService()
	ctx := context.Background()

	// Add exclusion for product 10 (active through end of May)
	exclusionsRepo.Create(ctx, data.ExclusionDto{
		TenantID:  1,
		BrandName: "Coca-Cola",
		SkuIDs:    []int64{10},
		StartDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy: 42,
	})

	// Simulate sales
	event := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "600"}, // excluded
			{ProductID: 20, Total: "400"}, // not excluded
		},
	}
	uc.HandleSaleCompleted(ctx, event)
	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	resp, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.ClosedCount)

	for _, c := range commissionsRepo.commissions {
		assert.True(t, decimal.NewFromInt(1000).Equal(c.Gmv), "GMV should be 1000")
		assert.True(t, decimal.NewFromInt(600).Equal(c.ExcludedAmount), "excluded should be 600, got %s", c.ExcludedAmount)
		assert.True(t, decimal.NewFromInt(400).Equal(c.TaxableAmount), "taxable should be 400, got %s", c.TaxableAmount)
		// commission = 400 * 0.03 = 12
		expectedCommission := decimal.NewFromInt(400).Mul(biz.DefaultRate)
		assert.True(t, expectedCommission.Equal(c.Commission), "commission should be %s, got %s", expectedCommission, c.Commission)
	}
}

func TestClosePeriod_WithReturns(t *testing.T) {
	svc, uc, commissionsRepo, _, _ := setupService()
	ctx := context.Background()

	// Sale of 1000
	saleEvent := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "1000"},
		},
	}
	uc.HandleSaleCompleted(ctx, saleEvent)

	// Return of 200
	returnEvent := messages.ReturnCompletedEvent{
		TenantID: 1, SaleID: 100, ReturnID: 50, Total: "200", Timestamp: "2026-05-16T10:00:00Z",
		Items: []messages.ReturnCompletedEventItem{
			{ProductID: 10, Total: "200"},
		},
	}
	uc.HandleReturnCompleted(ctx, returnEvent)

	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	resp, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.ClosedCount)

	for _, c := range commissionsRepo.commissions {
		// GMV = 1000 - 200 = 800
		assert.True(t, decimal.NewFromInt(800).Equal(c.Gmv), "GMV should be 800, got %s", c.Gmv)
	}
}

func TestClosePeriod_IdempotentOnAlreadyClosed(t *testing.T) {
	svc, uc, _, _, _ := setupService()
	ctx := context.Background()

	event := messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "500", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 10, Total: "500"}},
	}
	uc.HandleSaleCompleted(ctx, event)
	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	// Close period first time
	resp1, _ := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	assert.Equal(t, int32(1), resp1.ClosedCount)

	// Close again -- should find 0 pending
	resp2, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp2.ClosedCount)
}

func TestClosePeriod_InvalidDate(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "invalid"})
	require.Error(t, err)
}

func TestClosePeriod_MultiTenant(t *testing.T) {
	svc, uc, commissionsRepo, _, _ := setupService()
	ctx := context.Background()

	// Tenant 1
	uc.HandleSaleCompleted(ctx, messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 10, Total: "1000"}},
	})
	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	// Tenant 2
	uc.HandleSaleCompleted(ctx, messages.SaleCompletedEvent{
		TenantID: 2, SaleID: 200, Total: "500", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 20, Total: "500"}},
	})
	uc.EnsureCommissionExists(ctx, 2, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	resp, err := svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.ClosedCount)

	// Verify both tenants got charged
	chargedCount := 0
	for _, c := range commissionsRepo.commissions {
		if c.Status == enum.Charged {
			chargedCount++
		}
	}
	assert.Equal(t, 2, chargedCount)
}

// ========================================
// Story 7.4: Billing Report
// ========================================

func TestGetBillingReport_FromCommission(t *testing.T) {
	svc, uc, _, _, _ := setupService()
	ctx := context.Background()

	// Create sale events and close period
	uc.HandleSaleCompleted(ctx, messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "600"},
			{ProductID: 20, Total: "400"},
		},
	})
	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))

	svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})

	resp, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		TenantId:    1,
		PeriodStart: "2026-05-01",
		PeriodEnd:   "2026-06-01",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)

	assert.Equal(t, int64(1), resp.Report.TenantId)
	assert.Equal(t, "2026-05-01", resp.Report.PeriodStart)
	assert.Equal(t, "2026-06-01", resp.Report.PeriodEnd)
	assert.Equal(t, "1000", resp.Report.TotalGmv)
	assert.Equal(t, "0", resp.Report.ExcludedAmount)
	assert.Equal(t, "1000", resp.Report.TaxableAmount)
	assert.Equal(t, "CHARGED", resp.Report.Status)
}

func TestGetBillingReport_OnTheFly(t *testing.T) {
	svc, uc, _, _, _ := setupService()
	ctx := context.Background()

	// Sale events without ClosePeriod
	uc.HandleSaleCompleted(ctx, messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "800", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "800"},
		},
	})

	// Don't create commission, don't close period -- report should still work
	resp, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		TenantId:    1,
		PeriodStart: "2026-05-01",
		PeriodEnd:   "2026-06-01",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)

	assert.Equal(t, "800", resp.Report.TotalGmv)
	assert.Equal(t, "PENDING", resp.Report.Status)
}

func TestGetBillingReport_WithExclusions(t *testing.T) {
	svc, uc, _, exclusionsRepo, _ := setupService()
	ctx := context.Background()

	// Add exclusion for product 10
	exclusionsRepo.Create(ctx, data.ExclusionDto{
		TenantID:  1,
		BrandName: "Test",
		SkuIDs:    []int64{10},
		StartDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy: 42,
	})

	uc.HandleSaleCompleted(ctx, messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1000", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "600"}, // excluded
			{ProductID: 20, Total: "400"}, // not excluded
		},
	})

	resp, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		TenantId:    1,
		PeriodStart: "2026-05-01",
		PeriodEnd:   "2026-06-01",
	})
	require.NoError(t, err)

	assert.Equal(t, "1000", resp.Report.TotalGmv)
	assert.Equal(t, "600", resp.Report.ExcludedAmount)
	assert.Equal(t, "400", resp.Report.TaxableAmount)
}

func TestGetBillingReport_InvalidDates(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		TenantId:    1,
		PeriodStart: "invalid",
		PeriodEnd:   "2026-06-01",
	})
	require.Error(t, err)
}

func TestGetBillingReport_NoTenantId(t *testing.T) {
	svc, _, _, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		PeriodStart: "2026-05-01",
		PeriodEnd:   "2026-06-01",
	})
	require.Error(t, err)
}

func TestGetBillingReport_TransparentBreakdown(t *testing.T) {
	svc, uc, _, exclusionsRepo, _ := setupService()
	ctx := context.Background()

	exclusionsRepo.Create(ctx, data.ExclusionDto{
		TenantID: 1, BrandName: "Brand", SkuIDs: []int64{10},
		StartDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy: 42,
	})

	// Sale
	uc.HandleSaleCompleted(ctx, messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "1500", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{
			{ProductID: 10, Total: "500"}, // excluded
			{ProductID: 20, Total: "700"},
			{ProductID: 30, Total: "300"},
		},
	})

	uc.EnsureCommissionExists(ctx, 1, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))
	svc.ClosePeriod(ctx, &v1.ClosePeriodRequest{PeriodEnd: "2026-06-01"})

	resp, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		TenantId:    1,
		PeriodStart: "2026-05-01",
		PeriodEnd:   "2026-06-01",
	})
	require.NoError(t, err)

	// Verify full transparent breakdown
	assert.Equal(t, "1500", resp.Report.TotalGmv)
	assert.Equal(t, "500", resp.Report.ExcludedAmount)
	assert.Equal(t, "1000", resp.Report.TaxableAmount)
	assert.Equal(t, "0.03", resp.Report.Rate)
	// commission = 1000 * 0.03 = 30
	assert.Equal(t, "30", resp.Report.Commission)
	assert.Equal(t, "CHARGED", resp.Report.Status)
}

func TestGetBillingReport_TenantIdFromContext(t *testing.T) {
	svc, uc, _, _, _ := setupService()

	uc.HandleSaleCompleted(context.Background(), messages.SaleCompletedEvent{
		TenantID: 1, SaleID: 100, Total: "500", Timestamp: "2026-05-15T10:00:00Z",
		Items: []messages.SaleCompletedEventItem{{ProductID: 10, Total: "500"}},
	})

	// Use tenant from context when tenant_id is 0 in request
	ctx := ctxWithTenant(1)
	resp, err := svc.GetBillingReport(ctx, &v1.GetBillingReportRequest{
		TenantId:    0,
		PeriodStart: "2026-05-01",
		PeriodEnd:   "2026-06-01",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Report.TenantId)
}
