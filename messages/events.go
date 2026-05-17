package messages

// SaleCompletedEventItem mirrors the sales service event item structure.
type SaleCompletedEventItem struct {
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Quantity    string `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
	Discount    string `json:"discount"`
	Total       string `json:"total"`
}

// SaleCompletedEvent is the payload of "sales.sale.completed" NATS subject.
type SaleCompletedEvent struct {
	TenantID  int64                    `json:"tenant_id"`
	SaleID    int64                    `json:"sale_id"`
	Total     string                   `json:"total"`
	Items     []SaleCompletedEventItem `json:"items"`
	Timestamp string                   `json:"timestamp"`
}

// ReturnCompletedEventItem mirrors the sales service return event item.
type ReturnCompletedEventItem struct {
	ProductID  int64  `json:"product_id"`
	SaleItemID int64  `json:"sale_item_id"`
	Quantity   string `json:"quantity"`
	UnitPrice  string `json:"unit_price"`
	Total      string `json:"total"`
}

// ReturnCompletedEvent is the payload of "sales.return.completed" NATS subject.
type ReturnCompletedEvent struct {
	TenantID  int64                      `json:"tenant_id"`
	SaleID    int64                      `json:"sale_id"`
	ReturnID  int64                      `json:"return_id"`
	Total     string                     `json:"total"`
	Items     []ReturnCompletedEventItem `json:"items"`
	Timestamp string                     `json:"timestamp"`
}

// InvoiceCreatedEvent is published to "platform-billing.invoice.created".
type InvoiceCreatedEvent struct {
	TenantID    int64  `json:"tenant_id"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Amount      string `json:"amount"`
	Timestamp   string `json:"timestamp"`
}
