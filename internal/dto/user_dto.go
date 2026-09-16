package dto

import (
	"time"

	"github.com/google/uuid"
)

type UserProfileResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	AvatarURL string    `json:"avatar_url"`
	Streak    int       `json:"streak"`
	CreatedAt time.Time `json:"created_at"`
}

type UserRankingDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Streak    int       `json:"streak"`
	Position  int       `json:"position"`
}

type UpdateProfileDTO struct {
	Name      string `json:"name" binding:"required,min=2,max=100"`
	AvatarURL string `json:"avatar_url"`
}

// UpdatePasswordDTO representa a troca de senha com validação
type UpdatePasswordDTO struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
}
