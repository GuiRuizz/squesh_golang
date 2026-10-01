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

// StepSpec é UM item interno de uma etapa:
//   - trilha de NUTRIÇÃO: a etapa é um DIA e os itens são as refeições
//     (Café da Manhã, Almoço, Café da Tarde, Jantar...), liberadas por horário;
//   - trilha de TREINO: a etapa é uma SESSÃO e os itens são os exercícios.
type StepSpec struct {
	Slot        string `json:"slot"`          // "cafe_manha" | "jantar" | "supino_reto"
	Title       string `json:"title"`         // "Café da Manhã" | "Supino Reto com Barra"
	Description string `json:"description"`   // detalhe do item
	Value       string `json:"value"`         // ex.: "4 séries de 10 a 12"
	RequiredHour int   `json:"required_hour"` // hora mínima para marcar (0 = sem trava de horário)
	// Done é transitório: preenchido pelo handler com o progresso do usuário
	// logado; nunca é salvo no banco (vem zerado nas escritas).
	Done *bool `json:"done,omitempty"`
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
//   - trilha de TREINO: a etapa é uma SESSÃO, e Steps traz os exercícios
//     (cada um marcado individualmente pelo app);
//   - trilha de NUTRIÇÃO: a etapa é um DIA, e Steps traz as refeições desse
//     dia (cada uma liberada pelo próprio horário).
//
// Etapas sem Steps continuam funcionando: um check só, no endpoint /complete.
type TrailItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TrailID     uuid.UUID `gorm:"type:uuid;not null" json:"trail_id"`
	Trail       Trail     `gorm:"foreignKey:TrailID" json:"-"` // associacao para validar o tipo da trilha (workout/nutrition)
	Order       int       `gorm:"not null" json:"order"`
	Title       string    `gorm:"type:varchar(100);not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Value       string    `gorm:"type:varchar(100)" json:"value"` // ex.: "3x12 repetições" ou "200g de peito de frango"
	// Steps traz os itens da etapa (refeições do dia / exercícios da sessão).
	// Precisa do serializer:json para o GORM gravar/tratar como coluna jsonb
	// (e não como relação).
	Steps     []StepSpec `gorm:"type:jsonb;serializer:json" json:"steps,omitempty"`
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
