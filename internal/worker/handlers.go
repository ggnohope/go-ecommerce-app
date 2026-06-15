package worker

import (
	"fmt"
	"log/slog"

	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/pkg/notification"
	"go-ecommerce-app/pkg/queue"
)

// OrderStore is the subset of order persistence the worker needs.
type OrderStore interface {
	FindOrderByID(id uint) (*domain.Order, error)
	UpdateOrder(id uint, updates map[string]interface{}) error
}

// Dispatcher routes a parsed OrderEvent to the right handler.
type Dispatcher struct {
	store    OrderStore
	notifier notification.NotificationClient
}

func NewDispatcher(store OrderStore, notifier notification.NotificationClient) *Dispatcher {
	return &Dispatcher{store: store, notifier: notifier}
}

// Handle returns nil on success (caller deletes the message) and a non-nil
// error to signal "retry later" (caller leaves the message for redelivery).
func (d *Dispatcher) Handle(event queue.OrderEvent) error {
	switch event.EventType {
	case queue.EventOrderPlaced:
		return d.handleOrderPlaced(event)
	case queue.EventOrderPaid:
		return d.handleOrderPaid(event)
	default:
		slog.Warn("worker: unknown event type, skipping", "type", event.EventType, "order", event.OrderID)
		return nil
	}
}

func (d *Dispatcher) handleOrderPlaced(event queue.OrderEvent) error {
	order, err := d.store.FindOrderByID(event.OrderID)
	if err != nil {
		return fmt.Errorf("worker: load order %d: %w", event.OrderID, err)
	}
	body := fmt.Sprintf("Your order #%d for $%.2f has been received.", order.ID, order.TotalAmount)
	if err := d.notifier.SendEmail(order.User.Email, "Order confirmation", body); err != nil {
		return fmt.Errorf("worker: send confirmation email for order %d: %w", order.ID, err)
	}
	slog.Info("worker: order confirmation sent", "order", order.ID)
	return nil
}

func (d *Dispatcher) handleOrderPaid(event queue.OrderEvent) error {
	order, err := d.store.FindOrderByID(event.OrderID)
	if err != nil {
		return fmt.Errorf("worker: load order %d: %w", event.OrderID, err)
	}
	// Idempotency guard: SQS is at-least-once, so this event may arrive twice.
	if order.Status == domain.OrderStatusConfirmed {
		slog.Info("worker: order already confirmed, skipping (idempotent)", "order", order.ID)
		return nil
	}
	if err := d.store.UpdateOrder(order.ID, map[string]interface{}{
		"status": domain.OrderStatusConfirmed,
	}); err != nil {
		return fmt.Errorf("worker: confirm order %d: %w", order.ID, err)
	}
	body := fmt.Sprintf("Payment for order #%d is confirmed. Thank you!", order.ID)
	if err := d.notifier.SendEmail(order.User.Email, "Payment received", body); err != nil {
		return fmt.Errorf("worker: send payment email for order %d: %w", order.ID, err)
	}
	slog.Info("worker: order confirmed and notified", "order", order.ID)
	return nil
}
