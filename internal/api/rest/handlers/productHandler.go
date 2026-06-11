package handlers

import (
	"go-ecommerce-app/internal/api/rest"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/service"
	"go-ecommerce-app/pkg/storage"

	"github.com/gofiber/fiber/v2"
)

type ProductHandler struct {
	svc service.ProductService
}

func (h *ProductHandler) GetProducts(ctx *fiber.Ctx) error {
	var filter dto.ProductFilter
	if err := ctx.QueryParser(&filter); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid query params"})
	}
	products, total, err := h.svc.GetProducts(filter)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.JSON(fiber.Map{"data": products, "total": total})
}

func (h *ProductHandler) GetProduct(ctx *fiber.Ctx) error {
	id, err := ctx.ParamsInt("id")
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid product id"})
	}
	product, err := h.svc.GetProduct(uint(id))
	if err != nil {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.JSON(fiber.Map{"data": product})
}

func (h *ProductHandler) GetCategories(ctx *fiber.Ctx) error {
	categories, err := h.svc.GetCategories()
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return ctx.JSON(fiber.Map{"data": categories})
}

func SetupProductRoutes(restHandler *rest.RestHandler) {
	var s3Client *storage.S3Client
	if restHandler.S3Client != nil {
		s3Client = restHandler.S3Client
	}

	productSvc := service.NewProductService(restHandler.DB, s3Client)
	h := ProductHandler{svc: productSvc}

	app := restHandler.App
	app.Get("/products", h.GetProducts)
	app.Get("/products/:id", h.GetProduct)
	app.Get("/categories", h.GetCategories)
}
