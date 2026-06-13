package repository

import (
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"

	"gorm.io/gorm"
)

type ProductRepository interface {
	CreateProduct(product domain.Product) (*domain.Product, error)
	FindProductByID(id uint) (*domain.Product, error)
	FindProductsBySellerID(sellerID uint) ([]domain.Product, error)
	FindProducts(filter dto.ProductFilter) ([]domain.Product, int64, error)
	UpdateProduct(id uint, updates map[string]interface{}) (*domain.Product, error)
	DeleteProduct(id uint) error
	FindCategories() ([]domain.Category, error)
	CreateCategory(cat domain.Category) (*domain.Category, error)
	AddProductImage(image domain.ProductImage) (*domain.ProductImage, error)
}

type productRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepository{db}
}

func (r *productRepository) CreateProduct(product domain.Product) (*domain.Product, error) {
	if err := r.db.Create(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *productRepository) FindProductByID(id uint) (*domain.Product, error) {
	var product domain.Product
	err := r.db.Preload("Category").Preload("Images").First(&product, id).Error
	return &product, err
}

func (r *productRepository) FindProductsBySellerID(sellerID uint) ([]domain.Product, error) {
	var products []domain.Product
	err := r.db.Preload("Category").Preload("Images").
		Where("seller_id = ?", sellerID).Find(&products).Error
	return products, err
}

func (r *productRepository) FindProducts(filter dto.ProductFilter) ([]domain.Product, int64, error) {
	var products []domain.Product
	var count int64

	query := r.db.Model(&domain.Product{}).Where("status = ?", "active")
	if filter.CategoryID > 0 {
		query = query.Where("category_id = ?", filter.CategoryID)
	}
	if filter.MinPrice > 0 {
		query = query.Where("price >= ?", filter.MinPrice)
	}
	if filter.MaxPrice > 0 {
		query = query.Where("price <= ?", filter.MaxPrice)
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", like, like)
	}

	query.Count(&count)

	page, limit := filter.Page, filter.Limit
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	err := query.Preload("Category").Preload("Images").
		Limit(limit).Offset((page - 1) * limit).Find(&products).Error
	return products, count, err
}

func (r *productRepository) UpdateProduct(id uint, updates map[string]interface{}) (*domain.Product, error) {
	var product domain.Product
	if err := r.db.First(&product, id).Error; err != nil {
		return nil, err
	}
	if err := r.db.Model(&product).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

// DeleteProduct soft-deletes a product by marking it "archived" rather than
// removing the row. This hides it from the catalog (which filters status =
// 'active') while preserving order history — order_items reference products
// with an ON DELETE RESTRICT constraint, so a hard delete would fail for any
// product that has ever been ordered.
func (r *productRepository) DeleteProduct(id uint) error {
	return r.db.Model(&domain.Product{}).Where("id = ?", id).
		Update("status", "archived").Error
}

func (r *productRepository) FindCategories() ([]domain.Category, error) {
	var categories []domain.Category
	err := r.db.Find(&categories).Error
	return categories, err
}

func (r *productRepository) CreateCategory(cat domain.Category) (*domain.Category, error) {
	if err := r.db.Create(&cat).Error; err != nil {
		return nil, err
	}
	return &cat, nil
}

func (r *productRepository) AddProductImage(image domain.ProductImage) (*domain.ProductImage, error) {
	if err := r.db.Create(&image).Error; err != nil {
		return nil, err
	}
	return &image, nil
}
