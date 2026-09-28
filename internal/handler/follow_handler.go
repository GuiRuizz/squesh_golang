package handler

import (
	"net/http"

	"squesh_golang/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FollowHandler struct {
	DB *gorm.DB
}

func NewFollowHandler(db *gorm.DB) *FollowHandler {
	return &FollowHandler{DB: db}
}

// FollowUser faz o usuário logado seguir outro (POST /users/:id/follow).
// Idempotente: já seguindo não é erro. Não permite seguir a si mesmo.
func (h *FollowHandler) FollowUser(c *gin.Context) {
	followerID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuário inválido"})
		return
	}

	if targetID == followerID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Você não pode seguir a si mesmo"})
		return
	}

	var target domain.User
	if err := h.DB.First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	var count int64
	if err := h.DB.Model(&domain.Follow{}).
		Where("follower_id = ? AND following_id = ?", followerID, targetID).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao verificar relação"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusOK, gin.H{"message": "Você já segue este usuário", "following": true})
		return
	}

	follow := domain.Follow{FollowerID: followerID, FollowingID: targetID}
	if err := h.DB.Create(&follow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao seguir usuário"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Usuário seguido com sucesso",
		"following": true,
	})
}

// UnfollowUser deixa de seguir outro usuário (DELETE /users/:id/follow).
// Idempotente: 204 mesmo sem relação prévia.
func (h *FollowHandler) UnfollowUser(c *gin.Context) {
	followerID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuário inválido"})
		return
	}

	if err := h.DB.Where("follower_id = ? AND following_id = ?", followerID, targetID).
		Delete(&domain.Follow{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao deixar de seguir"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// ListFollowers lista quem segue o usuário (GET /users/:id/followers, público)
func (h *FollowHandler) ListFollowers(c *gin.Context) {
	h.listUsers(c, "following_id", "follower_id")
}

// ListFollowing lista quem o usuário segue (GET /users/:id/following, público)
func (h *FollowHandler) ListFollowing(c *gin.Context) {
	h.listUsers(c, "follower_id", "following_id")
}

// listUsers é o núcleo comum das duas listas: busca na tabela de relações pela
// coluna de entrada e devolve os usuários da coluna de saída.
func (h *FollowHandler) listUsers(c *gin.Context, byColumn, pickColumn string) {
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuário inválido"})
		return
	}

	var target domain.User
	if err := h.DB.First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	var relations []domain.Follow
	if err := h.DB.Where(byColumn+" = ?", targetID).
		Order("created_at desc").
		Find(&relations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar relações"})
		return
	}

	ids := make([]uuid.UUID, 0, len(relations))
	for _, r := range relations {
		switch pickColumn {
		case "follower_id":
			ids = append(ids, r.FollowerID)
		case "following_id":
			ids = append(ids, r.FollowingID)
		}
	}

	var users []domain.User
	if len(ids) > 0 {
		if err := h.DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar usuários"})
			return
		}
	}

	c.JSON(http.StatusOK, users)
}
