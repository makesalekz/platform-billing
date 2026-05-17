package schema

import (
	"gitlab.calendaria.team/services/platform-billing/ent/enum"
	"gitlab.calendaria.team/services/platform-billing/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

type Commission struct {
	ent.Schema
}

func (Commission) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("tenant_id").Immutable(),
		field.Time("period_start"),
		field.Time("period_end"),
		field.Float("gmv").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("excluded_amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("taxable_amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("rate").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("commission").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("manager_share_rate").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("manager_share").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Enum("status").GoType(enum.CommissionStatus("")).Default(enum.Pending.Value()),
	}
}

func (Commission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "period_start", "period_end").Unique(),
	}
}

func (Commission) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
