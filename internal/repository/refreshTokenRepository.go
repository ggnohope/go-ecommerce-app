package repository

import (
	"go-ecommerce-app/internal/domain"
	"time"

	"gorm.io/gorm"
)

type RefreshTokenRepository interface {
	Create(token domain.RefreshToken) (*domain.RefreshToken, error)
	FindByHash(hash string) (*domain.RefreshToken, error)
	Revoke(id uint) error
	RevokeAllForUser(userID uint) error
}

type refreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) RefreshTokenRepository {
	return &refreshTokenRepository{db}
}

func (r *refreshTokenRepository) Create(token domain.RefreshToken) (*domain.RefreshToken, error) {
	if err := r.db.Create(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *refreshTokenRepository) FindByHash(hash string) (*domain.RefreshToken, error) {
	var token domain.RefreshToken
	err := r.db.Where("token_hash = ?", hash).First(&token).Error
	return &token, err
}

func (r *refreshTokenRepository) Revoke(id uint) error {
	now := time.Now()
	return r.db.Model(&domain.RefreshToken{}).
		Where("id = ?", id).
		Update("revoked_at", now).Error
}

func (r *refreshTokenRepository) RevokeAllForUser(userID uint) error {
	now := time.Now()
	return r.db.Model(&domain.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}
