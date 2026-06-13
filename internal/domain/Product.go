package domain

import "time"

type Category struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	Name     string    `json:"name" gorm:"unique;not null"`
	ParentID *uint     `json:"parent_id"`
	Parent   *Category `json:"-" gorm:"foreignKey:ParentID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
}

type Product struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null"`
	Description string         `json:"description"`
	Price       float64        `json:"price" gorm:"not null"`
	Stock       int            `json:"stock" gorm:"default:0"`
	CategoryID  uint           `json:"category_id"`
	Category    Category       `json:"category" gorm:"foreignKey:CategoryID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SellerID    uint           `json:"seller_id" gorm:"not null;index"`
	Seller      User           `json:"-" gorm:"foreignKey:SellerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Images      []ProductImage `json:"images" gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Status      string         `json:"status" gorm:"default:active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type ProductImage struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	ProductID uint   `json:"product_id" gorm:"not null;index"`
	URL       string `json:"url" gorm:"not null"`
	Position  int    `json:"position" gorm:"default:0"`
}
