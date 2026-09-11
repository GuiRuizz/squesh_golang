package handler

import (
	"net/http"
	"github.com/google/uuid"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PostHandler struct {
	DB *gorm.DB
}

func NewPostHandler(db *gorm.DB) *PostHandler {
	return &PostHandler{DB: db}
}

func (h *PostHandler) GetFeed(c *gin.Context) {
	var posts []domain.Post
	if err := h.DB.Preload("User").Preload("Comments.User").Order("created_at desc").Find(&posts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar posts do feed"})
		return
	}
	c.JSON(http.StatusOK, posts)
}

func (h *PostHandler) GetUserPosts(c *gin.Context) {
	userID := c.Param("user_id")
	var posts []domain.Post
	if err := h.DB.Where("user_id = ?", userID).Preload("Comments.User").Order("created_at desc").Find(&posts).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Posts não encontrados para este usuário"})
		return
	}
	c.JSON(http.StatusOK, posts)
}

type UpdateCaptionDTO struct {
	Caption string `json:"caption" binding:"required"`
}

func (h *PostHandler) UpdatePostCaption(c *gin.Context) {
	postID := c.Param("id")
	var dto UpdateCaptionDTO

	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	h.DB.Model(&post).Update("caption", dto.Caption)
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	postID := c.Param("id")
	result := h.DB.Delete(&domain.Post{}, "id = ?", postID)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao apagar o post"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	var input dto.CreatePostDTO

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	parsedUserID, err := uuid.Parse(input.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuário inválido"})
		return
	}

	// Valida se o usuário existe no banco
	var user domain.User
	if err := h.DB.First(&user, "id = ?", parsedUserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	post := domain.Post{
		UserID:   parsedUserID,
		ImageURL: input.ImageURL,
		Caption:  input.Caption,
	}

	if result := h.DB.Create(&post); result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar post: " + result.Error.Error()})
		return
	}

	c.JSON(http.StatusCreated, post)
}