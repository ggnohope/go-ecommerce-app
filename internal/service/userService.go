package service

import (
	"errors"
	"fmt"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/helper"
	"go-ecommerce-app/internal/repository"
	"go-ecommerce-app/pkg/notification"
	"log"
	"time"

	"gorm.io/gorm"
)

type UserService interface {
	// Public
	SignUp(input dto.UserSignUp) (string, error)
	Login(email string, password string) (string, error)

	// Verification
	GetVerificationCode(userId uint) (string, error)
	Verify(userID uint, code string) error

	// Profile
	CreateProfile(input any) error
	GetProfile(userID string) (domain.User, error)

	// Cart
	AddToCart(userID string, productID string, quantity int) error
	GetCart(userID string) ([]any, error)

	// Orders
	GetOrders(userID string) ([]any, error)
	GetOrder(orderID string) (any, error)

	// Seller
	BecomeSeller(userID string) error
}

type userService struct {
	repo         repository.UserRepository
	auth         *helper.Auth
	notification notification.NotificationClient
}

func NewUserService(db *gorm.DB, auth *helper.Auth, client notification.NotificationClient) UserService {
	newRepo := repository.NewUserRepository(db)
	return &userService{newRepo, auth, client}
}

// Public

func (u *userService) SignUp(input dto.UserSignUp) (string, error) {
	// Check existing user
	existingUser, _ := u.repo.FindUser(input.Email)
	if existingUser != nil {
		return "", errors.New("email already exists")
	}
	// Hash password
	hashedPassword, err := u.auth.CreateHashedPassword(input.Password)
	if err != nil {
		return "", err
	}
	// Create user
	user, err := u.repo.CreateUser(domain.User{
		Email:    input.Email,
		Password: hashedPassword,
		Phone:    input.Phone,
		UserType: "buyer",
	})
	if err != nil {
		return "", err
	}
	log.Printf("user created: %v", user.ID)

	return u.auth.GenerateToken(
		user.ID,
		user.Email,
		user.UserType,
	)
}

func (u *userService) Login(email string, password string) (string, error) {
	user, err := u.repo.FindUser(email)
	if err != nil {
		return "", errors.New("user does not exist with the provided email")
	}

	err = u.auth.VerifyPassword(password, user.Password)
	if err != nil {
		return "", errors.New("invalid password")
	}

	return u.auth.GenerateToken(user.ID, user.Email, user.UserType)
}

// Verification

func (u *userService) GetVerificationCode(userId uint) (string, error) {
	user, err := u.repo.FindUserById(userId)
	if err != nil {
		log.Printf("verification: user_not_found user=%d err=%s", userId, err.Error())
		return "", errors.New("user not found")
	}
	if user.Verified == true {
		log.Printf("verification: user_already_verified user=%d", userId)
		return "", errors.New("user is already verified")
	}

	// Validate user has at least one contact method
	if err := helper.ValidateEmail(user.Email); err != nil {
		log.Printf("verification: invalid_email user=%d email=%s err=%s", userId, user.Email, err.Error())
		return "", notification.NewValidationError(fmt.Sprintf("User email is invalid: %s", err.Error()))
	}

	if err := helper.ValidatePhoneNumber(user.Phone); err != nil {
		log.Printf("verification: invalid_phone user=%d phone=%s err=%s", userId, user.Phone, err.Error())
		return "", notification.NewValidationError(fmt.Sprintf("User phone number is invalid: %s", err.Error()))
	}

	code, err := u.auth.GenerateCode()
	if err != nil {
		log.Printf("verification: code_generation_error user=%d err=%s", userId, err.Error())
		return "", err
	}

	// Try to send notification (SMS primary, email backup)
	smsMessage := fmt.Sprintf("Verification Code: %s (Valid for 30 minutes)", code)

	var lastError error
	var successCount int

	// Attempt SMS
	log.Printf("verification: sms_send_attempt user=%d phone=%s", userId, user.Phone)
	smsErr := u.notification.SendSMS(user.Phone, smsMessage)
	if smsErr != nil {
		log.Printf("verification: sms_send_failed user=%d phone=%s err=%s", userId, user.Phone, smsErr.Error())
		lastError = smsErr
	} else {
		successCount++
		log.Printf("verification: sms_send_success user=%d phone=%s", userId, user.Phone)
	}

	// If SMS failed, try email as backup
	var emailErr error
	if smsErr != nil {
		emailSubject := "Your Verification Code"
		emailBody := fmt.Sprintf("Your verification code is: %s (Valid for 30 minutes)", code)

		log.Printf("verification: email_send_attempt user=%d email=%s (sms failed)", userId, user.Email)
		emailErr = u.notification.SendEmail(user.Email, emailSubject, emailBody)
		if emailErr != nil {
			log.Printf("verification: email_send_failed user=%d email=%s err=%s", userId, user.Email, emailErr.Error())
			lastError = emailErr
		} else {
			successCount++
			log.Printf("verification: email_send_success user=%d email=%s", userId, user.Email)
		}
	}

	// Both SMS and Email failed - return error and do NOT persist code
	if successCount == 0 {
		if notification.IsValidationError(lastError) {
			log.Printf("verification: all_methods_validation_failed user=%d", userId)
			return "", lastError
		}
		log.Printf("verification: all_delivery_methods_failed user=%d smsErr=%v emailErr=%v", userId, smsErr, emailErr)
		return "", fmt.Errorf("verification: failed to send notification via all methods")
	}

	// At least one method succeeded - persist code
	log.Printf("verification: persisting_code user=%d successCount=%d", userId, successCount)
	_, err = u.repo.UpdateUser(userId, domain.User{
		Code:   code,
		Expiry: time.Now().Add(time.Minute * 30),
	})
	if err != nil {
		// Code was sent but failed to persist - log this critical issue
		log.Printf("verification: critical_persist_error user=%d code was sent but db update failed err=%s", userId, err.Error())
		return "", fmt.Errorf("verification: code sent but failed to save")
	}

	log.Printf("verification: code_generated_successfully user=%d", userId)
	return code, nil
}

func (u *userService) isVerified(ID uint) bool {
	user, err := u.repo.FindUserById(ID)
	if err != nil {
		return false
	}

	return user.Verified == true
}

func (u *userService) Verify(userId uint, code string) error {
	if u.isVerified(userId) {
		return errors.New("user is already verified")
	}

	user, err := u.repo.FindUserById(userId)
	if err != nil {
		return err
	}
	if user.Code != code {
		return errors.New("verification code incorrect")
	}
	if time.Now().After(user.Expiry) {
		return errors.New("verification code expired")
	}

	_, err = u.repo.UpdateUser(userId, domain.User{
		Verified: true,
	})
	if err != nil {
		return err
	}

	return nil
}

// Profile

func (u *userService) CreateProfile(input any) error {
	return errors.New("not implemented")
}

func (u *userService) GetProfile(userID string) (domain.User, error) {
	return domain.User{}, errors.New("not implemented")
}

// Cart

func (u *userService) AddToCart(userID string, productID string, quantity int) error {
	return errors.New("not implemented")
}

func (u *userService) GetCart(userID string) ([]any, error) {
	return nil, errors.New("not implemented")
}

// Orders

func (u *userService) GetOrders(userID string) ([]any, error) {
	return nil, errors.New("not implemented")
}

func (u *userService) GetOrder(orderID string) (any, error) {
	return nil, errors.New("not implemented")
}

// Seller

func (u *userService) BecomeSeller(userID string) error {
	return errors.New("not implemented")
}
