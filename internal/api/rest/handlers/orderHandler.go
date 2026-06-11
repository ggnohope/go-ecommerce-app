package handlers

import (
	"go-ecommerce-app/internal/api/response"
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"

	"github.com/gofiber/fiber/v2"
)

type OrderHandler struct {
	svc service.OrderService
}

// CreatePaymentIntent godoc
// @Summary     Create a Stripe payment intent for an order
// @Tags        orders
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body dto.CreatePaymentIntentInput true "Order ID"
// @Success     200 {object} response.APIResponse
// @Failure     400 {object} response.ErrorResponse
// @Failure     401 {object} response.ErrorResponse
// @Failure     404 {object} response.ErrorResponse
// @Router      /orders/payment/intent [post]
func (h *OrderHandler) CreatePaymentIntent(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.CreatePaymentIntentInput
	if err := ctx.BodyParser(&input); err != nil || input.OrderID == 0 {
		return response.BadRequest(ctx, "order_id is required")
	}
	pi, err := h.svc.CreatePaymentIntent(input.OrderID, user.ID)
	if err != nil {
		switch err.Error() {
		case "order not found":
			return response.NotFound(ctx, err.Error())
		case "payment service not configured", "order is already paid":
			return response.BadRequest(ctx, err.Error())
		default:
			return response.InternalError(ctx)
		}
	}
	return response.OK(ctx, pi)
}

// StripeWebhook godoc
// @Summary     Stripe webhook receiver
// @Description Receives and processes Stripe payment events. Should only be called by Stripe.
// @Tags        orders
// @Accept      json
// @Produce     json
// @Param       Stripe-Signature header string true "Stripe webhook signature"
// @Success     200
// @Failure     400 {object} response.ErrorResponse
// @Router      /orders/payment/webhook [post]
func (h *OrderHandler) StripeWebhook(ctx *fiber.Ctx) error {
	signature := ctx.Get("Stripe-Signature")
	if err := h.svc.HandleStripeEvent(ctx.Body(), signature); err != nil {
		return response.BadRequest(ctx, err.Error())
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
