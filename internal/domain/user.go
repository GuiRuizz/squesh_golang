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
	Bio            string     `gorm:"size:200" json:"bio"`
	StreakCount    int        `gorm:"default:0;not null" json:"streak"`
	LastActiveDate *time.Time `json:"last_active_date,omitempty"`
	Points         int        `gorm:"default:0;not null" json:"points"`
	// StripeCustomerID é o cliente (cus_xxx) no Stripe. Fica vazio até o
	// primeiro pagamento e é criado uma única vez: as cobranças seguintes
	// reaproveitam o mesmo cliente, e é nele que o cartão fica guardado para a
	// renovação da assinatura.
	StripeCustomerID string `gorm:"size:120;index" json:"-"`
	// Preferências de notificação em jsonb. O campo não aparece no JSON do
	// usuário: quem consome é o /users/me, via UserPreferencesResponseDTO.
	Preferences UserPreferences `gorm:"type:jsonb;serializer:json" json:"-"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// UserPreferences são as preferências de notificação do usuário.
//
// Os campos são PONTEIROS de propósito: nil = "nunca configurado", e aí vale o
// padrão (ligado). Com bool simples, uma linha criada antes das preferências
// existirem (coluna jsonb vazia) apareceria com tudo desligado, e o usuário
// também não conseguiria desligar as quatro de uma vez — o json salvo seria
// "{}" e voltaria como "tudo ligado".
type UserPreferences struct {
	PushEnabled      *bool `json:"push_enabled,omitempty"`
	WorkoutReminders *bool `json:"workout_reminders,omitempty"`
	ShopNews         *bool `json:"shop_news,omitempty"`
	SocialAlerts     *bool `json:"social_alerts,omitempty"`
}

// DefaultPreferences são as preferências de quem nunca mexeu em nada.
func DefaultPreferences() UserPreferences {
	on := true
	return UserPreferences{
		PushEnabled:      &on,
		WorkoutReminders: &on,
		ShopNews:         &on,
		SocialAlerts:     &on,
	}
}

func boolOr(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// Push é o botão geral: nenhuma notificação chega se estiver desligado.
func (p UserPreferences) Push() bool { return boolOr(p.PushEnabled, true) }

// Aceita só as categorias que o usuário deixou ligadas. Categoria vazia
// ("info"/"system") só depende do botão geral.
func (p UserPreferences) Accepts(category string) bool {
	if !p.Push() {
		return false
	}
	switch category {
	case "shop":
		return boolOr(p.ShopNews, true)
	case "streak", "workout", "trail":
		return boolOr(p.WorkoutReminders, true)
	case "social":
		return boolOr(p.SocialAlerts, true)
	default:
		return true
	}
}

// UserTrailProgress registra UMA conclusão do usuário:
//   - item SEM etapas internas: uma linha, com StepIndex = -1;
//   - item COM etapas (dia de alimentação / sessão de treino): uma linha por
//     etapa marcada, com StepIndex = índice dentro de TrailItem.Steps. A etapa
//     só conta como concluída quando todas as suas partes estão marcadas.
type UserTrailProgress struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index:idx_user_item_step,unique" json:"user_id"`
	TrailItemID uuid.UUID `gorm:"type:uuid;not null;index:idx_user_item_step,unique" json:"trail_item_id"`
	// StepIndex: -1 = item simples; >= 0 = índice da etapa dentro de TrailItem.Steps.
	// SEM tag `default`: o GORM substitui o valor zero pelo default no INSERT,
	// o que quebraria a etapa de índice 0. Sempre gravamos o campo explicitamente.
	StepIndex   int       `gorm:"not null;index:idx_user_item_step,unique" json:"step_index"`
	CompletedAt time.Time `json:"completed_at"`
}

func (base *User) BeforeCreate(tx *gorm.DB) (err error) {
	if base.ID == uuid.Nil {
		base.ID = uuid.New()
	}
	return
}
