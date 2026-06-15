package worker

import (
	"go-ecommerce-app/internal/domain"

	"gorm.io/gorm"
)

// gormOrderStore implements OrderStore over GORM. It preloads User so handlers
// can read the customer's email for notifications.
type gormOrderStore struct {
	db *gorm.DB
}

func NewGormOrderStore(db *gorm.DB) OrderStore {
	return &gormOrderStore{db: db}
}

func (s *gormOrderStore) FindOrderByID(id uint) (*domain.Order, error) {
	var order domain.Order
	if err := s.db.Preload("User").First(&order, id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (s *gormOrderStore) UpdateOrder(id uint, updates map[string]interface{}) error {
	return s.db.Model(&domain.Order{}).Where("id = ?", id).Updates(updates).Error
}
