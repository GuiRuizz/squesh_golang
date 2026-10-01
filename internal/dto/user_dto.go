package dto

import (
	"time"

	"github.com/google/uuid"
)

// UserPreferencesResponseDTO são as preferências já com o padrão aplicado
// (nil virou true), então o app nunca precisa saber do detalhe do jsonb.
type UserPreferencesResponseDTO struct {
	PushEnabled      bool `json:"push_enabled"`
	WorkoutReminders bool `json:"workout_reminders"`
	ShopNews         bool `json:"shop_news"`
	SocialAlerts     bool `json:"social_alerts"`
}

type UserProfileResponseDTO struct {
	ID          uuid.UUID                   `json:"id"`
	Name        string                      `json:"name"`
	Email       string                      `json:"email"`
	Role        string                      `json:"role"`
	AvatarURL   string                      `json:"avatar_url"`
	Bio         string                      `json:"bio"`
	Streak      int                         `json:"streak"`
	Points      int                         `json:"points"`
	Preferences UserPreferencesResponseDTO  `json:"preferences"`
	CreatedAt   time.Time                   `json:"created_at"`
}

type UserRankingDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Streak    int       `json:"streak"`
	Position  int       `json:"position"`
}

// UpdateProfileDTO: todos os campos são PONTEIROS de propósito. O app edita o
// perfil por partes (só a bio, só a foto) e, com string simples, o que não
// viesse no corpo apagaria o dado gravado.
type UpdateProfileDTO struct {
	Name      *string `json:"name" binding:"omitempty,min=2,max=100"`
	Bio       *string `json:"bio" binding:"omitempty,max=200"`
	AvatarURL *string `json:"avatar_url" binding:"omitempty,max=500"`
}

// UpdatePreferencesDTO: só os campos enviados mudam; os ausentes ficam como
// estão. Por isso ponteiros de novo.
type UpdatePreferencesDTO struct {
	PushEnabled      *bool `json:"push_enabled"`
	WorkoutReminders *bool `json:"workout_reminders"`
	ShopNews         *bool `json:"shop_news"`
	SocialAlerts     *bool `json:"social_alerts"`
}

// UpdatePasswordDTO representa a troca de senha com validação
type UpdatePasswordDTO struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
}
