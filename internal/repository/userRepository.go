package repository

import (
	"errors"
	"go-ecommerce-app/internal/domain"
	"log"

	"gorm.io/gorm"
)

type UserRepository interface {
	CreateUser(user domain.User) (*domain.User, error)
	FindUser(email string) (*domain.User, error)
	FindUserById(id uint) (*domain.User, error)
	UpdateUser(id uint, user domain.User) (*domain.User, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db}
}

func (u *userRepository) CreateUser(user domain.User) (*domain.User, error) {
	err := u.db.Create(&user).Error

	if err != nil {
		log.Printf(`Create user error: %v`, err)
		return nil, err
	}

	return &user, nil
}

func (u *userRepository) FindUser(email string) (*domain.User, error) {
	var user domain.User
	err := u.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("User not found with email: %s", email)
			return nil, err
		}
		log.Printf("Find user error: %v", err)
		return nil, err
	}
	return &user, nil
}

func (u *userRepository) FindUserById(id uint) (*domain.User, error) {
	var user domain.User
	err := u.db.First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("User not found with id: %d", id)
			return nil, err
		}
		log.Printf("Find user by id error: %v", err)
		return nil, err
	}
	return &user, nil
}

func (u *userRepository) UpdateUser(id uint, user domain.User) (*domain.User, error) {
	var existingUser domain.User

	err := u.db.First(&existingUser, id).Error
	if err != nil {
		log.Printf("Find user before update error: %v", err)
		return nil, err
	}

	err = u.db.Model(&existingUser).Updates(user).Error
	if err != nil {
		log.Printf("Update user error: %v", err)
		return nil, err
	}

	err = u.db.First(&existingUser, id).Error
	if err != nil {
		log.Printf("Fetch updated user error: %v", err)
		return nil, err
	}

	return &existingUser, nil
}
