package service

import (
	"squesh_golang/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NotificationService struct {
	DB *gorm.DB
}

func NewNotificationService(db *gorm.DB) *NotificationService {
	return &NotificationService{DB: db}
}

// CreateNotification envia uma notificação para o usuário (pode ser chamada por qualquer handler/módulo)
func (s *NotificationService) CreateNotification(userID uuid.UUID, title, message, notifType string) error {
	notif := domain.Notification{
		UserID:  userID,
		Title:   title,
		Message: message,
		Type:    notifType, // ex: "shop", "streak", "system"
	}

	return s.DB.Create(&notif).Error
}
