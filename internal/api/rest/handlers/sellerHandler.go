package handlers

import (
	"go-ecommerce-app/internal/api/response"
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"

	"github.com/gofiber/fiber/v2"
)

type SellerHandler struct {
	svc service.ProductService
}

func (h *SellerHandler) CreateProduct(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	var input dto.CreateProductInput
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	product, err := h.svc.CreateProduct(user.ID, input)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.Created(ctx, product)
}

func (h *SellerHandler) GetSellerProducts(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	products, err := h.svc.GetSellerProducts(user.ID)
	if err != nil {
		return response.InternalError(ctx)
	}
	return response.OK(ctx, products)
}

func (h *SellerHandler) UpdateProduct(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid product id")
	}
	var input dto.UpdateProductInput
	if err = ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	product, err := h.svc.UpdateProduct(uint(id), user.ID, input)
	if err != nil {
		switch err.Error() {
		case "unauthorized":
			return response.Forbidden(ctx, err.Error())
		case "product not found":
			return response.NotFound(ctx, err.Error())
		default:
			return response.InternalError(ctx)
		}
	}
	return response.OK(ctx, product)
}

func (h *SellerHandler) DeleteProduct(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid product id")
	}
	if err = h.svc.DeleteProduct(uint(id), user.ID); err != nil {
		switch err.Error() {
		case "unauthorized":
			return response.Forbidden(ctx, err.Error())
		case "product not found":
			return response.NotFound(ctx, err.Error())
		default:
			return response.InternalError(ctx)
		}
	}
	return response.NoContent(ctx)
}

func (h *SellerHandler) UploadProductImage(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return response.Unauthorized(ctx, "unauthorized")
	}
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return response.BadRequest(ctx, "invalid product id")
	}
	file, err := ctx.FormFile("image")
	if err != nil {
		return response.BadRequest(ctx, "image file is required")
	}
	f, err := file.Open()
	if err != nil {
		return response.InternalError(ctx)
	}
	defer f.Close()

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	image, err := h.svc.UploadProductImage(uint(id), user.ID, file.Filename, f, contentType)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.Created(ctx, image)
}

func (h *SellerHandler) CreateCategory(ctx *fiber.Ctx) error {
	var input dto.CreateCategoryInput
	if err := ctx.BodyParser(&input); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	category, err := h.svc.CreateCategory(input)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	return response.Created(ctx, category)
}

func SetupSellerRoutes(restHandler *rest.RestHandler) {
	h := SellerHandler{svc: service.NewProductService(restHandler.DB, restHandler.S3Client)}

	seller := restHandler.App.Group("/seller",
		restHandler.Auth.Authorize,
		restHandler.Auth.SellerOnly,
	)
	seller.Post("/product", h.CreateProduct)
	seller.Get("/products", h.GetSellerProducts)
	seller.Put("/product/:id", h.UpdateProduct)
	seller.Delete("/product/:id", h.DeleteProduct)
	seller.Post("/product/:id/image", h.UploadProductImage)
	seller.Post("/category", h.CreateCategory)
}
