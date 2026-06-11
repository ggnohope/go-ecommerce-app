package repository

import (
	"go-ecommerce-app/internal/domain"

	"gorm.io/gorm"
)

type OrderRepository interface {
	CreateOrder(order domain.Order) (*domain.Order, error)
	FindOrderByID(id uint) (*domain.Order, error)
	FindOrdersByUserID(userID uint) ([]domain.Order, error)
	UpdateOrder(id uint, updates map[string]interface{}) error
}

type orderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{db}
}

func (r *orderRepository) CreateOrder(order domain.Order) (*domain.Order, error) {
	if err := r.db.Create(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *orderRepository) FindOrderByID(id uint) (*domain.Order, error) {
	var order domain.Order
	err := r.db.Preload("Items.Product.Images").First(&order, id).Error
	return &order, err
}

func (r *orderRepository) FindOrdersByUserID(userID uint) ([]domain.Order, error) {
	var orders []domain.Order
	err := r.db.Preload("Items.Product").
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&orders).Error
	return orders, err
}

func (r *orderRepository) UpdateOrder(id uint, updates map[string]interface{}) error {
	return r.db.Model(&domain.Order{}).Where("id = ?", id).Updates(updates).Error
}
