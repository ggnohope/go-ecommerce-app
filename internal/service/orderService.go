package service

import (
	"errors"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/repository"
	"go-ecommerce-app/pkg/payment"
	"go-ecommerce-app/pkg/queue"
	"log"
	"math"

	"gorm.io/gorm"
)

type OrderService interface {
	PlaceOrder(userID uint, input dto.PlaceOrderInput) (*domain.Order, error)
	GetOrders(userID uint) ([]domain.Order, error)
	GetOrder(orderID uint, userID uint) (*domain.Order, error)
	CreatePaymentLink(orderID uint, userID uint) (*payment.PaymentLink, error)
	HandlePayOSWebhook(payload []byte) error
}

type orderService struct {
	orderRepo   repository.OrderRepository
	cartRepo    repository.CartRepository
	sqsClient   *queue.SQSClient
	payosClient *payment.PayOSClient
}

func NewOrderService(
	db *gorm.DB,
	sqsClient *queue.SQSClient,
	payosClient *payment.PayOSClient,
) OrderService {
	return &orderService{
		orderRepo:   repository.NewOrderRepository(db),
		cartRepo:    repository.NewCartRepository(db),
		sqsClient:   sqsClient,
		payosClient: payosClient,
	}
}

func (s *orderService) PlaceOrder(userID uint, input dto.PlaceOrderInput) (*domain.Order, error) {
	cart, err := s.cartRepo.GetCart(userID)
	if err != nil || len(cart.Items) == 0 {
		return nil, errors.New("cart is empty")
	}

	var total float64
	orderItems := make([]domain.OrderItem, 0, len(cart.Items))
	for _, item := range cart.Items {
		total += item.Price * float64(item.Quantity)
		orderItems = append(orderItems, domain.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		})
	}

	order, err := s.orderRepo.CreateOrder(domain.Order{
		UserID:          userID,
		Status:          domain.OrderStatusPending,
		TotalAmount:     math.Round(total*100) / 100,
		PaymentStatus:   domain.PaymentStatusPending,
		ShippingAddress: input.ShippingAddress,
		Items:           orderItems,
	})
	if err != nil {
		return nil, err
	}

	if err = s.cartRepo.ClearCart(cart.ID); err != nil {
		log.Printf("order: failed to clear cart after order=%d user=%d err=%v", order.ID, userID, err)
	}

	if s.sqsClient != nil {
		ev := queue.OrderEvent{
			EventType: queue.EventOrderPlaced,
			OrderID:   order.ID,
			UserID:    userID,
			Amount:    order.TotalAmount,
		}
		if err = s.sqsClient.PublishOrderEvent(ev); err != nil {
			log.Printf("order: sqs publish failed order=%d err=%v", order.ID, err)
		}
	}

	return order, nil
}

func (s *orderService) GetOrders(userID uint) ([]domain.Order, error) {
	return s.orderRepo.FindOrdersByUserID(userID)
}

func (s *orderService) GetOrder(orderID uint, userID uint) (*domain.Order, error) {
	order, err := s.orderRepo.FindOrderByID(orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}
	if order.UserID != userID {
		return nil, errors.New("order not found")
	}
	return order, nil
}

func (s *orderService) CreatePaymentLink(orderID uint, userID uint) (*payment.PaymentLink, error) {
	if s.payosClient == nil {
		return nil, errors.New("payment service not configured")
	}

	order, err := s.orderRepo.FindOrderByID(orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}
	if order.UserID != userID {
		return nil, errors.New("order not found")
	}
	if order.PaymentStatus == domain.PaymentStatusPaid {
		return nil, errors.New("order is already paid")
	}

	// TotalAmount is treated as VND (payOS only accepts integer dong).
	amountVND := int64(math.Round(order.TotalAmount))
	link, err := s.payosClient.CreatePaymentLink(orderID, amountVND)
	if err != nil {
		return nil, err
	}

	// Reuse the existing payment_intent_id column to store payOS's link id.
	if err = s.orderRepo.UpdateOrder(orderID, map[string]interface{}{
		"payment_intent_id": link.PaymentLinkID,
	}); err != nil {
		log.Printf("order: failed to save payment_link_id order=%d err=%v", orderID, err)
	}

	return link, nil
}

// HandlePayOSWebhook verifies a payOS webhook and, for a settled payment,
// marks the order paid and publishes ORDER_PAID. The worker owns the
// transition to confirmed (see internal/worker).
func (s *orderService) HandlePayOSWebhook(payload []byte) error {
	if s.payosClient == nil {
		return errors.New("payment service not configured")
	}

	data, err := s.payosClient.VerifyWebhook(payload)
	if err != nil {
		return err
	}

	orderID := uint(data.OrderCode)

	// Ignore webhooks for unknown orders (e.g. payOS's registration test ping)
	// so they don't poison the queue.
	order, err := s.orderRepo.FindOrderByID(orderID)
	if err != nil {
		log.Printf("payos webhook: unknown order=%d, ignoring", orderID)
		return nil
	}

	// Guard against amount mismatch (under/overpayment) before crediting.
	if data.Amount != int64(math.Round(order.TotalAmount)) {
		log.Printf("payos webhook: amount mismatch order=%d got=%d want=%d, ignoring",
			orderID, data.Amount, int64(math.Round(order.TotalAmount)))
		return nil
	}

	// Persist paid first; only publish once the DB write succeeds, so a failed
	// write makes payOS retry instead of leaving a published-but-unpaid order.
	if err := s.orderRepo.UpdateOrder(orderID, map[string]interface{}{
		"payment_status": domain.PaymentStatusPaid,
	}); err != nil {
		return err
	}

	if s.sqsClient != nil {
		if sqsErr := s.sqsClient.PublishOrderEvent(queue.OrderEvent{
			EventType: queue.EventOrderPaid,
			OrderID:   orderID,
		}); sqsErr != nil {
			log.Printf("payos webhook: sqs publish failed order=%d err=%v", orderID, sqsErr)
		}
	}
	return nil
}
