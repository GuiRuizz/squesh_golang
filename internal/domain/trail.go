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

// MealSpec é UMA refeição dentro de um DIA. Em trilhas de nutrição cada item
// da trilha representa um dia e carrega a lista de refeições desse dia
// (Café da Manhã, Almoço, Café da Tarde, Jantar...).
type MealSpec struct {
	Slot         string `json:"slot"`          // "cafe_manha" | "almoco" | "cafe_tarde" | "jantar"
	Title        string `json:"title"`         // "Café da Manhã"
	Description  string `json:"description"`   // "3 ovos mexidos + café sem açúcar"
	Value        string `json:"value"`         // opcional: detalhe/meta da refeição
	RequiredHour int    `json:"required_hour"` // hora mínima para marcar (6, 12, 15, 20)
	// Consumed é transitório: preenchido pelo handler com o progresso do
	// usuário logado; nunca é salvo no banco (vem zerado nas escritas).
	Consumed *bool `json:"consumed,omitempty"`
}

type Trail struct {
	ID          uuid.UUID   `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Title       string      `gorm:"type:varchar(100);not null" json:"title"`
	Description string      `gorm:"type:text" json:"description"`
	Type        TrailType   `gorm:"type:varchar(20);not null" json:"type"`  // "workout" ou "nutrition"
	Level       string      `gorm:"type:varchar(20);not null" json:"level"` // "iniciante", "intermediario", "avancado"
	Items       []TrailItem `gorm:"foreignKey:TrailID" json:"items,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// TrailItem é uma ETAPA da trilha:
//   - trilha de TREINO: a etapa é o próprio treino (Value ex.: "3x12 repetições").
//   - trilha de NUTRIÇÃO: a etapa é um DIA, e Meals traz as refeições desse
//     dia (cada uma marcada individualmente pelo app).
type TrailItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TrailID     uuid.UUID `gorm:"type:uuid;not null" json:"trail_id"`
	Trail       Trail     `gorm:"foreignKey:TrailID" json:"-"` // associacao para validar o tipo da trilha (workout/nutrition)
	Order       int       `gorm:"not null" json:"order"`
	Title       string    `gorm:"type:varchar(100);not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Value       string    `gorm:"type:varchar(100)" json:"value"` // ex: "3x12 repetições" ou "200g de peito de frango"
	// Meals traz as refeições do dia (só nutrição). Precisa do serializer:json
	// para o GORM gravar/tratar como coluna jsonb (e não como relação).
	Meals     []MealSpec `gorm:"type:jsonb;serializer:json" json:"meals,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	// Completed é transitório (gorm:"-"): preenchido pelo handler com o
	// progresso do usuário logado; omitido quando não há usuário.
	Completed *bool `gorm:"-" json:"completed,omitempty"`
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
