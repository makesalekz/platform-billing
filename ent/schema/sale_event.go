package schema

import (
	"gitlab.calendaria.team/services/platform-billing/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// SaleEvent stores raw sale/return events from NATS for retroactive aggregation.
type SaleEvent struct {
	ent.Schema
}

func (SaleEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("tenant_id").Immutable(),
		field.Int64("sale_id").Immutable(),
		field.Int64("product_id").Immutable(),
		field.Float("amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Time("completed_at"),
		field.Bool("is_return").Default(false),
	}
}

func (SaleEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "completed_at"),
		index.Fields("tenant_id", "sale_id", "product_id", "is_return").Unique(),
	}
}

func (SaleEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
