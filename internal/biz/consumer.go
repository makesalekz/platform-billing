package biz

import (
	"context"
	"encoding/json"

	"github.com/makesalekz/platform-billing/messages"

	"github.com/go-kratos/kratos/v2/log"
	nats "github.com/nats-io/nats.go"
)

// Consumer subscribes to sales NATS events and delegates to BillingUsecase.
type Consumer struct {
	log  *log.Helper
	uc   *BillingUsecase
	subs []*nats.Subscription
}

// NewConsumer creates subscriptions for sale and return events.
// Returns a cleanup function that unsubscribes.
func NewConsumer(nc *nats.Conn, uc *BillingUsecase, logger log.Logger) (*Consumer, func(), error) {
	l := log.NewHelper(logger)
	c := &Consumer{log: l, uc: uc}

	if nc == nil {
		l.Info("NATS not configured, skipping consumer subscriptions")
		return c, func() {}, nil
	}

	saleSub, err := nc.Subscribe("sales.sale.completed", c.handleSale)
	if err != nil {
		return nil, nil, err
	}

	returnSub, err := nc.Subscribe("sales.return.completed", c.handleReturn)
	if err != nil {
		saleSub.Unsubscribe()
		return nil, nil, err
	}

	c.subs = []*nats.Subscription{saleSub, returnSub}
	l.Info("Subscribed to sales.sale.completed and sales.return.completed")

	cleanup := func() {
		for _, sub := range c.subs {
			sub.Unsubscribe()
		}
	}

	return c, cleanup, nil
}

func (c *Consumer) handleSale(msg *nats.Msg) {
	var event messages.SaleCompletedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		c.log.Errorf("failed to unmarshal sale completed event: %v", err)
		return
	}

	if err := c.uc.HandleSaleCompleted(context.Background(), event); err != nil {
		c.log.Errorf("failed to handle sale completed: %v", err)
	}
}

func (c *Consumer) handleReturn(msg *nats.Msg) {
	var event messages.ReturnCompletedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		c.log.Errorf("failed to unmarshal return completed event: %v", err)
		return
	}

	if err := c.uc.HandleReturnCompleted(context.Background(), event); err != nil {
		c.log.Errorf("failed to handle return completed: %v", err)
	}
}
