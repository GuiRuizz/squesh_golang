package handler

import (
	"net/http"
	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NotificationHandler struct {
	DB *gorm.DB
}

func NewNotificationHandler(db *gorm.DB) *NotificationHandler {
	return &NotificationHandler{DB: db}
}

// GetUserNotifications lista as notificações do usuário logado
func (h *NotificationHandler) GetUserNotifications(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}
	userID := userIDCtx.(uuid.UUID)

	var notifications []domain.Notification
	if err := h.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar notificações"})
		return
	}

	// Mapeia domain.Notification para dto.NotificationResponse
	responseList := make([]dto.NotificationResponse, len(notifications))
	for i, notif := range notifications {
		responseList[i] = dto.NotificationResponse{
			ID:        notif.ID,
			UserID:    notif.UserID,
			Title:     notif.Title,
			Message:   notif.Message,
			Type:      notif.Type,
			IsRead:    notif.IsRead,
			CreatedAt: notif.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": responseList,
	})
}

// MarkAsRead marca uma notificação específica como lida
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}
	userID := userIDCtx.(uuid.UUID)
	notificationID := c.Param("id")

	result := h.DB.Model(&domain.Notification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("is_read", true)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar notificação"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notificação não encontrada"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notificação marcada como lida"})
}

// MarkAllAsRead marca TODAS as notificações não lidas do usuário como lidas
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}
	userID := userIDCtx.(uuid.UUID)

	// Atualiza apenas as notificações do usuário que ainda não foram lidas
	result := h.DB.Model(&domain.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar notificações"})
		return
	}

	c.JSON(http.StatusOK, dto.MarkAllAsReadResponse{
		Message:      "Todas as notificações foram marcadas como lidas",
		RowsAffected: result.RowsAffected,
	})
}

func (h *NotificationHandler) MarkAsUnread(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}
	userID := userIDCtx.(uuid.UUID)

	notificationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de notificação em formato inválido"})
		return
	}

	result := h.DB.Model(&domain.Notification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("is_read", false)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar notificação"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notificação não encontrada"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notificação marcada como não lida"})
}
	