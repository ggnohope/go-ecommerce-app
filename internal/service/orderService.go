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
	CreatePaymentIntent(orderID uint, userID uint) (*payment.PaymentIntent, error)
	HandleStripeEvent(payload []byte, signature string) error
}

type orderService struct {
	orderRepo    repository.OrderRepository
	cartRepo     repository.CartRepository
	sqsClient    *queue.SQSClient
	stripeClient *payment.StripeClient
}

func NewOrderService(
	db *gorm.DB,
	sqsClient *queue.SQSClient,
	stripeClient *payment.StripeClient,
) OrderService {
	return &orderService{
		orderRepo:    repository.NewOrderRepository(db),
		cartRepo:     repository.NewCartRepository(db),
		sqsClient:    sqsClient,
		stripeClient: stripeClient,
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

func (s *orderService) CreatePaymentIntent(orderID uint, userID uint) (*payment.PaymentIntent, error) {
	if s.stripeClient == nil {
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

	amountCents := int64(math.Round(order.TotalAmount * 100))
	pi, err := s.stripeClient.CreatePaymentIntent(orderID, amountCents)
	if err != nil {
		return nil, err
	}

	if err = s.orderRepo.UpdateOrder(orderID, map[string]interface{}{
		"payment_intent_id": pi.ID,
	}); err != nil {
		log.Printf("order: failed to save payment_intent_id order=%d err=%v", orderID, err)
	}

	return pi, nil
}

func (s *orderService) HandleStripeEvent(payload []byte, signature string) error {
	if s.stripeClient == nil {
		return errors.New("payment service not configured")
	}

	event, err := s.stripeClient.ValidateWebhook(payload, signature)
	if err != nil {
		return err
	}

	switch event.Type {
	case "payment_intent.succeeded":
		orderID, err := payment.ExtractOrderID(event)
		if err != nil {
			log.Printf("stripe webhook: %v", err)
			return nil
		}
		if sqsErr := s.sqsClient.PublishOrderEvent(queue.OrderEvent{
			EventType: queue.EventOrderPaid,
			OrderID:   orderID,
		}); sqsErr != nil {
			log.Printf("stripe webhook: sqs publish failed order=%d err=%v", orderID, sqsErr)
		}
		return s.orderRepo.UpdateOrder(orderID, map[string]interface{}{
			"payment_status": domain.PaymentStatusPaid,
			"status":         domain.OrderStatusConfirmed,
		})

	case "payment_intent.payment_failed":
		orderID, err := payment.ExtractOrderID(event)
		if err != nil {
			return nil
		}
		return s.orderRepo.UpdateOrder(orderID, map[string]interface{}{
			"payment_status": domain.PaymentStatusFailed,
		})
	}

	return nil
}
