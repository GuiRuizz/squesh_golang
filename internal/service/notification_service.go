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
//
// Ela respeita as preferências do usuário: se ele desligou a categoria da
// notificação (loja, treino, social) ou o botão geral, a notificação é
// descartada e nenhum registro é criado. Sem registro, o app também não
// mostra sino nenhum — que é o efeito esperado de "desligei isso".
func (s *NotificationService) CreateNotification(userID uuid.UUID, title, message, notifType string) error {
	var user domain.User
	if err := s.DB.Select("id", "preferences").First(&user, "id = ?", userID).Error; err != nil {
		// Usuário não encontrado (ou já apagado): não há para quem enviar.
		return nil
	}

	if !user.Preferences.Accepts(notifType) {
		return nil
	}

	notif := domain.Notification{
		UserID:  userID,
		Title:   title,
		Message: message,
		Type:    notifType, // ex: "shop", "streak", "system"
	}

	return s.DB.Create(&notif).Error
}
