package data

import (
	"context"
	"time"

	"gitlab.calendaria.team/services/platform-billing/ent"
	"gitlab.calendaria.team/services/platform-billing/ent/exclusion"
)

type ExclusionsRepo interface {
	Create(ctx context.Context, dto ExclusionDto) (*ent.Exclusion, error)
	List(ctx context.Context, tenantID int64) ([]*ent.Exclusion, error)
	Delete(ctx context.Context, tenantID, id int64) error
	ListActive(ctx context.Context, tenantID int64, at time.Time) ([]*ent.Exclusion, error)
}

type exclusionsRepo struct {
	db *ent.Client
}

func NewExclusionsRepo(d *Data) ExclusionsRepo {
	return &exclusionsRepo{db: d.db}
}

func (r *exclusionsRepo) Create(ctx context.Context, dto ExclusionDto) (*ent.Exclusion, error) {
	return r.db.Exclusion.Create().
		SetTenantID(dto.TenantID).
		SetBrandName(dto.BrandName).
		SetSkuIds(dto.SkuIDs).
		SetStartDate(dto.StartDate).
		SetEndDate(dto.EndDate).
		SetCreatedBy(dto.CreatedBy).
		Save(ctx)
}

func (r *exclusionsRepo) List(ctx context.Context, tenantID int64) ([]*ent.Exclusion, error) {
	return r.db.Exclusion.Query().
		Where(exclusion.TenantID(tenantID)).
		Order(ent.Desc(exclusion.FieldCreatedAt)).
		All(ctx)
}

func (r *exclusionsRepo) Delete(ctx context.Context, tenantID, id int64) error {
	_, err := r.db.Exclusion.Delete().
		Where(exclusion.ID(id), exclusion.TenantID(tenantID)).
		Exec(ctx)
	return err
}

func (r *exclusionsRepo) ListActive(ctx context.Context, tenantID int64, at time.Time) ([]*ent.Exclusion, error) {
	return r.db.Exclusion.Query().
		Where(
			exclusion.TenantID(tenantID),
			exclusion.StartDateLTE(at),
			exclusion.EndDateGTE(at),
		).
		All(ctx)
}
