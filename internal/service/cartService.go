package service

import (
	"errors"
	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/dto"
	"go-ecommerce-app/internal/repository"

	"gorm.io/gorm"
)

type CartService interface {
	AddToCart(userID uint, input dto.AddToCartInput) (*domain.Cart, error)
	GetCart(userID uint) (*domain.Cart, error)
	RemoveFromCart(userID uint, itemID uint) error
}

type cartService struct {
	cartRepo    repository.CartRepository
	productRepo repository.ProductRepository
}

func NewCartService(db *gorm.DB) CartService {
	return &cartService{
		cartRepo:    repository.NewCartRepository(db),
		productRepo: repository.NewProductRepository(db),
	}
}

func (s *cartService) AddToCart(userID uint, input dto.AddToCartInput) (*domain.Cart, error) {
	if input.Quantity <= 0 {
		input.Quantity = 1
	}

	product, err := s.productRepo.FindProductByID(input.ProductID)
	if err != nil {
		return nil, errors.New("product not found")
	}
	if product.Status != "active" {
		return nil, errors.New("product is not available")
	}
	if product.Stock < input.Quantity {
		return nil, errors.New("insufficient stock")
	}

	cart, err := s.cartRepo.FindOrCreateCart(userID)
	if err != nil {
		return nil, err
	}

	existingItem, err := s.cartRepo.FindCartItem(cart.ID, input.ProductID)
	if err == nil {
		newQty := existingItem.Quantity + input.Quantity
		if err = s.cartRepo.UpdateItemQuantity(existingItem.ID, newQty); err != nil {
			return nil, err
		}
	} else {
		if _, err = s.cartRepo.AddItem(domain.CartItem{
			CartID:    cart.ID,
			ProductID: input.ProductID,
			Quantity:  input.Quantity,
			Price:     product.Price,
		}); err != nil {
			return nil, err
		}
	}

	return s.cartRepo.GetCart(userID)
}

func (s *cartService) GetCart(userID uint) (*domain.Cart, error) {
	cart, err := s.cartRepo.GetCart(userID)
	if err != nil {
		return &domain.Cart{UserID: userID, Items: []domain.CartItem{}}, nil
	}
	return cart, nil
}

func (s *cartService) RemoveFromCart(userID uint, itemID uint) error {
	cart, err := s.cartRepo.FindOrCreateCart(userID)
	if err != nil {
		return errors.New("cart not found")
	}
	return s.cartRepo.RemoveItemFromCart(cart.ID, itemID)
}
