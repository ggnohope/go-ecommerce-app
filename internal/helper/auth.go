package helper

import (
	"errors"
	"fmt"
	"go-ecommerce-app/internal/domain"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	if err != nil {
		log.Println(err)
		return "", errors.New("error hashing password")
	}

	return string(hashedPassword), nil
}

func (a *Auth) GenerateToken(id uint, email string, role string) (string, error) {

	if len(email) == 0 || len(role) == 0 || id == 0 {
		return "", errors.New("id, email or role required")
	}
	claims := jwt.MapClaims{
		"id":    id,
		"email": email,
		"role":  role,
		"exp":   time.Now().Add(time.Hour * 24).Unix(),
	}
	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)
	tokenString, err := token.SignedString([]byte(a.Secret))
	if err != nil {
		log.Println("Atuh::GenerateToken: ", err)
		return "", errors.New("error signing token")
	}
	return tokenString, nil
}

func (a *Auth) VerifyPassword(password string, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword(
		[]byte(hashedPassword),
		[]byte(password),
	)
}

func (a *Auth) VerifyToken(t string) (domain.User, error) {
	tokenArray := strings.Split(t, " ")

	if len(tokenArray) != 2 {
		return domain.User{}, errors.New("invalid token")
	}

	tokenString := tokenArray[1]

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header)
		}
		return []byte(a.Secret), nil
	})

	if err != nil {
		log.Println(err)
		return domain.User{}, errors.New("invalid signing method")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if float64(time.Now().Unix()) > claims["exp"].(float64) {
			return domain.User{}, errors.New("token is expired")
		}

		user := domain.User{
			ID:       uint(claims["id"].(float64)),
			Email:    claims["email"].(string),
			UserType: claims["role"].(string),
		}

		return user, nil
	}

	return domain.User{}, errors.New("token verification failed")
}

func (a *Auth) Authorize(ctx *fiber.Ctx) error {
	authHeader := ctx.Get("Authorization")

	if authHeader == "" {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "authorization header is required",
		})
	}

	user, err := a.VerifyToken(authHeader)

	if err == nil {
		ctx.Locals("user", user)
		return ctx.Next()
	} else {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "unauthorized",
			"error":   err.Error(),
		})
	}
}

func (a *Auth) GetCurrentUser(ctx *fiber.Ctx) domain.User {
	user := ctx.Locals("user")

	return user.(domain.User)
}

func (a *Auth) GenerateCode() (string, error) {
	return RandomNumber(6)
}
