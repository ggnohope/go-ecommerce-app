package response

import (
	"math"

	"github.com/gofiber/fiber/v2"
)

type envelope struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

type paginatedEnvelope struct {
	Data any   `json:"data"`
	Meta *Meta `json:"meta"`
}

// Meta holds pagination metadata returned in list responses.
type Meta struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
	Pages int   `json:"pages"`
}

// APIResponse is the standard envelope for successful responses.
// swagger:model
type APIResponse struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// ErrorResponse is the envelope returned on error responses.
// swagger:model
type ErrorResponse struct {
	Error string `json:"error"`
}

// PaginatedAPIResponse is the envelope for paginated list responses.
// swagger:model
type PaginatedAPIResponse struct {
	Data any   `json:"data"`
	Meta *Meta `json:"meta"`
}

func OK(ctx *fiber.Ctx, data any) error {
	return ctx.Status(fiber.StatusOK).JSON(envelope{Data: data})
}

func Created(ctx *fiber.Ctx, data any) error {
	return ctx.Status(fiber.StatusCreated).JSON(envelope{Data: data})
}

func NoContent(ctx *fiber.Ctx) error {
	return ctx.SendStatus(fiber.StatusNoContent)
}

func BadRequest(ctx *fiber.Ctx, msg string) error {
	return ctx.Status(fiber.StatusBadRequest).JSON(envelope{Error: msg})
}

func Unauthorized(ctx *fiber.Ctx, msg string) error {
	return ctx.Status(fiber.StatusUnauthorized).JSON(envelope{Error: msg})
}

func Forbidden(ctx *fiber.Ctx, msg string) error {
	return ctx.Status(fiber.StatusForbidden).JSON(envelope{Error: msg})
}

func NotFound(ctx *fiber.Ctx, msg string) error {
	return ctx.Status(fiber.StatusNotFound).JSON(envelope{Error: msg})
}

func InternalError(ctx *fiber.Ctx) error {
	return ctx.Status(fiber.StatusInternalServerError).JSON(envelope{Error: "internal server error"})
}

func TooManyRequests(ctx *fiber.Ctx) error {
	return ctx.Status(fiber.StatusTooManyRequests).JSON(envelope{Error: "too many requests, please slow down"})
}

func Paginated(ctx *fiber.Ctx, data any, page, limit int, total int64) error {
	pages := int(math.Ceil(float64(total) / float64(limit)))
	if pages == 0 {
		pages = 1
	}
	return ctx.Status(fiber.StatusOK).JSON(paginatedEnvelope{
		Data: data,
		Meta: &Meta{Page: page, Limit: limit, Total: total, Pages: pages},
	})
}
