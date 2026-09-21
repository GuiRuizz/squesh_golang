package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID             uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name           string     `gorm:"size:100;not null" json:"name"`
	Email          string     `gorm:"size:150;uniqueIndex;not null" json:"email"`
	Password       string     `gorm:"not null" json:"-"`
	Role           string     `gorm:"size:20;default:'user';not null" json:"role"` // "user" ou "admin"
	AvatarURL      string     `json:"avatar_url"`
	StreakCount    int        `gorm:"default:0;not null" json:"streak"`
	LastActiveDate *time.Time `json:"last_active_date,omitempty"`
	Points         int        `gorm:"default:0;not null" json:"points"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type UserTrailProgress struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index:idx_user_item,unique" json:"user_id"`
	TrailItemID uuid.UUID `gorm:"type:uuid;not null;index:idx_user_item,unique" json:"trail_item_id"`
	CompletedAt time.Time `json:"completed_at"`
}

func (base *User) BeforeCreate(tx *gorm.DB) (err error) {
	if base.ID == uuid.Nil {
		base.ID = uuid.New()
	}
	return
}
