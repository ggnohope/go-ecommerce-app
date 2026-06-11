package handlers

import (
	"go-ecommerce-app/internal/api/response"
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ProductHandler struct {
	svc service.ProductService
}

func (h *ProductHandler) GetProducts(ctx *fiber.Ctx) error {
	var filter dto.ProductFilter
	if err := ctx.QueryParser(&filter); err != nil {
		return response.BadRequest(ctx, "invalid query params")
	}
	products, total, err := h.svc.GetProducts(filter)
	if err != nil {
		return response.InternalError(ctx)
	}
	page, limit := filter.Page, filter.Limit
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	return response.Paginated(ctx, products, page, limit, total)
}

func (h *ProductHandler) GetProduct(ctx *fiber.Ctx) error {
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid product id")
	}
	product, err := h.svc.GetProduct(uint(id))
	if err != nil {
		return response.NotFound(ctx, "product not found")
	}
	return response.OK(ctx, product)
}

func (h *ProductHandler) GetCategories(ctx *fiber.Ctx) error {
	categories, err := h.svc.GetCategories()
	if err != nil {
		return response.InternalError(ctx)
	}
	return response.OK(ctx, categories)
}

func SetupProductRoutes(restHandler *rest.RestHandler) {
	h := ProductHandler{svc: service.NewProductService(restHandler.DB, restHandler.S3Client)}

	restHandler.App.Get("/products", h.GetProducts)
	restHandler.App.Get("/products/:id", h.GetProduct)
	restHandler.App.Get("/categories", h.GetCategories)

}
