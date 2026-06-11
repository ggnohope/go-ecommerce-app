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

// GetProducts godoc
// @Summary     List products with optional filters
// @Tags        products
// @Produce     json
// @Param       category_id query int    false "Filter by category ID"
// @Param       min_price   query number false "Minimum price"
// @Param       max_price   query number false "Maximum price"
// @Param       search      query string false "Search term"
// @Param       page        query int    false "Page number (default 1)"
// @Param       limit       query int    false "Results per page (default 20)"
// @Success     200 {object} response.PaginatedAPIResponse{data=[]domain.Product}
// @Failure     400 {object} response.ErrorResponse
// @Router      /products [get]
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

// GetProduct godoc
// @Summary     Get a product by ID
// @Tags        products
// @Produce     json
// @Param       id path int true "Product ID"
// @Success     200 {object} response.APIResponse{data=domain.Product}
// @Failure     400 {object} response.ErrorResponse
// @Failure     404 {object} response.ErrorResponse
// @Router      /products/{id} [get]
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

// GetCategories godoc
// @Summary     List all product categories
// @Tags        products
// @Produce     json
// @Success     200 {object} response.APIResponse{data=[]domain.Category}
// @Failure     500 {object} response.ErrorResponse
// @Router      /categories [get]
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
