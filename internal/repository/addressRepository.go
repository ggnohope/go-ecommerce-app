package repository

import (
	"go-ecommerce-app/internal/domain"

	"gorm.io/gorm"
)

type AddressRepository interface {
	Create(address domain.Address) (*domain.Address, error)
	FindByUserID(userID uint) ([]domain.Address, error)
	FindByID(id uint) (*domain.Address, error)
	Update(id uint, updates map[string]interface{}) (*domain.Address, error)
	Delete(id uint) error
	UnsetDefault(userID uint) error
}

type addressRepository struct {
	db *gorm.DB
}

func NewAddressRepository(db *gorm.DB) AddressRepository {
	return &addressRepository{db}
}

func (r *addressRepository) Create(address domain.Address) (*domain.Address, error) {
	if err := r.db.Create(&address).Error; err != nil {
		return nil, err
	}
	return &address, nil
}

func (r *addressRepository) FindByUserID(userID uint) ([]domain.Address, error) {
	var addresses []domain.Address
	err := r.db.Where("user_id = ?", userID).
		Order("is_default desc, created_at desc").
		Find(&addresses).Error
	return addresses, err
}

func (r *addressRepository) FindByID(id uint) (*domain.Address, error) {
	var address domain.Address
	err := r.db.First(&address, id).Error
	return &address, err
}

func (r *addressRepository) Update(id uint, updates map[string]interface{}) (*domain.Address, error) {
	var address domain.Address
	if err := r.db.First(&address, id).Error; err != nil {
		return nil, err
	}
	if err := r.db.Model(&address).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &address, nil
}

func (r *addressRepository) Delete(id uint) error {
	return r.db.Delete(&domain.Address{}, id).Error
}

// UnsetDefault clears the default flag for all of a user's addresses.
// Call before setting a new default to enforce single-default invariant.
func (r *addressRepository) UnsetDefault(userID uint) error {
	return r.db.Model(&domain.Address{}).
		Where("user_id = ? AND is_default = true", userID).
		Update("is_default", false).Error
}
