package handlers

import (
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
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	var input dto.CreateProductInput
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}

	product, err := h.svc.CreateProduct(user.ID, input)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusCreated).JSON(fiber.Map{"data": product})
}

func (h *SellerHandler) GetSellerProducts(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	products, err := h.svc.GetSellerProducts(user.ID)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.JSON(fiber.Map{"data": products})
}

func (h *SellerHandler) UpdateProduct(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	id, err := ctx.ParamsInt("id")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid product id"})
	}

	var input dto.UpdateProductInput
	if err = ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}

	product, err := h.svc.UpdateProduct(uint(id), user.ID, input)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err.Error() == "unauthorized" {
			status = fiber.StatusForbidden
		} else if err.Error() == "product not found" {
			status = fiber.StatusNotFound
		}
		return ctx.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.JSON(fiber.Map{"data": product})
}

func (h *SellerHandler) DeleteProduct(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	id, err := ctx.ParamsInt("id")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid product id"})
	}

	if err = h.svc.DeleteProduct(uint(id), user.ID); err != nil {
		status := fiber.StatusInternalServerError
		if err.Error() == "unauthorized" {
			status = fiber.StatusForbidden
		} else if err.Error() == "product not found" {
			status = fiber.StatusNotFound
		}
		return ctx.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusNoContent).Send(nil)
}

func (h *SellerHandler) UploadProductImage(ctx *fiber.Ctx) error {
	user, ok := ctx.Locals("user").(domain.User)
	if !ok {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	id, err := ctx.ParamsInt("id")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid product id"})
	}

	file, err := ctx.FormFile("image")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "image file is required"})
	}

	f, err := file.Open()
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to read file"})
	}
	defer f.Close()

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	image, err := h.svc.UploadProductImage(uint(id), user.ID, file.Filename, f, contentType)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusCreated).JSON(fiber.Map{"data": image})
}

func (h *SellerHandler) CreateCategory(ctx *fiber.Ctx) error {
	var input dto.CreateCategoryInput
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid request body"})
	}
	category, err := h.svc.CreateCategory(input)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.Status(fiber.StatusCreated).JSON(fiber.Map{"data": category})
}

func SetupSellerRoutes(restHandler *rest.RestHandler) {
	productSvc := service.NewProductService(restHandler.DB, restHandler.S3Client)
	h := SellerHandler{svc: productSvc}

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
