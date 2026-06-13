package database

import (
	"fmt"
	"log/slog"

	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/internal/helper"

	"gorm.io/gorm"
)

// SeedPassword is the plaintext password assigned to every seeded demo account.
const SeedPassword = "password123"

// Seed inserts initial reference and demo data. It is idempotent: every row is
// matched on a natural key first, so running it repeatedly will not create
// duplicates. Requires the schema to already be migrated (run Up first).
func Seed(db *gorm.DB) error {
	auth := helper.NewAuth("seed") // secret is unused by bcrypt hashing
	hashed, err := auth.CreateHashedPassword(SeedPassword)
	if err != nil {
		return fmt.Errorf("seed: hash password: %w", err)
	}

	// ── Categories (the store departments) ──────────────────────────────────
	categoryNames := []string{"Lighting", "Seating", "Tableware", "Textiles", "Stationery"}
	categoryID := map[string]uint{}
	for _, name := range categoryNames {
		cat := domain.Category{Name: name}
		if err := db.Where("name = ?", name).FirstOrCreate(&cat).Error; err != nil {
			return fmt.Errorf("seed category %q: %w", name, err)
		}
		categoryID[name] = cat.ID
	}

	// ── Demo users ──────────────────────────────────────────────────────────
	seller := domain.User{
		FirstName: "Demo", LastName: "Seller",
		Email: "seller@demo.com", Phone: "+84900000001",
		Password: hashed, Verified: true, UserType: "seller",
	}
	if err := db.Where("email = ?", seller.Email).FirstOrCreate(&seller).Error; err != nil {
		return fmt.Errorf("seed seller: %w", err)
	}

	buyer := domain.User{
		FirstName: "Demo", LastName: "Buyer",
		Email: "buyer@demo.com", Phone: "+84900000002",
		Password: hashed, Verified: true, UserType: "buyer",
	}
	if err := db.Where("email = ?", buyer.Email).FirstOrCreate(&buyer).Error; err != nil {
		return fmt.Errorf("seed buyer: %w", err)
	}

	// ── Demo products (owned by the seller) ─────────────────────────────────
	type seedProduct struct {
		name, desc, category string
		price                float64
		stock                int
	}
	products := []seedProduct{
		{"Arc Floor Lamp", "Brushed-steel arc lamp with marble base.", "Lighting", 189.00, 25},
		{"Pendant Light", "Hand-blown glass pendant, warm dimmable LED.", "Lighting", 129.50, 40},
		{"Lounge Chair", "Mid-century walnut frame with wool upholstery.", "Seating", 449.00, 12},
		{"Ceramic Dinner Set", "12-piece stoneware set, reactive glaze.", "Tableware", 89.00, 60},
		{"Linen Throw", "Stonewashed pure-linen throw, 130x170cm.", "Textiles", 59.00, 80},
		{"A5 Notebook", "Dotted-grid notebook, 160gsm paper.", "Stationery", 12.00, 200},
	}
	created := 0
	for _, p := range products {
		prod := domain.Product{
			Name:        p.name,
			Description: p.desc,
			Price:       p.price,
			Stock:       p.stock,
			CategoryID:  categoryID[p.category],
			SellerID:    seller.ID,
			Status:      "active",
		}
		res := db.Where("name = ? AND seller_id = ?", p.name, seller.ID).FirstOrCreate(&prod)
		if res.Error != nil {
			return fmt.Errorf("seed product %q: %w", p.name, res.Error)
		}
		created += int(res.RowsAffected)
	}

	slog.Info("seed complete",
		"categories", len(categoryNames),
		"users", 2,
		"products", len(products),
		"new_products", created,
	)
	slog.Info("demo credentials",
		"seller", seller.Email,
		"buyer", buyer.Email,
		"password", SeedPassword,
	)
	return nil
}
