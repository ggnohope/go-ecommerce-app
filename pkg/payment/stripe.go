package payment

import (
	"encoding/json"
	"fmt"

	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/paymentintent"
	"github.com/stripe/stripe-go/v76/webhook"
)

type StripeClient struct {
	webhookSecret string
}

type PaymentIntent struct {
	ID           string `json:"id"`
	ClientSecret string `json:"client_secret"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
}

func NewStripeClient(secretKey, webhookSecret string) *StripeClient {
	stripe.Key = secretKey
	return &StripeClient{webhookSecret: webhookSecret}
}

func (s *StripeClient) CreatePaymentIntent(orderID uint, amountCents int64) (*PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountCents),
		Currency: stripe.String("usd"),
		Metadata: map[string]string{
			"order_id": fmt.Sprintf("%d", orderID),
		},
	}
	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe: payment intent failed: %w", err)
	}
	return &PaymentIntent{
		ID:           pi.ID,
		ClientSecret: pi.ClientSecret,
		Amount:       pi.Amount,
		Currency:     string(pi.Currency),
		Status:       string(pi.Status),
	}, nil
}

func (s *StripeClient) ValidateWebhook(payload []byte, signature string) (stripe.Event, error) {
	event, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		return stripe.Event{}, fmt.Errorf("stripe: webhook validation failed: %w", err)
	}
	return event, nil
}

// ExtractOrderID pulls order_id from a payment intent event's metadata.
func ExtractOrderID(event stripe.Event) (uint, error) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return 0, fmt.Errorf("stripe: failed to unmarshal payment intent: %w", err)
	}
	idStr, ok := pi.Metadata["order_id"]
	if !ok || idStr == "" {
		return 0, fmt.Errorf("stripe: order_id missing from metadata")
	}
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		return 0, fmt.Errorf("stripe: invalid order_id: %w", err)
	}
	return id, nil
}
