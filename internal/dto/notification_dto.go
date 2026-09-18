package dto

import (
	"time"

	"github.com/google/uuid"
)

// NotificationResponse define o formato retornado na API
type NotificationResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Type      string    `json:"type"` // ex: "SHOP", "POST", "SYSTEM"
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// MarkAllAsReadResponse define o retorno após atualizar em lote
type MarkAllAsReadResponse struct {
	Message      string `json:"message"`
	RowsAffected int64  `json:"rows_affected"`
}
