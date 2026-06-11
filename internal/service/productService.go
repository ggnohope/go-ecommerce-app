package service

import (
	"errors"
	"fmt"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/repository"
	"go-ecommerce-app/pkg/storage"
	"io"
	"log"
	"time"

	"gorm.io/gorm"
)

type ProductService interface {
	CreateProduct(sellerID uint, input dto.CreateProductInput) (*domain.Product, error)
	GetProduct(id uint) (*domain.Product, error)
	GetProducts(filter dto.ProductFilter) ([]domain.Product, int64, error)
	UpdateProduct(id uint, sellerID uint, input dto.UpdateProductInput) (*domain.Product, error)
	DeleteProduct(id uint, sellerID uint) error
	UploadProductImage(productID uint, sellerID uint, filename string, data io.Reader, contentType string) (*domain.ProductImage, error)
	GetSellerProducts(sellerID uint) ([]domain.Product, error)
	GetCategories() ([]domain.Category, error)
	CreateCategory(input dto.CreateCategoryInput) (*domain.Category, error)
}

type productService struct {
	repo     repository.ProductRepository
	s3Client *storage.S3Client
}

func NewProductService(db *gorm.DB, s3Client *storage.S3Client) ProductService {
	return &productService{
		repo:     repository.NewProductRepository(db),
		s3Client: s3Client,
	}
}

func (s *productService) CreateProduct(sellerID uint, input dto.CreateProductInput) (*domain.Product, error) {
	if input.Name == "" {
		return nil, errors.New("product name is required")
	}
	if input.Price <= 0 {
		return nil, errors.New("price must be greater than 0")
	}
	return s.repo.CreateProduct(domain.Product{
		Name:        input.Name,
		Description: input.Description,
		Price:       input.Price,
		Stock:       input.Stock,
		CategoryID:  input.CategoryID,
		SellerID:    sellerID,
	})
}

func (s *productService) GetProduct(id uint) (*domain.Product, error) {
	product, err := s.repo.FindProductByID(id)
	if err != nil {
		return nil, errors.New("product not found")
	}
	return product, nil
}

func (s *productService) GetProducts(filter dto.ProductFilter) ([]domain.Product, int64, error) {
	return s.repo.FindProducts(filter)
}

func (s *productService) UpdateProduct(id uint, sellerID uint, input dto.UpdateProductInput) (*domain.Product, error) {
	product, err := s.repo.FindProductByID(id)
	if err != nil {
		return nil, errors.New("product not found")
	}
	if product.SellerID != sellerID {
		return nil, errors.New("unauthorized")
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Price > 0 {
		updates["price"] = input.Price
	}
	if input.Stock >= 0 {
		updates["stock"] = input.Stock
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}

	return s.repo.UpdateProduct(id, updates)
}

func (s *productService) DeleteProduct(id uint, sellerID uint) error {
	product, err := s.repo.FindProductByID(id)
	if err != nil {
		return errors.New("product not found")
	}
	if product.SellerID != sellerID {
		return errors.New("unauthorized")
	}
	return s.repo.DeleteProduct(id)
}

func (s *productService) UploadProductImage(productID uint, sellerID uint, filename string, data io.Reader, contentType string) (*domain.ProductImage, error) {
	product, err := s.repo.FindProductByID(productID)
	if err != nil {
		return nil, errors.New("product not found")
	}
	if product.SellerID != sellerID {
		return nil, errors.New("unauthorized")
	}
	if s.s3Client == nil {
		return nil, errors.New("image storage not configured")
	}

	key := s.s3Client.ProductImageKey(productID, filename)
	url, err := s.s3Client.UploadFile(key, data, contentType)
	if err != nil {
		log.Printf("product: s3 upload failed product=%d err=%v", productID, err)
		return nil, fmt.Errorf("failed to upload image: %w", err)
	}

	return s.repo.AddProductImage(domain.ProductImage{
		ProductID: productID,
		URL:       url,
		Position:  len(product.Images),
	})
}

func (s *productService) GetSellerProducts(sellerID uint) ([]domain.Product, error) {
	return s.repo.FindProductsBySellerID(sellerID)
}

func (s *productService) GetCategories() ([]domain.Category, error) {
	return s.repo.FindCategories()
}

func (s *productService) CreateCategory(input dto.CreateCategoryInput) (*domain.Category, error) {
	if input.Name == "" {
		return nil, errors.New("category name is required")
	}
	return s.repo.CreateCategory(domain.Category{
		Name:     input.Name,
		ParentID: input.ParentID,
	})
}
