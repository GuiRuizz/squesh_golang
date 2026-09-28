package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Post struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	ImageURL  string    `gorm:"not null" json:"image_url"`
	Caption   string    `json:"caption"`
	Comments  []Comment `gorm:"foreignKey:PostID;constraint:OnDelete:CASCADE" json:"comments,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// LikesCount e LikedByMe são transitórios (gorm:"-"): preenchidos pelo
	// handler em lote a partir da tabela de curtidas; liked_by_me só aparece
	// quando há um usuário autenticado na requisição.
	LikesCount int   `gorm:"-" json:"likes_count"`
	LikedByMe  *bool `gorm:"-" json:"liked_by_me,omitempty"`
}

func (base *Post) BeforeCreate(tx *gorm.DB) (err error) {
	if base.ID == uuid.Nil {
		base.ID = uuid.New()
	}
	return
}
