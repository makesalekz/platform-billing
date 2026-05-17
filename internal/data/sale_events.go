package data

import (
	"context"
	"time"

	"gitlab.calendaria.team/services/platform-billing/ent"
	"gitlab.calendaria.team/services/platform-billing/ent/saleevent"
)

type SaleEventsRepo interface {
	Upsert(ctx context.Context, dto SaleEventDto) error
	ListByTenantAndPeriod(ctx context.Context, tenantID int64, from, to time.Time) ([]*ent.SaleEvent, error)
}

type saleEventsRepo struct {
	db *ent.Client
}

func NewSaleEventsRepo(d *Data) SaleEventsRepo {
	return &saleEventsRepo{db: d.db}
}

func (r *saleEventsRepo) Upsert(ctx context.Context, dto SaleEventDto) error {
	return r.db.SaleEvent.Create().
		SetTenantID(dto.TenantID).
		SetSaleID(dto.SaleID).
		SetProductID(dto.ProductID).
		SetAmount(dto.Amount).
		SetCompletedAt(dto.CompletedAt).
		SetIsReturn(dto.IsReturn).
		OnConflictColumns(saleevent.FieldTenantID, saleevent.FieldSaleID, saleevent.FieldProductID, saleevent.FieldIsReturn).
		UpdateAmount().
		UpdateCompletedAt().
		Exec(ctx)
}

func (r *saleEventsRepo) ListByTenantAndPeriod(ctx context.Context, tenantID int64, from, to time.Time) ([]*ent.SaleEvent, error) {
	return r.db.SaleEvent.Query().
		Where(
			saleevent.TenantID(tenantID),
			saleevent.CompletedAtGTE(from),
			saleevent.CompletedAtLT(to),
		).
		All(ctx)
}
