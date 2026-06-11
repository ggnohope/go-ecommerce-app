package helper

import (
	"encoding/hex"
	"errors"
	"fmt"
	"go-ecommerce-app/internal/domain"
	"log/slog"
	"strings"
	"time"

	"crypto/rand"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	AccessTokenExpiry  = 15 * time.Minute
	RefreshTokenExpiry = 7 * 24 * time.Hour
)

type Auth struct {
	Secret string
}

func NewAuth(secret string) *Auth {
	return &Auth{secret}
}

func (a *Auth) CreateHashedPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", errors.New("password must be at least 6 characters")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.New("error hashing password")
	}
	return string(hashed), nil
}

func (a *Auth) VerifyPassword(password, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// GenerateAccessToken issues a short-lived JWT (15 min) used to authenticate requests.
func (a *Auth) GenerateAccessToken(id uint, email, role string) (string, error) {
	if id == 0 || email == "" || role == "" {
		return "", errors.New("id, email, and role are required")
	}
	claims := jwt.MapClaims{
		"id":    id,
		"email": email,
		"role":  role,
		"exp":   time.Now().Add(AccessTokenExpiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(a.Secret))
	if err != nil {
		return "", errors.New("error signing token")
	}
	return signed, nil
}

// GenerateRefreshToken returns a 32-byte cryptographically random hex string (64 chars).
// The caller is responsible for hashing it (via helper.HashToken) before storing.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// VerifyAccessToken parses and validates a Bearer JWT, returning the embedded user claims.
func (a *Auth) VerifyAccessToken(t string) (domain.User, error) {
	parts := strings.SplitN(t, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return domain.User{}, errors.New("invalid authorization header format")
	}

	token, err := jwt.Parse(parts[1], func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(a.Secret), nil
	})
	if err != nil {
		return domain.User{}, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return domain.User{}, errors.New("token verification failed")
	}
	if float64(time.Now().Unix()) > claims["exp"].(float64) {
		return domain.User{}, errors.New("token is expired")
	}

	return domain.User{
		ID:       uint(claims["id"].(float64)),
		Email:    claims["email"].(string),
		UserType: claims["role"].(string),
	}, nil
}

// Authorize is the JWT middleware for protected routes.
func (a *Auth) Authorize(ctx *fiber.Ctx) error {
	authHeader := ctx.Get("Authorization")
	if authHeader == "" {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "authorization header is required",
		})
	}
	user, err := a.VerifyAccessToken(authHeader)
	if err != nil {
		slog.Warn("unauthorized request", "path", ctx.Path(), "err", err.Error())
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}
	ctx.Locals("user", user)
	return ctx.Next()
}

// SellerOnly rejects non-seller users with 403.
func (a *Auth) SellerOnly(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok || user.UserType != "seller" {
		return ctx.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "seller access required",
		})
	}
	return ctx.Next()
}

func (a *Auth) GetCurrentUser(ctx *fiber.Ctx) domain.User {
	return ctx.Locals("user").(domain.User)
}

func (a *Auth) GenerateCode() (string, error) {
	return RandomNumber(6)
}
