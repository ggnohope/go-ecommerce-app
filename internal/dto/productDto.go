package dto

type CreateProductInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	CategoryID  uint    `json:"category_id"`
}

type UpdateProductInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Status      string  `json:"status"`
}

type ProductFilter struct {
	CategoryID uint    `query:"category_id"`
	MinPrice   float64 `query:"min_price"`
	MaxPrice   float64 `query:"max_price"`
	Search     string  `query:"search"`
	Page       int     `query:"page"`
	Limit      int     `query:"limit"`
}

type CreateCategoryInput struct {
	Name     string `json:"name"`
	ParentID *uint  `json:"parent_id"`
}
