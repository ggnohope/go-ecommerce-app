package dto

type UserLogin struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type UserSignUp struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Phone    string `json:"phone" binding:"required"`
}

type VerifyUser struct {
	Code string `json:"code" binding:"required"`
}

type UserProfile struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type AddressInput struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	Country    string `json:"country"`
	PostalCode string `json:"postal_code"`
	IsDefault  bool   `json:"is_default"`
}

type AddToCartInput struct {
	ProductID uint `json:"product_id"`
	Quantity  int  `json:"quantity"`
}

// TokenPair is returned by login, register, and refresh endpoints.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // access token TTL in seconds
}

type RefreshTokenInput struct {
	RefreshToken string `json:"refresh_token"`
}
