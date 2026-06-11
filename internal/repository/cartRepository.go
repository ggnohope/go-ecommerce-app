package repository

import (
	"go-ecommerce-app/internal/domain"

	"gorm.io/gorm"
)

type CartRepository interface {
	FindOrCreateCart(userID uint) (*domain.Cart, error)
	GetCart(userID uint) (*domain.Cart, error)
	AddItem(item domain.CartItem) (*domain.CartItem, error)
	UpdateItemQuantity(itemID uint, quantity int) error
	FindCartItem(cartID, productID uint) (*domain.CartItem, error)
	RemoveItemFromCart(cartID, itemID uint) error
	ClearCart(cartID uint) error
}

type cartRepository struct {
	db *gorm.DB
}

func NewCartRepository(db *gorm.DB) CartRepository {
	return &cartRepository{db}
}

func (r *cartRepository) FindOrCreateCart(userID uint) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.Where(domain.Cart{UserID: userID}).FirstOrCreate(&cart).Error
	return &cart, err
}

func (r *cartRepository) GetCart(userID uint) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.
		Preload("Items.Product.Images").
		Preload("Items.Product.Category").
		Where("user_id = ?", userID).
		First(&cart).Error
	return &cart, err
}

func (r *cartRepository) AddItem(item domain.CartItem) (*domain.CartItem, error) {
	if err := r.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *cartRepository) UpdateItemQuantity(itemID uint, quantity int) error {
	return r.db.Model(&domain.CartItem{}).Where("id = ?", itemID).Update("quantity", quantity).Error
}

func (r *cartRepository) FindCartItem(cartID, productID uint) (*domain.CartItem, error) {
	var item domain.CartItem
	err := r.db.Where("cart_id = ? AND product_id = ?", cartID, productID).First(&item).Error
	return &item, err
}

func (r *cartRepository) RemoveItemFromCart(cartID, itemID uint) error {
	return r.db.Where("id = ? AND cart_id = ?", itemID, cartID).Delete(&domain.CartItem{}).Error
}

func (r *cartRepository) ClearCart(cartID uint) error {
	return r.db.Where("cart_id = ?", cartID).Delete(&domain.CartItem{}).Error
}
