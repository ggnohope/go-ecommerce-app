package handlers

import (
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"
	"go-ecommerce-app/pkg/notification"

	"github.com/gofiber/fiber/v2"
)

type UserHandler struct {
	service service.UserService
}

// ====================
// Public handlers
// ====================

func (userHandler *UserHandler) Register(context *fiber.Ctx) error {

	user := dto.UserSignUp{}
	err := context.BodyParser(&user)

	if err != nil {
		return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid credentials",
		})
	}

	// Validate email is provided
	if user.Email == "" {
		return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Email is required",
		})
	}

	// Validate phone is provided
	if user.Phone == "" {
		return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Phone is required",
		})
	}

	// Validate password length
	if len(user.Password) < 6 {
		return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Password must be at least 6 characters",
		})
	}

	token, err := userHandler.service.SignUp(user)

	if err != nil {
		return context.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": token,
	})
}

func (userHandler *UserHandler) Login(context *fiber.Ctx) error {
	loginInput := dto.UserLogin{}
	err := context.BodyParser(&loginInput)
	if err != nil {
		return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid credentials",
		})
	}

	token, err := userHandler.service.Login(loginInput.Email, loginInput.Password)
	if err != nil {
		return context.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": token,
	})
}

// ====================
// Private handlers
// ====================

func (userHandler *UserHandler) GetVerificationCode(context *fiber.Ctx) error {
	user, ok := context.Locals("user").(domain.User)
	if !ok {
		return context.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "invalid user context",
		})
	}
	code, err := userHandler.service.GetVerificationCode(user.ID)
	if err != nil {
		// Return 400 for validation errors (invalid phone/email)
		if notification.IsValidationError(err) {
			return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": err.Error(),
			})
		}
		// Return 500 for server/delivery errors
		return context.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": code,
	})
}

func (userHandler *UserHandler) Verify(context *fiber.Ctx) error {
	user, ok := context.Locals("user").(domain.User)
	if !ok {
		return context.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "invalid user context",
		})
	}

	verifyInput := dto.VerifyUser{}
	err := context.BodyParser(&verifyInput)
	if err != nil {
		return context.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid verification code",
		})
	}

	err = userHandler.service.Verify(user.ID, verifyInput.Code)
	if err != nil {
		return context.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "verified successfully",
	})
}

func (userHandler *UserHandler) CreateProfile(context *fiber.Ctx) error {
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "profile created",
	})
}

func (userHandler *UserHandler) GetProfile(context *fiber.Ctx) error {
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"name": "Hoa",
		"age":  22,
	})
}

func (userHandler *UserHandler) AddToCart(context *fiber.Ctx) error {
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "added to cart",
	})
}

func (userHandler *UserHandler) GetCart(context *fiber.Ctx) error {
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"items": []string{
			"iphone",
			"macbook",
		},
	})
}

func (userHandler *UserHandler) GetOrders(context *fiber.Ctx) error {
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "all orders",
	})
}

func (userHandler *UserHandler) GetOrder(context *fiber.Ctx) error {
	id := context.Params("id")
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"orderId": id,
	})
}

func (userHandler *UserHandler) BecomeSeller(context *fiber.Ctx) error {
	return context.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "become seller success",
	})
}

func SetupUserRoutes(restHandler *rest.RestHandler) {
	app := restHandler.App

	userService := service.NewUserService(restHandler.DB, restHandler.Auth, restHandler.NotificationClient)
	handler := UserHandler{
		service: userService,
	}

	pubRoutes := app.Group("/user")
	privateRoutes := pubRoutes.Group("/me", restHandler.Auth.Authorize)

	pubRoutes.Post("/register", handler.Register)
	pubRoutes.Post("/login", handler.Login)

	privateRoutes.Get("/verify", handler.GetVerificationCode)
	privateRoutes.Post("/verify", handler.Verify)
	privateRoutes.Post("/profile", handler.CreateProfile)
	privateRoutes.Get("/profile", handler.GetProfile)
	privateRoutes.Post("/cart", handler.AddToCart)
	privateRoutes.Get("/cart", handler.GetCart)
	privateRoutes.Get("/order", handler.GetOrders)
	privateRoutes.Get("/order/:id", handler.GetOrder)
	privateRoutes.Post("/become-seller", handler.BecomeSeller)
}
