package dto

type PlaceOrderInput struct {
	ShippingAddress string `json:"shipping_address"`
}

type CreatePaymentIntentInput struct {
	OrderID uint `json:"order_id"`
}
