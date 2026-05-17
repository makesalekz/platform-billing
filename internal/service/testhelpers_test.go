package service

import (
	"context"
	"time"

	"gitlab.calendaria.team/services/platform-billing/ent"
	"gitlab.calendaria.team/services/platform-billing/ent/enum"
	"gitlab.calendaria.team/services/platform-billing/internal/biz"
	"gitlab.calendaria.team/services/platform-billing/internal/data"
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
)

// --- Mock CommissionsRepo ---

type mockCommissionsRepo struct {
	commissions map[int64]*ent.Commission
	nextID      int64
}

func newMockCommissionsRepo() *mockCommissionsRepo {
	return &mockCommissionsRepo{
		commissions: make(map[int64]*ent.Commission),
		nextID:      1,
	}
}

func (m *mockCommissionsRepo) GetOrCreate(_ context.Context, tenantID int64, periodStart, periodEnd time.Time, rate decimal.Decimal) (*ent.Commission, error) {
	for _, c := range m.commissions {
		if c.TenantID == tenantID && c.PeriodStart.Equal(periodStart) && c.PeriodEnd.Equal(periodEnd) {
			return c, nil
		}
	}
	c := &ent.Commission{
		ID:          m.nextID,
		TenantID:    tenantID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Rate:        rate,
		Status:      enum.Pending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.commissions[m.nextID] = c
	m.nextID++
	return c, nil
}

func (m *mockCommissionsRepo) Get(_ context.Context, tenantID int64, periodStart, periodEnd time.Time) (*ent.Commission, error) {
	for _, c := range m.commissions {
		if c.TenantID == tenantID && c.PeriodStart.Equal(periodStart) && c.PeriodEnd.Equal(periodEnd) {
			return c, nil
		}
	}
	return nil, &ent.NotFoundError{}
}

func (m *mockCommissionsRepo) ListPending(_ context.Context, periodEnd time.Time) ([]*ent.Commission, error) {
	var result []*ent.Commission
	for _, c := range m.commissions {
		if c.Status == enum.Pending && !c.PeriodEnd.After(periodEnd) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockCommissionsRepo) UpdateStatus(_ context.Context, id int64, status enum.CommissionStatus) (*ent.Commission, error) {
	c, ok := m.commissions[id]
	if !ok {
		return nil, &ent.NotFoundError{}
	}
	c.Status = status
	return c, nil
}

func (m *mockCommissionsRepo) UpdateAmounts(_ context.Context, id int64, gmv, excludedAmount, taxableAmount, commissionAmount decimal.Decimal) (*ent.Commission, error) {
	c, ok := m.commissions[id]
	if !ok {
		return nil, &ent.NotFoundError{}
	}
	c.Gmv = gmv
	c.ExcludedAmount = excludedAmount
	c.TaxableAmount = taxableAmount
	c.Commission = commissionAmount
	return c, nil
}

// --- Mock ExclusionsRepo ---

type mockExclusionsRepo struct {
	exclusions map[int64]*ent.Exclusion
	nextID     int64
}

func newMockExclusionsRepo() *mockExclusionsRepo {
	return &mockExclusionsRepo{
		exclusions: make(map[int64]*ent.Exclusion),
		nextID:     1,
	}
}

func (m *mockExclusionsRepo) Create(_ context.Context, dto data.ExclusionDto) (*ent.Exclusion, error) {
	e := &ent.Exclusion{
		ID:        m.nextID,
		TenantID:  dto.TenantID,
		BrandName: dto.BrandName,
		SkuIds:    dto.SkuIDs,
		StartDate: dto.StartDate,
		EndDate:   dto.EndDate,
		CreatedBy: dto.CreatedBy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.exclusions[m.nextID] = e
	m.nextID++
	return e, nil
}

func (m *mockExclusionsRepo) List(_ context.Context, tenantID int64) ([]*ent.Exclusion, error) {
	var result []*ent.Exclusion
	for _, e := range m.exclusions {
		if e.TenantID == tenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockExclusionsRepo) Delete(_ context.Context, tenantID, id int64) error {
	e, ok := m.exclusions[id]
	if !ok || e.TenantID != tenantID {
		return nil
	}
	delete(m.exclusions, id)
	return nil
}

func (m *mockExclusionsRepo) ListActive(_ context.Context, tenantID int64, at time.Time) ([]*ent.Exclusion, error) {
	var result []*ent.Exclusion
	for _, e := range m.exclusions {
		if e.TenantID == tenantID && !e.StartDate.After(at) && !e.EndDate.Before(at) {
			result = append(result, e)
		}
	}
	return result, nil
}

// --- Mock SaleEventsRepo ---

type mockSaleEventsRepo struct {
	events map[int64]*ent.SaleEvent
	nextID int64
}

func newMockSaleEventsRepo() *mockSaleEventsRepo {
	return &mockSaleEventsRepo{
		events: make(map[int64]*ent.SaleEvent),
		nextID: 1,
	}
}

func (m *mockSaleEventsRepo) Upsert(_ context.Context, dto data.SaleEventDto) error {
	// Check for existing (tenant_id, sale_id, product_id, is_return)
	for _, ev := range m.events {
		if ev.TenantID == dto.TenantID && ev.SaleID == dto.SaleID && ev.ProductID == dto.ProductID && ev.IsReturn == dto.IsReturn {
			ev.Amount = dto.Amount
			ev.CompletedAt = dto.CompletedAt
			return nil
		}
	}
	ev := &ent.SaleEvent{
		ID:          m.nextID,
		TenantID:    dto.TenantID,
		SaleID:      dto.SaleID,
		ProductID:   dto.ProductID,
		Amount:      dto.Amount,
		CompletedAt: dto.CompletedAt,
		IsReturn:    dto.IsReturn,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.events[m.nextID] = ev
	m.nextID++
	return nil
}

func (m *mockSaleEventsRepo) ListByTenantAndPeriod(_ context.Context, tenantID int64, from, to time.Time) ([]*ent.SaleEvent, error) {
	var result []*ent.SaleEvent
	for _, ev := range m.events {
		if ev.TenantID == tenantID && !ev.CompletedAt.Before(from) && ev.CompletedAt.Before(to) {
			result = append(result, ev)
		}
	}
	return result, nil
}

// --- Test setup ---

func setupService() (*PlatformBillingService, *biz.BillingUsecase, *mockCommissionsRepo, *mockExclusionsRepo, *mockSaleEventsRepo) {
	commissionsRepo := newMockCommissionsRepo()
	exclusionsRepo := newMockExclusionsRepo()
	saleEventsRepo := newMockSaleEventsRepo()

	uc := biz.NewBillingUsecase(log.DefaultLogger, commissionsRepo, exclusionsRepo, saleEventsRepo, nil)
	svc := NewPlatformBillingService(uc)
	return svc, uc, commissionsRepo, exclusionsRepo, saleEventsRepo
}

func ctxWithTenant(tenantID int64) context.Context {
	return auth.NewTenantContext(context.Background(), tenantID)
}

func ctxWithTenantAndActor(tenantID, actorID int64) context.Context {
	ctx := auth.NewTenantContext(context.Background(), tenantID)
	return auth.NewActorContext(ctx, actorID)
}
