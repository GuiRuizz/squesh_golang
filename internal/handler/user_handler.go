package handler

import (
	"net/http"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{DB: db}
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	// Obter do contexto do Gin
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return
	}

	// Type assertion direto para uuid.UUID
	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return
	}

	// Agora você já tem a variável 'userID' do tipo uuid.UUID para usar nas buscas do GORM!

	var user domain.User
	if err := h.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	// Calcula o streak baseado nos posts do usuário
	streak := h.calculateStreak(user.ID)

	response := dto.UserProfileResponseDTO{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		AvatarURL: user.AvatarURL,
		Streak:    streak,
		CreatedAt: user.CreatedAt,
	}

	c.JSON(http.StatusOK, response)
}

// Lógica de cálculo de dias consecutivos com postagens
func (h *UserHandler) calculateStreak(userID uuid.UUID) int {
	var postDates []time.Time

	// Busca apenas a data (sem hora) dos posts do usuário, ordenados do mais recente ao mais antigo
	h.DB.Model(&domain.Post{}).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Pluck("DATE(created_at)", &postDates)

	if len(postDates) == 0 {
		return 0
	}

	now := time.Now().Truncate(24 * time.Hour)
	latestPostDate := postDates[0].Truncate(24 * time.Hour)

	// Se o post mais recente for anterior a ontem, o streak foi quebrado
	daysDifference := int(now.Sub(latestPostDate).Hours() / 24)
	if daysDifference > 1 {
		return 0
	}

	streak := 0
	checkDate := latestPostDate

	// Mapeia datas únicas em que o usuário postou
	dateSet := make(map[string]bool)
	for _, dt := range postDates {
		dateSet[dt.Format("2006-01-02")] = true
	}

	// Incrementa enquanto houver post no dia consecutivo anterior
	for {
		dateStr := checkDate.Format("2006-01-02")
		if dateSet[dateStr] {
			streak++
			checkDate = checkDate.AddDate(0, 0, -1)
		} else {
			break
		}
	}

	return streak
}
