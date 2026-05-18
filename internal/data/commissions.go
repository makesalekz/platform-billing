package data

import (
	"context"
	"time"

	"github.com/makesalekz/platform-billing/ent"
	"github.com/makesalekz/platform-billing/ent/commission"
	"github.com/makesalekz/platform-billing/ent/enum"

	"github.com/shopspring/decimal"
)

type CommissionsRepo interface {
	GetOrCreate(ctx context.Context, tenantID int64, periodStart, periodEnd time.Time, rate decimal.Decimal) (*ent.Commission, error)
	Get(ctx context.Context, tenantID int64, periodStart, periodEnd time.Time) (*ent.Commission, error)
	ListPending(ctx context.Context, periodEnd time.Time) ([]*ent.Commission, error)
	UpdateStatus(ctx context.Context, id int64, status enum.CommissionStatus) (*ent.Commission, error)
	UpdateAmounts(ctx context.Context, id int64, gmv, excludedAmount, taxableAmount, commissionAmount decimal.Decimal) (*ent.Commission, error)
	UpdateManagerShare(ctx context.Context, id int64, shareRate, managerShare decimal.Decimal) error
}

type commissionsRepo struct {
	db *ent.Client
}

func NewCommissionsRepo(d *Data) CommissionsRepo {
	return &commissionsRepo{db: d.db}
}

func (r *commissionsRepo) GetOrCreate(ctx context.Context, tenantID int64, periodStart, periodEnd time.Time, rate decimal.Decimal) (*ent.Commission, error) {
	// Try to find existing
	c, err := r.db.Commission.Query().
		Where(
			commission.TenantID(tenantID),
			commission.PeriodStart(periodStart),
			commission.PeriodEnd(periodEnd),
		).
		Only(ctx)
	if err == nil {
		return c, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}

	// Create new
	return r.db.Commission.Create().
		SetTenantID(tenantID).
		SetPeriodStart(periodStart).
		SetPeriodEnd(periodEnd).
		SetRate(rate).
		SetStatus(enum.Pending).
		Save(ctx)
}

func (r *commissionsRepo) Get(ctx context.Context, tenantID int64, periodStart, periodEnd time.Time) (*ent.Commission, error) {
	return r.db.Commission.Query().
		Where(
			commission.TenantID(tenantID),
			commission.PeriodStart(periodStart),
			commission.PeriodEnd(periodEnd),
		).
		Only(ctx)
}

func (r *commissionsRepo) ListPending(ctx context.Context, periodEnd time.Time) ([]*ent.Commission, error) {
	return r.db.Commission.Query().
		Where(
			commission.StatusEQ(enum.Pending),
			commission.PeriodEndLTE(periodEnd),
		).
		All(ctx)
}

func (r *commissionsRepo) UpdateStatus(ctx context.Context, id int64, status enum.CommissionStatus) (*ent.Commission, error) {
	return r.db.Commission.UpdateOneID(id).
		SetStatus(status).
		Save(ctx)
}

func (r *commissionsRepo) UpdateAmounts(ctx context.Context, id int64, gmv, excludedAmount, taxableAmount, commissionAmount decimal.Decimal) (*ent.Commission, error) {
	return r.db.Commission.UpdateOneID(id).
		SetGmv(gmv).
		SetExcludedAmount(excludedAmount).
		SetTaxableAmount(taxableAmount).
		SetCommission(commissionAmount).
		Save(ctx)
}

func (r *commissionsRepo) UpdateManagerShare(ctx context.Context, id int64, shareRate, managerShare decimal.Decimal) error {
	return r.db.Commission.UpdateOneID(id).
		SetManagerShareRate(shareRate).
		SetManagerShare(managerShare).
		Exec(ctx)
}
