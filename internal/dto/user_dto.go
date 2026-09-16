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
