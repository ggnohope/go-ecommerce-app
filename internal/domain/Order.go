package domain

import "time"

type OrderStatus string
type PaymentStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

const (
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusPaid     PaymentStatus = "paid"
	PaymentStatusFailed   PaymentStatus = "failed"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

type Address struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	UserID     uint   `json:"user_id" gorm:"not null;index"`
	User       User   `json:"-" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	Country    string `json:"country"`
	PostalCode string `json:"postal_code"`
	IsDefault  bool   `json:"is_default" gorm:"default:false"`
}

type Order struct {
	ID              uint          `json:"id" gorm:"primaryKey"`
	UserID          uint          `json:"user_id" gorm:"not null;index"`
	User            User          `json:"-" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Status          OrderStatus   `json:"status" gorm:"default:pending"`
	TotalAmount     float64       `json:"total_amount"`
	PaymentIntentID string        `json:"payment_intent_id"`
	PaymentStatus   PaymentStatus `json:"payment_status" gorm:"default:pending"`
	ShippingAddress string        `json:"shipping_address"`
	Items           []OrderItem   `json:"items" gorm:"foreignKey:OrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type OrderItem struct {
	ID        uint    `json:"id" gorm:"primaryKey"`
	OrderID   uint    `json:"order_id" gorm:"not null;index"`
	ProductID uint    `json:"product_id" gorm:"not null"`
	Product   Product `json:"product" gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}
