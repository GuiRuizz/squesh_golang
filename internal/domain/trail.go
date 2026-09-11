package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TrailType string

const (
	TrailTypeWorkout   TrailType = "workout"
	TrailTypeNutrition TrailType = "nutrition"
)

type Trail struct {
	ID          uuid.UUID   `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Title       string      `gorm:"type:varchar(100);not null" json:"title"`
	Description string      `gorm:"type:text" json:"description"`
	Type        TrailType   `gorm:"type:varchar(20);not null" json:"type"` // "workout" ou "nutrition"
	Level       string      `gorm:"type:varchar(20);not null" json:"level"` // "iniciante", "intermediario", "avancado"
	Items       []TrailItem `gorm:"foreignKey:TrailID" json:"items,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type TrailItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TrailID     uuid.UUID `gorm:"type:uuid;not null" json:"trail_id"`
	Order       int       `gorm:"not null" json:"order"`
	Title       string    `gorm:"type:varchar(100);not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Value       string    `gorm:"type:varchar(100)" json:"value"` // ex: "3x12 repetições" ou "200g de peito de frango"
	CreatedAt   time.Time `json:"created_at"`
}

func (t *Trail) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return
}

func (ti *TrailItem) BeforeCreate(tx *gorm.DB) (err error) {
	if ti.ID == uuid.Nil {
		ti.ID = uuid.New()
	}
	return
}