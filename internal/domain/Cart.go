package domain

import "time"

type Cart struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	UserID    uint       `json:"user_id" gorm:"not null;uniqueIndex"`
	User      User       `json:"-" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Items     []CartItem `json:"items" gorm:"foreignKey:CartID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type CartItem struct {
	ID        uint    `json:"id" gorm:"primaryKey"`
	CartID    uint    `json:"cart_id" gorm:"not null;index"`
	ProductID uint    `json:"product_id" gorm:"not null"`
	Product   Product `json:"product" gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Quantity  int     `json:"quantity" gorm:"default:1"`
	Price     float64 `json:"price"`
}
