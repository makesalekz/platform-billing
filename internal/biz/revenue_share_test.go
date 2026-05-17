package biz

import (
	"context"
	"testing"
	"time"

	"gitlab.calendaria.team/services/platform-billing/ent"
	"gitlab.calendaria.team/services/platform-billing/ent/enum"
	"gitlab.calendaria.team/services/platform-billing/internal/data"
	tenants_v1 "gitlab.calendaria.team/services/tenants/api/tenants/v1"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// --- Minimal mocks for revenue share tests ---

type mockCommissions struct {
	commissions map[int64]*ent.Commission
	nextID      int64
}

func newMockCommissions() *mockCommissions {
	return &mockCommissions{commissions: make(map[int64]*ent.Commission), nextID: 1}
}

func (m *mockCommissions) GetOrCreate(_ context.Context, tenantID int64, start, end time.Time, rate decimal.Decimal) (*ent.Commission, error) {
	c := &ent.Commission{ID: m.nextID, TenantID: tenantID, PeriodStart: start, PeriodEnd: end, Rate: rate, Status: enum.Pending}
	m.commissions[m.nextID] = c
	m.nextID++
	return c, nil
}
func (m *mockCommissions) Get(_ context.Context, _ int64, _, _ time.Time) (*ent.Commission, error) {
	return nil, &ent.NotFoundError{}
}
func (m *mockCommissions) ListPending(_ context.Context, _ time.Time) ([]*ent.Commission, error) {
	var result []*ent.Commission
	for _, c := range m.commissions {
		if c.Status == enum.Pending {
			result = append(result, c)
		}
	}
	return result, nil
}
func (m *mockCommissions) UpdateStatus(_ context.Context, id int64, s enum.CommissionStatus) (*ent.Commission, error) {
	m.commissions[id].Status = s
	return m.commissions[id], nil
}
func (m *mockCommissions) UpdateAmounts(_ context.Context, id int64, gmv, excl, taxable, comm decimal.Decimal) (*ent.Commission, error) {
	c := m.commissions[id]
	c.Gmv = gmv
	c.ExcludedAmount = excl
	c.TaxableAmount = taxable
	c.Commission = comm
	return c, nil
}
func (m *mockCommissions) UpdateManagerShare(_ context.Context, id int64, shareRate, managerShare decimal.Decimal) error {
	c := m.commissions[id]
	c.ManagerShareRate = shareRate
	c.ManagerShare = managerShare
	return nil
}

type mockExcl struct{}

func (m *mockExcl) Create(_ context.Context, _ data.ExclusionDto) (*ent.Exclusion, error) {
	return nil, nil
}
func (m *mockExcl) List(_ context.Context, _ int64) ([]*ent.Exclusion, error) { return nil, nil }
func (m *mockExcl) Delete(_ context.Context, _, _ int64) error                { return nil }
func (m *mockExcl) ListActive(_ context.Context, _ int64, _ time.Time) ([]*ent.Exclusion, error) {
	return nil, nil
}

type mockSaleEvts struct {
	events []*ent.SaleEvent
}

func (m *mockSaleEvts) Upsert(_ context.Context, _ data.SaleEventDto) error { return nil }
func (m *mockSaleEvts) ListByTenantAndPeriod(_ context.Context, _ int64, _, _ time.Time) ([]*ent.SaleEvent, error) {
	return m.events, nil
}

type mockTenantsWithReferred struct {
	referredBy *int64
}

func (m *mockTenantsWithReferred) GetTenant(_ context.Context, _ int64) (*tenants_v1.Tenant, error) {
	return &tenants_v1.Tenant{ReferredBy: m.referredBy}, nil
}

func TestClosePeriod_WithManagerShare(t *testing.T) {
	commissions := newMockCommissions()
	saleEvents := &mockSaleEvts{
		events: []*ent.SaleEvent{
			{TenantID: 1, ProductID: 100, Amount: decimal.NewFromInt(10000)},
		},
	}

	managerID := int64(42)
	tenantsClient := &mockTenantsWithReferred{referredBy: &managerID}

	uc := &BillingUsecase{
		log:           log.NewHelper(log.DefaultLogger),
		commissions:   commissions,
		exclusions:    &mockExcl{},
		saleEvents:    saleEvents,
		tenantsClient: tenantsClient,
	}

	// Create a commission record
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	c, _ := commissions.GetOrCreate(context.Background(), 1, start, end, DefaultRate)

	// Close the period
	closed, failed, err := uc.ClosePeriod(context.Background(), end)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), closed)
	assert.Equal(t, int32(0), failed)

	// Verify manager share was calculated
	// GMV = 10000, commission = 10000 * 0.03 = 300
	// manager_share = 300 * 0.20 = 60
	assert.True(t, c.ManagerShareRate.Equal(DefaultManagerShareRate))
	expectedShare := decimal.NewFromInt(10000).Mul(DefaultRate).Mul(DefaultManagerShareRate) // 60
	assert.True(t, c.ManagerShare.Equal(expectedShare), "expected %s, got %s", expectedShare, c.ManagerShare)
}

func TestClosePeriod_NoReferredBy(t *testing.T) {
	commissions := newMockCommissions()
	saleEvents := &mockSaleEvts{
		events: []*ent.SaleEvent{
			{TenantID: 1, ProductID: 100, Amount: decimal.NewFromInt(5000)},
		},
	}

	// No referred_by
	tenantsClient := &mockTenantsWithReferred{referredBy: nil}

	uc := &BillingUsecase{
		log:           log.NewHelper(log.DefaultLogger),
		commissions:   commissions,
		exclusions:    &mockExcl{},
		saleEvents:    saleEvents,
		tenantsClient: tenantsClient,
	}

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	c, _ := commissions.GetOrCreate(context.Background(), 1, start, end, DefaultRate)

	closed, _, err := uc.ClosePeriod(context.Background(), end)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), closed)

	// Manager share should remain zero (no referred_by)
	assert.True(t, c.ManagerShare.IsZero())
	assert.True(t, c.ManagerShareRate.IsZero())
}
