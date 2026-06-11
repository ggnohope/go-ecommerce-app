package handlers

import (
	"go-ecommerce-app/internal/api/middleware"
	"go-ecommerce-app/internal/api/response"
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

// ── Public ────────────────────────────────────────────────────────────────────

func (h *UserHandler) Register(ctx *fiber.Ctx) error {
	var input dto.UserSignUp
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	switch {
	case input.Email == "":
		return response.BadRequest(ctx, "email is required")
	case input.Phone == "":
		return response.BadRequest(ctx, "phone is required")
	case len(input.Password) < 6:
		return response.BadRequest(ctx, "password must be at least 6 characters")
	}
	pair, err := h.svc.SignUp(input, ctx.Get("User-Agent"), ctx.IP())
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.Created(ctx, pair)
}

func (h *UserHandler) Login(ctx *fiber.Ctx) error {
	var input dto.UserLogin
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	pair, err := h.svc.Login(input.Email, input.Password, ctx.Get("User-Agent"), ctx.IP())
	if err != nil {
		return response.Unauthorized(ctx, err.Error())
	}
	return response.OK(ctx, pair)
}

// Refresh exchanges a valid refresh token for a new access + refresh token pair.
func (h *UserHandler) Refresh(ctx *fiber.Ctx) error {
	var input dto.RefreshTokenInput
	if err := ctx.BodyParser(&input); err != nil || input.RefreshToken == "" {
		return response.BadRequest(ctx, "refresh_token is required")
	}
	pair, err := h.svc.Refresh(input.RefreshToken, ctx.Get("User-Agent"), ctx.IP())
	if err != nil {
		return response.Unauthorized(ctx, err.Error())
	}
	return response.OK(ctx, pair)
}

// Logout revokes the supplied refresh token.
func (h *UserHandler) Logout(ctx *fiber.Ctx) error {
	var input dto.RefreshTokenInput
	if err := ctx.BodyParser(&input); err != nil || input.RefreshToken == "" {
		return response.BadRequest(ctx, "refresh_token is required")
	}
	if err := h.svc.Logout(input.RefreshToken); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.NoContent(ctx)
}

// ── Protected ─────────────────────────────────────────────────────────────────

// LogoutAll revokes every refresh token for the authenticated user.
func (h *UserHandler) LogoutAll(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	if err := h.svc.LogoutAll(user.ID); err != nil {
		return response.InternalError(ctx)
	}
	return response.NoContent(ctx)
}

func (h *UserHandler) GetVerificationCode(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	if _, err := h.svc.GetVerificationCode(user.ID); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.OK(ctx, fiber.Map{"message": "verification code sent"})
}

func (h *UserHandler) Verify(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.VerifyUser
	if err := ctx.BodyParser(&input); err != nil || input.Code == "" {
		return response.BadRequest(ctx, "code is required")
	}
	if err := h.svc.Verify(user.ID, input.Code); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.OK(ctx, fiber.Map{"message": "account verified successfully"})
}

// ── Profile ───────────────────────────────────────────────────────────────────

func (h *UserHandler) GetProfile(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	profile, err := h.svc.GetProfile(user.ID)
	if err != nil {
		return response.NotFound(ctx, "profile not found")
	}
	profile.Password = ""
	profile.Code = ""
	return response.OK(ctx, profile)
}

func (h *UserHandler) CreateProfile(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.UserProfile
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if err := h.svc.CreateProfile(user.ID, input); err != nil {
		return response.InternalError(ctx)
	}
	return response.OK(ctx, fiber.Map{"message": "profile updated"})
}

func (h *UserHandler) BecomeSeller(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	if err := h.svc.BecomeSeller(user.ID); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.OK(ctx, fiber.Map{"message": "seller account activated"})
}

// ── Addresses ─────────────────────────────────────────────────────────────────

func (h *UserHandler) GetAddresses(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	addresses, err := h.svc.GetAddresses(user.ID)
	if err != nil {
		return response.InternalError(ctx)
	}
	return response.OK(ctx, addresses)
}

func (h *UserHandler) AddAddress(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.AddressInput
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	addr, err := h.svc.AddAddress(user.ID, input)
	if err != nil {
		return response.InternalError(ctx)
	}
	return response.Created(ctx, addr)
}

func (h *UserHandler) UpdateAddress(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid address id")
	}
	var input dto.AddressInput
	if err = ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	addr, err := h.svc.UpdateAddress(user.ID, uint(id), input)
	if err != nil {
		if err.Error() == "address not found" {
			return response.NotFound(ctx, err.Error())
		}
		if err.Error() == "unauthorized" {
			return response.Forbidden(ctx, err.Error())
		}
		return response.InternalError(ctx)
	}
	return response.OK(ctx, addr)
}

func (h *UserHandler) DeleteAddress(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid address id")
	}
	if err = h.svc.DeleteAddress(user.ID, uint(id)); err != nil {
		if err.Error() == "address not found" {
			return response.NotFound(ctx, err.Error())
		}
		if err.Error() == "unauthorized" {
			return response.Forbidden(ctx, err.Error())
		}
		return response.InternalError(ctx)
	}
	return response.NoContent(ctx)
}

// ── Cart ──────────────────────────────────────────────────────────────────────

func (h *UserHandler) AddToCart(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.AddToCartInput
	if err := ctx.BodyParser(&input); err != nil || input.ProductID == 0 {
		return response.BadRequest(ctx, "product_id is required")
	}
	cart, err := h.cartSvc.AddToCart(user.ID, input)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.OK(ctx, cart)
}

func (h *UserHandler) GetCart(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	cart, err := h.cartSvc.GetCart(user.ID)
	if err != nil {
		return response.InternalError(ctx)
	}
	return response.OK(ctx, cart)
}

func (h *UserHandler) RemoveFromCart(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	itemID, err := ctx.ParamsInt("item_id")
	if err != nil {
		return response.BadRequest(ctx, "invalid item id")
	}
	if err = h.cartSvc.RemoveFromCart(user.ID, uint(itemID)); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.NoContent(ctx)
}

// ── Orders ────────────────────────────────────────────────────────────────────

func (h *UserHandler) PlaceOrder(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.PlaceOrderInput
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	order, err := h.orderSvc.PlaceOrder(user.ID, input)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.Created(ctx, order)
}

func (h *UserHandler) GetOrders(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	orders, err := h.orderSvc.GetOrders(user.ID)
	if err != nil {
		return response.InternalError(ctx)
	}
	return response.OK(ctx, orders)
}

func (h *UserHandler) GetOrder(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}
	order, err := h.orderSvc.GetOrder(uint(id), user.ID)
	if err != nil {
		return response.NotFound(ctx, err.Error())
	}
	return response.OK(ctx, order)
}

// ── Route setup ───────────────────────────────────────────────────────────────

func SetupUserRoutes(restHandler *rest.RestHandler) {
	userSvc := service.NewUserService(restHandler.DB, restHandler.Auth, restHandler.NotificationClient)
	cartSvc := service.NewCartService(restHandler.DB)
	orderSvc := service.NewOrderService(restHandler.DB, restHandler.SQSClient, restHandler.StripeClient)

	h := UserHandler{svc: userSvc, cartSvc: cartSvc, orderSvc: orderSvc}
	authLimit := middleware.AuthRateLimiter()

	pub := restHandler.App.Group("/user")
	me := pub.Group("/me", restHandler.Auth.Authorize)

	// Public — auth endpoints with strict rate limiting
	pub.Post("/register", authLimit, h.Register)
	pub.Post("/login", authLimit, h.Login)
	pub.Post("/refresh", authLimit, h.Refresh)
	pub.Post("/logout", h.Logout)

	// Protected
	me.Post("/logout-all", h.LogoutAll)
	me.Get("/verify", h.GetVerificationCode)
	me.Post("/verify", h.Verify)
	me.Get("/profile", h.GetProfile)
	me.Post("/profile", h.CreateProfile)
	me.Post("/become-seller", h.BecomeSeller)

	// Addresses
	me.Get("/address", h.GetAddresses)
	me.Post("/address", h.AddAddress)
	me.Put("/address/:id", h.UpdateAddress)
	me.Delete("/address/:id", h.DeleteAddress)

	// Cart
	me.Post("/cart", h.AddToCart)
	me.Get("/cart", h.GetCart)
	me.Delete("/cart/:item_id", h.RemoveFromCart)

	// Orders
	me.Post("/order", h.PlaceOrder)
	me.Get("/order", h.GetOrders)
	me.Get("/order/:id", h.GetOrder)
}
