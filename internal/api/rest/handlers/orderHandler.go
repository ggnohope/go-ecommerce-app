package handlers

import (
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"

	"github.com/gofiber/fiber/v2"
)

type OrderHandler struct {
	svc service.OrderService
}

func (h *OrderHandler) CreatePaymentIntent(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	var input dto.CreatePaymentIntentInput
	if err := ctx.BodyParser(&input); err != nil || input.OrderID == 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "order_id is required"})
	}

	pi, err := h.svc.CreatePaymentIntent(input.OrderID, user.ID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err.Error() == "order not found" {
			status = fiber.StatusNotFound
		} else if err.Error() == "payment service not configured" || err.Error() == "order is already paid" {
			status = fiber.StatusBadRequest
		}
		return ctx.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.JSON(fiber.Map{"data": pi})
}

func (h *OrderHandler) StripeWebhook(ctx *fiber.Ctx) error {
	signature := ctx.Get("Stripe-Signature")
	if err := h.svc.HandleStripeEvent(ctx.Body(), signature); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.SendStatus(fiber.StatusOK)
}

func SetupOrderRoutes(restHandler *rest.RestHandler) {
	orderSvc := service.NewOrderService(restHandler.DB, restHandler.SQSClient, restHandler.StripeClient)
	h := OrderHandler{svc: orderSvc}

	orders := restHandler.App.Group("/orders")
	orders.Post("/payment/webhook", h.StripeWebhook)
	orders.Post("/payment/intent", restHandler.Auth.Authorize, h.CreatePaymentIntent)
}
