package domain

import "time"

type RefreshToken struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	UserID    uint       `json:"-" gorm:"not null;index"`
	TokenHash string     `json:"-" gorm:"uniqueIndex;not null"`
	UserAgent string     `json:"user_agent"`
	IPAddress string     `json:"ip_address"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
}
