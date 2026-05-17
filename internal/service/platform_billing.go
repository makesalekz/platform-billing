package service

import (
	"context"
	"time"

	v1 "gitlab.calendaria.team/services/platform-billing/api/platform_billing/v1"
	"gitlab.calendaria.team/services/platform-billing/ent"
	"gitlab.calendaria.team/services/platform-billing/internal/biz"
	"gitlab.calendaria.team/services/platform-billing/internal/data"
	"gitlab.calendaria.team/services/utils/v2/auth"
)

type PlatformBillingService struct {
	v1.UnimplementedPlatformBillingServiceServer

	uc *biz.BillingUsecase
}

func NewPlatformBillingService(uc *biz.BillingUsecase) *PlatformBillingService {
	return &PlatformBillingService{uc: uc}
}

func (s *PlatformBillingService) CreateExclusion(ctx context.Context, req *v1.CreateExclusionRequest) (*v1.CreateExclusionReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	actorID := auth.GetActorIdFromContext(ctx)

	if req.GetBrandName() == "" {
		return nil, v1.ErrorInvalidRequest("brand_name is required")
	}
	if len(req.GetSkuIds()) == 0 {
		return nil, v1.ErrorInvalidRequest("sku_ids is required")
	}

	startDate, err := time.Parse("2006-01-02", req.GetStartDate())
	if err != nil {
		return nil, v1.ErrorInvalidRequest("invalid start_date format, use YYYY-MM-DD")
	}
	endDate, err := time.Parse("2006-01-02", req.GetEndDate())
	if err != nil {
		return nil, v1.ErrorInvalidRequest("invalid end_date format, use YYYY-MM-DD")
	}
	if endDate.Before(startDate) {
		return nil, v1.ErrorInvalidRequest("end_date must be after start_date")
	}

	dto := data.ExclusionDto{
		TenantID:  tenantID,
		BrandName: req.GetBrandName(),
		SkuIDs:    req.GetSkuIds(),
		StartDate: startDate,
		EndDate:   endDate,
		CreatedBy: actorID,
	}

	excl, err := s.uc.CreateExclusion(ctx, dto)
	if err != nil {
		return nil, err
	}

	return &v1.CreateExclusionReply{Exclusion: replyExclusion(excl)}, nil
}

func (s *PlatformBillingService) ListExclusions(ctx context.Context, _ *v1.ListExclusionsRequest) (*v1.ListExclusionsReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	items, err := s.uc.ListExclusions(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	result := make([]*v1.Exclusion, 0, len(items))
	for _, item := range items {
		result = append(result, replyExclusion(item))
	}

	return &v1.ListExclusionsReply{Items: result}, nil
}

func (s *PlatformBillingService) DeleteExclusion(ctx context.Context, req *v1.DeleteExclusionRequest) (*v1.DeleteExclusionReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	if err := s.uc.DeleteExclusion(ctx, tenantID, req.GetId()); err != nil {
		return nil, err
	}

	return &v1.DeleteExclusionReply{}, nil
}

func (s *PlatformBillingService) ClosePeriod(ctx context.Context, req *v1.ClosePeriodRequest) (*v1.ClosePeriodReply, error) {
	periodEnd, err := time.Parse("2006-01-02", req.GetPeriodEnd())
	if err != nil {
		return nil, v1.ErrorInvalidRequest("invalid period_end format, use YYYY-MM-DD")
	}

	closed, failed, err := s.uc.ClosePeriod(ctx, periodEnd)
	if err != nil {
		return nil, err
	}

	return &v1.ClosePeriodReply{
		ClosedCount: closed,
		FailedCount: failed,
	}, nil
}

func (s *PlatformBillingService) GetBillingReport(ctx context.Context, req *v1.GetBillingReportRequest) (*v1.GetBillingReportReply, error) {
	tenantID := req.GetTenantId()
	if tenantID == 0 {
		tenantID = auth.GetTenantIdFromContext(ctx)
	}
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("tenant_id is required")
	}

	periodStart, err := time.Parse("2006-01-02", req.GetPeriodStart())
	if err != nil {
		return nil, v1.ErrorInvalidRequest("invalid period_start format, use YYYY-MM-DD")
	}
	periodEnd, err := time.Parse("2006-01-02", req.GetPeriodEnd())
	if err != nil {
		return nil, v1.ErrorInvalidRequest("invalid period_end format, use YYYY-MM-DD")
	}

	report, err := s.uc.GetBillingReport(ctx, tenantID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	return &v1.GetBillingReportReply{
		Report: &v1.BillingReport{
			TenantId:       report.TenantID,
			PeriodStart:    report.PeriodStart.Format("2006-01-02"),
			PeriodEnd:      report.PeriodEnd.Format("2006-01-02"),
			TotalGmv:       report.TotalGMV.String(),
			ExcludedAmount: report.ExcludedAmount.String(),
			TaxableAmount:  report.TaxableAmount.String(),
			Rate:           report.Rate.String(),
			Commission:     report.Commission.String(),
			Status:         string(report.Status),
		},
	}, nil
}

func replyExclusion(e *ent.Exclusion) *v1.Exclusion {
	return &v1.Exclusion{
		Id:        e.ID,
		TenantId:  e.TenantID,
		BrandName: e.BrandName,
		SkuIds:    e.SkuIds,
		StartDate: e.StartDate.Format("2006-01-02"),
		EndDate:   e.EndDate.Format("2006-01-02"),
		CreatedBy: e.CreatedBy,
		CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
