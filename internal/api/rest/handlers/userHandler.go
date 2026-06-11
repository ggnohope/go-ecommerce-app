package handlers

import (
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"

	"github.com/gofiber/fiber/v2"
)

type UserHandler struct {
	svc      service.UserService
	cartSvc  service.CartService
	orderSvc service.OrderService
}

// ====================
// Public handlers
// ====================

func (h *UserHandler) Register(ctx *fiber.Ctx) error {
	var user dto.UserSignUp
	if err := ctx.BodyParser(&user); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid credentials"})
	}
	if user.Email == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "email is required"})
	}
	if user.Phone == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "phone is required"})
	}
	if len(user.Password) < 6 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "password must be at least 6 characters"})
	}
	token, err := h.svc.SignUp(user)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": token})
}

func (h *UserHandler) Login(ctx *fiber.Ctx) error {
	var input dto.UserLogin
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid credentials"})
	}
	token, err := h.svc.Login(input.Email, input.Password)
	if err != nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": token})
}

// ====================
// Protected handlers
// ====================

func (h *UserHandler) GetVerificationCode(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "invalid user context"})
	}
	code, err := h.svc.GetVerificationCode(user.ID)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": code})
}

func (h *UserHandler) Verify(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "invalid user context"})
	}
	var input dto.VerifyUser
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid verification code"})
	}
	if err := h.svc.Verify(user.ID, input.Code); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"message": "verified successfully"})
}

func (h *UserHandler) CreateProfile(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	var input dto.UserProfile
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}
	if err := h.svc.CreateProfile(user.ID, input); err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"message": "profile updated"})
}

func (h *UserHandler) GetProfile(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	profile, err := h.svc.GetProfile(user.ID)
	if err != nil {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "profile not found"})
	}
	profile.Password = ""
	profile.Code = ""
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": profile})
}

func (h *UserHandler) BecomeSeller(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	if err := h.svc.BecomeSeller(user.ID); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"message": "seller account activated"})
}

// Cart

func (h *UserHandler) AddToCart(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	var input dto.AddToCartInput
	if err := ctx.BodyParser(&input); err != nil || input.ProductID == 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "product_id is required"})
	}
	cart, err := h.cartSvc.AddToCart(user.ID, input)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": cart})
}

func (h *UserHandler) GetCart(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	cart, err := h.cartSvc.GetCart(user.ID)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": cart})
}

func (h *UserHandler) RemoveFromCart(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	itemID, err := ctx.ParamsInt("item_id")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid item id"})
	}
	if err = h.cartSvc.RemoveFromCart(user.ID, uint(itemID)); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"message": "item removed from cart"})
}

// Orders

func (h *UserHandler) PlaceOrder(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	var input dto.PlaceOrderInput
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}
	order, err := h.orderSvc.PlaceOrder(user.ID, input)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusCreated).JSON(fiber.Map{"data": order})
}

func (h *UserHandler) GetOrders(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	orders, err := h.orderSvc.GetOrders(user.ID)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": orders})
}

func (h *UserHandler) GetOrder(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid order id"})
	}
	order, err := h.orderSvc.GetOrder(uint(id), user.ID)
	if err != nil {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"data": order})
}

func SetupUserRoutes(restHandler *rest.RestHandler) {
	userSvc := service.NewUserService(restHandler.DB, restHandler.Auth, restHandler.NotificationClient)
	cartSvc := service.NewCartService(restHandler.DB)
	orderSvc := service.NewOrderService(restHandler.DB, restHandler.SQSClient, restHandler.StripeClient)

	h := UserHandler{
		svc:      userSvc,
		cartSvc:  cartSvc,
		orderSvc: orderSvc,
	}

	pubRoutes := restHandler.App.Group("/user")
	privateRoutes := pubRoutes.Group("/me", restHandler.Auth.Authorize)

	pubRoutes.Post("/register", h.Register)
	pubRoutes.Post("/login", h.Login)

	privateRoutes.Get("/verify", h.GetVerificationCode)
	privateRoutes.Post("/verify", h.Verify)
	privateRoutes.Get("/profile", h.GetProfile)
	privateRoutes.Post("/profile", h.CreateProfile)
	privateRoutes.Post("/cart", h.AddToCart)
	privateRoutes.Get("/cart", h.GetCart)
	privateRoutes.Delete("/cart/:item_id", h.RemoveFromCart)
	privateRoutes.Post("/order", h.PlaceOrder)
	privateRoutes.Get("/order", h.GetOrders)
	privateRoutes.Get("/order/:id", h.GetOrder)
	privateRoutes.Post("/become-seller", h.BecomeSeller)
}
