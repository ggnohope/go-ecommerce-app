package dto

type PlaceOrderInput struct {
	ShippingAddress string `json:"shipping_address"`
}

type CreatePaymentLinkInput struct {
	OrderID uint `json:"order_id"`
}
