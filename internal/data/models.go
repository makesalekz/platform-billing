package data

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/makesalekz/platform-billing/ent/enum"
)

type CommissionDto struct {
	ID             int64
	TenantID       int64
	PeriodStart    time.Time
	PeriodEnd      time.Time
	GMV            decimal.Decimal
	ExcludedAmount decimal.Decimal
	TaxableAmount  decimal.Decimal
	Rate           decimal.Decimal
	Commission     decimal.Decimal
	Status         enum.CommissionStatus
}

type ExclusionDto struct {
	ID        int64
	TenantID  int64
	BrandName string
	SkuIDs    []int64
	StartDate time.Time
	EndDate   time.Time
	CreatedBy int64
}

type SaleEventDto struct {
	TenantID    int64
	SaleID      int64
	ProductID   int64
	Amount      decimal.Decimal
	CompletedAt time.Time
	IsReturn    bool
}
