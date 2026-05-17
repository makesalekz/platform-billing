package biz

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.calendaria.team/services/platform-billing/ent"
	"gitlab.calendaria.team/services/platform-billing/ent/enum"
	"gitlab.calendaria.team/services/platform-billing/internal/data"
	"gitlab.calendaria.team/services/platform-billing/messages"

	"github.com/go-kratos/kratos/v2/log"
	nats "github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
)

// Default commission rate (3%).
var DefaultRate = decimal.NewFromFloat(0.03)

// DefaultManagerShareRate is the default revenue share percentage for managers (20%).
var DefaultManagerShareRate = decimal.NewFromFloat(0.20)

type BillingUsecase struct {
	log            *log.Helper
	commissions    data.CommissionsRepo
	exclusions     data.ExclusionsRepo
	saleEvents     data.SaleEventsRepo
	tenantsClient  data.TenantsClient
	nc             *nats.Conn
}

func NewBillingUsecase(
	logger log.Logger,
	commissions data.CommissionsRepo,
	exclusions data.ExclusionsRepo,
	saleEvents data.SaleEventsRepo,
	tenantsClient data.TenantsClient,
	nc *nats.Conn,
) *BillingUsecase {
	return &BillingUsecase{
		log:           log.NewHelper(logger),
		commissions:   commissions,
		exclusions:    exclusions,
		saleEvents:    saleEvents,
		tenantsClient: tenantsClient,
		nc:            nc,
	}
}

// HandleSaleCompleted processes a sale.completed event from NATS.
func (uc *BillingUsecase) HandleSaleCompleted(ctx context.Context, event messages.SaleCompletedEvent) error {
	completedAt, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		completedAt = time.Now()
	}

	for _, item := range event.Items {
		amount, _ := decimal.NewFromString(item.Total)
		dto := data.SaleEventDto{
			TenantID:    event.TenantID,
			SaleID:      event.SaleID,
			ProductID:   item.ProductID,
			Amount:      amount,
			CompletedAt: completedAt,
			IsReturn:    false,
		}
		if err := uc.saleEvents.Upsert(ctx, dto); err != nil {
			uc.log.Errorf("failed to upsert sale event: %v", err)
		}
	}

	// Ensure a commission record exists for this tenant's period
	uc.EnsureCommissionExists(ctx, event.TenantID, completedAt)

	return nil
}

// HandleReturnCompleted processes a return.completed event from NATS.
func (uc *BillingUsecase) HandleReturnCompleted(ctx context.Context, event messages.ReturnCompletedEvent) error {
	completedAt, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		completedAt = time.Now()
	}

	for _, item := range event.Items {
		amount, _ := decimal.NewFromString(item.Total)
		dto := data.SaleEventDto{
			TenantID:    event.TenantID,
			SaleID:      event.SaleID,
			ProductID:   item.ProductID,
			Amount:      amount.Neg(), // Returns subtract from GMV
			CompletedAt: completedAt,
			IsReturn:    true,
		}
		if err := uc.saleEvents.Upsert(ctx, dto); err != nil {
			uc.log.Errorf("failed to upsert return event: %v", err)
		}
	}

	// Ensure a commission record exists for this tenant's period
	uc.EnsureCommissionExists(ctx, event.TenantID, completedAt)

	return nil
}

// CreateExclusion creates a brand sponsorship exclusion.
func (uc *BillingUsecase) CreateExclusion(ctx context.Context, dto data.ExclusionDto) (*ent.Exclusion, error) {
	return uc.exclusions.Create(ctx, dto)
}

// ListExclusions returns all exclusions for a tenant.
func (uc *BillingUsecase) ListExclusions(ctx context.Context, tenantID int64) ([]*ent.Exclusion, error) {
	return uc.exclusions.List(ctx, tenantID)
}

// DeleteExclusion removes an exclusion.
func (uc *BillingUsecase) DeleteExclusion(ctx context.Context, tenantID, id int64) error {
	return uc.exclusions.Delete(ctx, tenantID, id)
}

// ClosePeriod finalizes all PENDING commissions for a given period_end.
// It aggregates sale events, applies exclusions, calculates commission.
func (uc *BillingUsecase) ClosePeriod(ctx context.Context, periodEnd time.Time) (closed, failed int32, err error) {
	commissions, err := uc.commissions.ListPending(ctx, periodEnd)
	if err != nil {
		return 0, 0, err
	}

	for _, c := range commissions {
		if err := uc.closeCommission(ctx, c); err != nil {
			uc.log.Errorf("failed to close commission %d: %v", c.ID, err)
			uc.commissions.UpdateStatus(ctx, c.ID, enum.Failed)
			failed++
		} else {
			closed++
		}
	}

	return closed, failed, nil
}

func (uc *BillingUsecase) closeCommission(ctx context.Context, c *ent.Commission) error {
	// Aggregate sale events for this tenant+period
	events, err := uc.saleEvents.ListByTenantAndPeriod(ctx, c.TenantID, c.PeriodStart, c.PeriodEnd)
	if err != nil {
		return err
	}

	// Get active exclusions for this tenant during the period
	// Use period_end minus 1 second to check within the period boundary
	checkTime := c.PeriodEnd.Add(-time.Second)
	activeExclusions, err := uc.exclusions.ListActive(ctx, c.TenantID, checkTime)
	if err != nil {
		return err
	}

	// Build excluded SKU set
	excludedSKUs := make(map[int64]bool)
	for _, excl := range activeExclusions {
		for _, skuID := range excl.SkuIds {
			excludedSKUs[skuID] = true
		}
	}

	// Calculate GMV and excluded amount
	gmv := decimal.Zero
	excludedAmount := decimal.Zero
	for _, ev := range events {
		gmv = gmv.Add(ev.Amount)
		if excludedSKUs[ev.ProductID] {
			excludedAmount = excludedAmount.Add(ev.Amount)
		}
	}

	taxableAmount := gmv.Sub(excludedAmount)
	if taxableAmount.IsNegative() {
		taxableAmount = decimal.Zero
	}
	commissionAmount := taxableAmount.Mul(c.Rate)

	// Update commission record
	_, err = uc.commissions.UpdateAmounts(ctx, c.ID, gmv, excludedAmount, taxableAmount, commissionAmount)
	if err != nil {
		return err
	}

	// Revenue share: lookup referred_by via tenants service (Story 10.5)
	uc.calculateManagerShare(ctx, c.ID, c.TenantID, commissionAmount)

	_, err = uc.commissions.UpdateStatus(ctx, c.ID, enum.Charged)
	if err != nil {
		return err
	}

	// Publish invoice created event
	uc.publishInvoiceCreated(c.TenantID, c.PeriodStart, c.PeriodEnd, commissionAmount)

	return nil
}

// calculateManagerShare looks up the tenant's referred_by and calculates manager's cut.
func (uc *BillingUsecase) calculateManagerShare(ctx context.Context, commissionID, tenantID int64, commissionAmount decimal.Decimal) {
	if uc.tenantsClient == nil {
		return
	}

	tenant, err := uc.tenantsClient.GetTenant(ctx, tenantID)
	if err != nil {
		uc.log.Warnf("failed to get tenant %d for revenue share: %v", tenantID, err)
		return
	}

	if tenant == nil || tenant.ReferredBy == nil || *tenant.ReferredBy == 0 {
		return
	}

	// Use default share rate
	shareRate := DefaultManagerShareRate
	managerShare := commissionAmount.Mul(shareRate)

	if err := uc.commissions.UpdateManagerShare(ctx, commissionID, shareRate, managerShare); err != nil {
		uc.log.Errorf("failed to update manager share for commission %d: %v", commissionID, err)
	}
}

func (uc *BillingUsecase) publishInvoiceCreated(tenantID int64, periodStart, periodEnd time.Time, amount decimal.Decimal) {
	if uc.nc == nil {
		return
	}

	event := messages.InvoiceCreatedEvent{
		TenantID:    tenantID,
		PeriodStart: periodStart.Format(time.RFC3339),
		PeriodEnd:   periodEnd.Format(time.RFC3339),
		Amount:      amount.String(),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		uc.log.Errorf("failed to marshal invoice created event: %v", err)
		return
	}

	if err := uc.nc.Publish("platform-billing.invoice.created", payload); err != nil {
		uc.log.Errorf("failed to publish invoice created event: %v", err)
	}
}

// GetBillingReport returns the billing report for a tenant and period.
func (uc *BillingUsecase) GetBillingReport(ctx context.Context, tenantID int64, periodStart, periodEnd time.Time) (*BillingReport, error) {
	c, err := uc.commissions.Get(ctx, tenantID, periodStart, periodEnd)
	if err != nil {
		if ent.IsNotFound(err) {
			// Calculate on-the-fly for periods without a commission record
			return uc.calculateReport(ctx, tenantID, periodStart, periodEnd)
		}
		return nil, err
	}

	// For PENDING commissions, calculate on-the-fly from sale events
	// (amounts are only finalized at ClosePeriod)
	if c.Status == enum.Pending {
		return uc.calculateReport(ctx, tenantID, periodStart, periodEnd)
	}

	return &BillingReport{
		TenantID:       c.TenantID,
		PeriodStart:    c.PeriodStart,
		PeriodEnd:      c.PeriodEnd,
		TotalGMV:       c.Gmv,
		ExcludedAmount: c.ExcludedAmount,
		TaxableAmount:  c.TaxableAmount,
		Rate:           c.Rate,
		Commission:     c.Commission,
		Status:         c.Status,
	}, nil
}

func (uc *BillingUsecase) calculateReport(ctx context.Context, tenantID int64, periodStart, periodEnd time.Time) (*BillingReport, error) {
	events, err := uc.saleEvents.ListByTenantAndPeriod(ctx, tenantID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	checkTime := periodEnd.Add(-time.Second)
	activeExclusions, err := uc.exclusions.ListActive(ctx, tenantID, checkTime)
	if err != nil {
		return nil, err
	}

	excludedSKUs := make(map[int64]bool)
	for _, excl := range activeExclusions {
		for _, skuID := range excl.SkuIds {
			excludedSKUs[skuID] = true
		}
	}

	gmv := decimal.Zero
	excludedAmount := decimal.Zero
	for _, ev := range events {
		gmv = gmv.Add(ev.Amount)
		if excludedSKUs[ev.ProductID] {
			excludedAmount = excludedAmount.Add(ev.Amount)
		}
	}

	taxableAmount := gmv.Sub(excludedAmount)
	if taxableAmount.IsNegative() {
		taxableAmount = decimal.Zero
	}
	commissionAmount := taxableAmount.Mul(DefaultRate)

	return &BillingReport{
		TenantID:       tenantID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		TotalGMV:       gmv,
		ExcludedAmount: excludedAmount,
		TaxableAmount:  taxableAmount,
		Rate:           DefaultRate,
		Commission:     commissionAmount,
		Status:         enum.Pending,
	}, nil
}

// EnsureCommissionExists creates a commission record for a tenant+period if it doesn't exist.
// Called by the consumer when a sale event arrives.
func (uc *BillingUsecase) EnsureCommissionExists(ctx context.Context, tenantID int64, completedAt time.Time) {
	periodStart, periodEnd := MonthPeriod(completedAt)
	_, err := uc.commissions.GetOrCreate(ctx, tenantID, periodStart, periodEnd, DefaultRate)
	if err != nil {
		uc.log.Errorf("failed to ensure commission exists for tenant %d: %v", tenantID, err)
	}
}

// MonthPeriod returns the first and last+1 day of the month containing t.
func MonthPeriod(t time.Time) (time.Time, time.Time) {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return start, end
}

type BillingReport struct {
	TenantID       int64
	PeriodStart    time.Time
	PeriodEnd      time.Time
	TotalGMV       decimal.Decimal
	ExcludedAmount decimal.Decimal
	TaxableAmount  decimal.Decimal
	Rate           decimal.Decimal
	Commission     decimal.Decimal
	Status         enum.CommissionStatus
}
