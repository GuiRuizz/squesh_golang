package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefreshToken guarda os refresh tokens de cada sessão do usuário.
// Só armazenamos o hash (SHA-256) do token, nunca o token em si.
type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"-"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_refresh_user" json:"-"`
	TokenHash string     `gorm:"size:64;not null;index" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"-"`
	RevokedAt *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

func (rt *RefreshToken) BeforeCreate(tx *gorm.DB) (err error) {
	if rt.ID == uuid.Nil {
		rt.ID = uuid.New()
	}
	return
}
