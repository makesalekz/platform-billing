package schema

import (
	"github.com/makesalekz/platform-billing/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Exclusion struct {
	ent.Schema
}

func (Exclusion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("tenant_id").Immutable(),
		field.String("brand_name").NotEmpty(),
		field.JSON("sku_ids", []int64{}),
		field.Time("start_date"),
		field.Time("end_date"),
		field.Int64("created_by"),
	}
}

func (Exclusion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
	}
}

func (Exclusion) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
