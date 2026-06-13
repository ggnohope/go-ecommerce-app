package service

import (
	"errors"
	"fmt"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/helper"
	"go-ecommerce-app/internal/repository"
	"go-ecommerce-app/pkg/notification"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

type UserService interface {
	// Auth
	SignUp(input dto.UserSignUp, userAgent, ip string) (dto.TokenPair, error)
	Login(email, password, userAgent, ip string) (dto.TokenPair, error)
	Refresh(rawToken, userAgent, ip string) (dto.TokenPair, error)
	Logout(rawToken string) error
	LogoutAll(userID uint) error

	// Verification
	GetVerificationCode(userID uint) (string, error)
	Verify(userID uint, code string) error

	// Profile
	CreateProfile(userID uint, input dto.UserProfile) error
	GetProfile(userID uint) (*domain.User, error)
	BecomeSeller(userID uint, userAgent, ip string) (dto.TokenPair, error)

	// Addresses
	AddAddress(userID uint, input dto.AddressInput) (*domain.Address, error)
	GetAddresses(userID uint) ([]domain.Address, error)
	UpdateAddress(userID uint, addressID uint, input dto.AddressInput) (*domain.Address, error)
	DeleteAddress(userID uint, addressID uint) error
}

type userService struct {
	repo             repository.UserRepository
	refreshTokenRepo repository.RefreshTokenRepository
	addressRepo      repository.AddressRepository
	auth             *helper.Auth
	notification     notification.NotificationClient
}

func NewUserService(db *gorm.DB, auth *helper.Auth, client notification.NotificationClient) UserService {
	return &userService{
		repo:             repository.NewUserRepository(db),
		refreshTokenRepo: repository.NewRefreshTokenRepository(db),
		addressRepo:      repository.NewAddressRepository(db),
		auth:             auth,
		notification:     client,
	}
}

// ── Auth ─────────────────────────────────────────────────────────────────────

func (u *userService) SignUp(input dto.UserSignUp, userAgent, ip string) (dto.TokenPair, error) {
	if existing, _ := u.repo.FindUser(input.Email); existing != nil {
		return dto.TokenPair{}, errors.New("email already registered")
	}
	hashed, err := u.auth.CreateHashedPassword(input.Password)
	if err != nil {
		return dto.TokenPair{}, err
	}
	user, err := u.repo.CreateUser(domain.User{
		Email:    input.Email,
		Password: hashed,
		Phone:    input.Phone,
		UserType: "buyer",
	})
	if err != nil {
		return dto.TokenPair{}, err
	}
	slog.Info("user registered", "user_id", user.ID, "email", user.Email)
	return u.issueTokenPair(user, userAgent, ip)
}

func (u *userService) Login(email, password, userAgent, ip string) (dto.TokenPair, error) {
	user, err := u.repo.FindUser(email)
	if err != nil {
		return dto.TokenPair{}, errors.New("invalid email or password")
	}
	if err = u.auth.VerifyPassword(password, user.Password); err != nil {
		return dto.TokenPair{}, errors.New("invalid email or password")
	}
	slog.Info("user logged in", "user_id", user.ID)
	return u.issueTokenPair(user, userAgent, ip)
}

// Refresh rotates the refresh token and issues a fresh access token.
// If a revoked token is presented, all sessions for that user are invalidated
// (refresh-token theft detection).
func (u *userService) Refresh(rawToken, userAgent, ip string) (dto.TokenPair, error) {
	tokenHash := helper.HashToken(rawToken)

	rt, err := u.refreshTokenRepo.FindByHash(tokenHash)
	if err != nil {
		return dto.TokenPair{}, errors.New("invalid refresh token")
	}

	if rt.RevokedAt != nil {
		slog.Warn("refresh token reuse detected — revoking all sessions",
			"user_id", rt.UserID, "ip", ip)
		_ = u.refreshTokenRepo.RevokeAllForUser(rt.UserID)
		return dto.TokenPair{}, errors.New("refresh token already revoked")
	}

	if time.Now().After(rt.ExpiresAt) {
		return dto.TokenPair{}, errors.New("refresh token expired")
	}

	// Revoke the used token before issuing a new one (rotation).
	if err = u.refreshTokenRepo.Revoke(rt.ID); err != nil {
		return dto.TokenPair{}, err
	}

	user, err := u.repo.FindUserById(rt.UserID)
	if err != nil {
		return dto.TokenPair{}, errors.New("user not found")
	}
	return u.issueTokenPair(user, userAgent, ip)
}

func (u *userService) Logout(rawToken string) error {
	tokenHash := helper.HashToken(rawToken)
	rt, err := u.refreshTokenRepo.FindByHash(tokenHash)
	if err != nil {
		return errors.New("invalid refresh token")
	}
	return u.refreshTokenRepo.Revoke(rt.ID)
}

func (u *userService) LogoutAll(userID uint) error {
	return u.refreshTokenRepo.RevokeAllForUser(userID)
}

// issueTokenPair generates an access + refresh token pair and persists the refresh token.
func (u *userService) issueTokenPair(user *domain.User, userAgent, ip string) (dto.TokenPair, error) {
	accessToken, err := u.auth.GenerateAccessToken(user.ID, user.Email, user.UserType)
	if err != nil {
		return dto.TokenPair{}, err
	}

	rawRefresh, err := helper.GenerateRefreshToken()
	if err != nil {
		return dto.TokenPair{}, err
	}

	_, err = u.refreshTokenRepo.Create(domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.HashToken(rawRefresh),
		UserAgent: userAgent,
		IPAddress: ip,
		ExpiresAt: time.Now().Add(helper.RefreshTokenExpiry),
	})
	if err != nil {
		return dto.TokenPair{}, err
	}

	return dto.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(helper.AccessTokenExpiry.Seconds()),
	}, nil
}

// ── Verification ──────────────────────────────────────────────────────────────

func (u *userService) GetVerificationCode(userID uint) (string, error) {
	user, err := u.repo.FindUserById(userID)
	if err != nil {
		return "", errors.New("user not found")
	}
	if user.Verified {
		return "", errors.New("user is already verified")
	}
	if err := helper.ValidateEmail(user.Email); err != nil {
		return "", notification.NewValidationError(fmt.Sprintf("user email is invalid: %s", err))
	}
	if err := helper.ValidatePhoneNumber(user.Phone); err != nil {
		return "", notification.NewValidationError(fmt.Sprintf("user phone number is invalid: %s", err))
	}

	code, err := u.auth.GenerateCode()
	if err != nil {
		return "", err
	}

	smsMsg := fmt.Sprintf("Verification Code: %s (Valid 30 minutes)", code)
	var deliveryOK bool

	if smsErr := u.notification.SendSMS(user.Phone, smsMsg); smsErr != nil {
		slog.Warn("sms send failed", "user_id", userID, "err", smsErr)
		emailBody := fmt.Sprintf("Your verification code is: %s (Valid 30 minutes)", code)
		if emailErr := u.notification.SendEmail(user.Email, "Your Verification Code", emailBody); emailErr != nil {
			slog.Error("email send failed", "user_id", userID, "err", emailErr)
			return "", fmt.Errorf("failed to send verification code")
		}
		deliveryOK = true
	} else {
		deliveryOK = true
	}

	if !deliveryOK {
		return "", fmt.Errorf("failed to send verification code")
	}

	_, err = u.repo.UpdateUser(userID, domain.User{
		Code:   code,
		Expiry: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		slog.Error("failed to persist verification code", "user_id", userID, "err", err)
		return "", fmt.Errorf("code sent but failed to save")
	}
	return code, nil
}

func (u *userService) Verify(userID uint, code string) error {
	user, err := u.repo.FindUserById(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if user.Verified {
		return errors.New("user is already verified")
	}
	if user.Code != code {
		return errors.New("verification code incorrect")
	}
	if time.Now().After(user.Expiry) {
		return errors.New("verification code expired")
	}
	_, err = u.repo.UpdateUser(userID, domain.User{Verified: true})
	return err
}

// ── Profile ───────────────────────────────────────────────────────────────────

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

// BecomeSeller upgrades the user and issues a fresh token pair so the new
// "seller" role claim takes effect immediately (the old access token still
// carries "buyer" until it expires).
func (u *userService) BecomeSeller(userID uint, userAgent, ip string) (dto.TokenPair, error) {
	user, err := u.repo.FindUserById(userID)
	if err != nil {
		return dto.TokenPair{}, errors.New("user not found")
	}
	if user.UserType == "seller" {
		return dto.TokenPair{}, errors.New("user is already a seller")
	}
	if !user.Verified {
		return dto.TokenPair{}, errors.New("account must be verified before becoming a seller")
	}
	updated, err := u.repo.UpdateUser(userID, domain.User{UserType: "seller"})
	if err != nil {
		return dto.TokenPair{}, err
	}
	slog.Info("user upgraded to seller", "user_id", userID)
	body := "Congratulations! Your seller account is now active. You can list products on our platform."
	if err := u.notification.SendEmail(user.Email, "Seller Account Activated", body); err != nil {
		slog.Warn("seller activation email failed", "user_id", userID, "err", err)
	}
	return u.issueTokenPair(updated, userAgent, ip)
}

// ── Addresses ─────────────────────────────────────────────────────────────────

func (u *userService) AddAddress(userID uint, input dto.AddressInput) (*domain.Address, error) {
	if input.IsDefault {
		_ = u.addressRepo.UnsetDefault(userID)
	}
	return u.addressRepo.Create(domain.Address{
		UserID:     userID,
		Street:     input.Street,
		City:       input.City,
		State:      input.State,
		Country:    input.Country,
		PostalCode: input.PostalCode,
		IsDefault:  input.IsDefault,
	})
}

func (u *userService) GetAddresses(userID uint) ([]domain.Address, error) {
	return u.addressRepo.FindByUserID(userID)
}

func (u *userService) UpdateAddress(userID uint, addressID uint, input dto.AddressInput) (*domain.Address, error) {
	addr, err := u.addressRepo.FindByID(addressID)
	if err != nil {
		return nil, errors.New("address not found")
	}
	if addr.UserID != userID {
		return nil, errors.New("unauthorized")
	}
	if input.IsDefault {
		_ = u.addressRepo.UnsetDefault(userID)
	}
	return u.addressRepo.Update(addressID, map[string]interface{}{
		"street":      input.Street,
		"city":        input.City,
		"state":       input.State,
		"country":     input.Country,
		"postal_code": input.PostalCode,
		"is_default":  input.IsDefault,
	})
}

func (u *userService) DeleteAddress(userID uint, addressID uint) error {
	addr, err := u.addressRepo.FindByID(addressID)
	if err != nil {
		return errors.New("address not found")
	}
	if addr.UserID != userID {
		return errors.New("unauthorized")
	}
	return u.addressRepo.Delete(addressID)
}
