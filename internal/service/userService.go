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
	SignUp(input dto.UserSignUp) (string, error)
	Login(email string, password string) (string, error)
	GetVerificationCode(userId uint) (string, error)
	Verify(userID uint, code string) error
	CreateProfile(userID uint, input dto.UserProfile) error
	GetProfile(userID uint) (*domain.User, error)
	BecomeSeller(userID uint) error
}

type userService struct {
	repo         repository.UserRepository
	auth         *helper.Auth
	notification notification.NotificationClient
}

func NewUserService(db *gorm.DB, auth *helper.Auth, client notification.NotificationClient) UserService {
	return &userService{repository.NewUserRepository(db), auth, client}
}

func (u *userService) SignUp(input dto.UserSignUp) (string, error) {
	existingUser, _ := u.repo.FindUser(input.Email)
	if existingUser != nil {
		return "", errors.New("email already exists")
	}
	hashedPassword, err := u.auth.CreateHashedPassword(input.Password)
	if err != nil {
		return "", err
	}
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
	return u.auth.GenerateToken(user.ID, user.Email, user.UserType)
}

func (u *userService) Login(email string, password string) (string, error) {
	user, err := u.repo.FindUser(email)
	if err != nil {
		return "", errors.New("user does not exist with the provided email")
	}
	if err = u.auth.VerifyPassword(password, user.Password); err != nil {
		return "", errors.New("invalid password")
	}
	return u.auth.GenerateToken(user.ID, user.Email, user.UserType)
}

func (u *userService) GetVerificationCode(userId uint) (string, error) {
	user, err := u.repo.FindUserById(userId)
	if err != nil {
		log.Printf("verification: user_not_found user=%d err=%s", userId, err.Error())
		return "", errors.New("user not found")
	}
	if user.Verified {
		return "", errors.New("user is already verified")
	}

	if err := helper.ValidateEmail(user.Email); err != nil {
		return "", notification.NewValidationError(fmt.Sprintf("user email is invalid: %s", err.Error()))
	}
	if err := helper.ValidatePhoneNumber(user.Phone); err != nil {
		return "", notification.NewValidationError(fmt.Sprintf("user phone number is invalid: %s", err.Error()))
	}

	code, err := u.auth.GenerateCode()
	if err != nil {
		return "", err
	}

	smsMessage := fmt.Sprintf("Verification Code: %s (Valid for 30 minutes)", code)
	var lastError error
	var successCount int

	smsErr := u.notification.SendSMS(user.Phone, smsMessage)
	if smsErr != nil {
		log.Printf("verification: sms_send_failed user=%d err=%s", userId, smsErr.Error())
		lastError = smsErr
	} else {
		successCount++
	}

	if smsErr != nil {
		emailBody := fmt.Sprintf("Your verification code is: %s (Valid for 30 minutes)", code)
		emailErr := u.notification.SendEmail(user.Email, "Your Verification Code", emailBody)
		if emailErr != nil {
			log.Printf("verification: email_send_failed user=%d err=%s", userId, emailErr.Error())
			lastError = emailErr
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		if notification.IsValidationError(lastError) {
			return "", lastError
		}
		return "", fmt.Errorf("verification: failed to send notification via all methods")
	}

	_, err = u.repo.UpdateUser(userId, domain.User{
		Code:   code,
		Expiry: time.Now().Add(time.Minute * 30),
	})
	if err != nil {
		log.Printf("verification: critical_persist_error user=%d err=%s", userId, err.Error())
		return "", fmt.Errorf("verification: code sent but failed to save")
	}

	return code, nil
}

func (u *userService) isVerified(ID uint) bool {
	user, err := u.repo.FindUserById(ID)
	if err != nil {
		return false
	}
	return user.Verified
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
	_, err = u.repo.UpdateUser(userId, domain.User{Verified: true})
	return err
}

func (u *userService) CreateProfile(userID uint, input dto.UserProfile) error {
	_, err := u.repo.UpdateUser(userID, domain.User{
		FirstName: input.FirstName,
		LastName:  input.LastName,
	})
	return err
}

func (u *userService) GetProfile(userID uint) (*domain.User, error) {
	return u.repo.FindUserById(userID)
}

func (u *userService) BecomeSeller(userID uint) error {
	user, err := u.repo.FindUserById(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if user.UserType == "seller" {
		return errors.New("user is already a seller")
	}
	if !user.Verified {
		return errors.New("account must be verified before becoming a seller")
	}
	if _, err = u.repo.UpdateUser(userID, domain.User{UserType: "seller"}); err != nil {
		return err
	}
	body := "Congratulations! Your seller account has been activated. You can now list products on our platform."
	if notifyErr := u.notification.SendEmail(user.Email, "Seller Account Activated", body); notifyErr != nil {
		log.Printf("seller notification failed: user=%d err=%v", userID, notifyErr)
	}
	return nil
}
