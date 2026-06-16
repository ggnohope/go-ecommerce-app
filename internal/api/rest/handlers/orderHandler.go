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

// CreatePaymentLink godoc
// @Summary     Create a payOS payment link for an order
// @Tags        orders
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body dto.CreatePaymentLinkInput true "Order ID"
// @Success     200 {object} response.APIResponse
// @Failure     400 {object} response.ErrorResponse
// @Failure     401 {object} response.ErrorResponse
// @Failure     404 {object} response.ErrorResponse
// @Router      /orders/payment/link [post]
func (h *OrderHandler) CreatePaymentLink(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.CreatePaymentLinkInput
	if err := ctx.BodyParser(&input); err != nil || input.OrderID == 0 {
		return response.BadRequest(ctx, "order_id is required")
	}
	link, err := h.svc.CreatePaymentLink(input.OrderID, user.ID)
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
	return response.OK(ctx, link)
}

// PayOSWebhook godoc
// @Summary     payOS webhook receiver
// @Description Receives and processes payOS payment events. Should only be called by payOS.
// @Tags        orders
// @Accept      json
// @Produce     json
// @Success     200
// @Failure     400 {object} response.ErrorResponse
// @Router      /orders/payment/webhook [post]
func (h *OrderHandler) PayOSWebhook(ctx *fiber.Ctx) error {
	if err := h.svc.HandlePayOSWebhook(ctx.Body()); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return ctx.SendStatus(fiber.StatusOK)
}

func SetupOrderRoutes(restHandler *rest.RestHandler) {
	orderSvc := service.NewOrderService(restHandler.DB, restHandler.SQSClient, restHandler.PayOSClient)
	h := OrderHandler{svc: orderSvc}

	orders := restHandler.App.Group("/orders")
	orders.Post("/payment/webhook", h.PayOSWebhook)
	orders.Post("/payment/link", restHandler.Auth.Authorize, h.CreatePaymentLink)
}
