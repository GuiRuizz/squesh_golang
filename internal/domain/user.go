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

// UserTrailProgress registra UMA conclusão do usuário:
//   - item de TREINO: uma linha por item, com MealIndex = -1;
//   - item de NUTRIÇÃO (um dia): uma linha por refeição marcada, com MealIndex
//     = índice da refeição dentro de TrailItem.Meals. O dia só conta como
//     concluído quando todas as refeições dele estão marcadas.
type UserTrailProgress struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index:idx_user_item_meal,unique" json:"user_id"`
	TrailItemID uuid.UUID `gorm:"type:uuid;not null;index:idx_user_item_meal,unique" json:"trail_item_id"`
	// MealIndex: -1 = item simples (treino); >= 0 = índice da refeição no dia.
	// SEM tag `default`: o GORM substitui o valor zero pelo default no INSERT,
	// o que quebraria a refeição de índice 0 (Café da Manhã). Sempre gravamos
	// o campo explicitamente.
	MealIndex   int       `gorm:"not null;index:idx_user_item_meal,unique" json:"meal_index"`
	CompletedAt time.Time `json:"completed_at"`
}

func (base *User) BeforeCreate(tx *gorm.DB) (err error) {
	if base.ID == uuid.Nil {
		base.ID = uuid.New()
	}
	return
}
