package handler

import (
	"net/http"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TrailHandler struct {
	DB *gorm.DB
}

func NewTrailHandler(db *gorm.DB) *TrailHandler {
	return &TrailHandler{DB: db}
}

// ListTrails busca todas as trilhas ou filtra por tipo (?type=workout ou ?type=nutrition)
func (h *TrailHandler) ListTrails(c *gin.Context) {
	trailType := c.Query("type")

	var trails []domain.Trail
	query := h.DB.Preload("Items")

	if trailType != "" {
		query = query.Where("type = ?", trailType)
	}

	if err := query.Find(&trails).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar trilhas"})
		return
	}

	c.JSON(http.StatusOK, trails)
}

// GetTrailByID busca os detalhes de uma trilha específica
func (h *TrailHandler) GetTrailByID(c *gin.Context) {
	idParam := c.Param("id")
	trailID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de trilha inválido"})
		return
	}

	var trail domain.Trail
	if err := h.DB.Preload("Items").First(&trail, "id = ?", trailID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trilha não encontrada"})
		return
	}

	c.JSON(http.StatusOK, trail)
}

// CreateTrail cria uma nova trilha
func (h *TrailHandler) CreateTrail(c *gin.Context) {
	var input dto.CreateTrailDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trail := domain.Trail{
		Title:       input.Title,
		Description: input.Description,
		Type:        domain.TrailType(input.Type),
		Level:       input.Level,
	}

	if err := h.DB.Create(&trail).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar trilha"})
		return
	}

	c.JSON(http.StatusCreated, trail)
}

// AddItemToTrail insere uma nova etapa/item na trilha
func (h *TrailHandler) AddItemToTrail(c *gin.Context) {
	idParam := c.Param("id")
	trailID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de trilha inválido"})
		return
	}

	var input dto.CreateTrailItemDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item := domain.TrailItem{
		TrailID:     trailID,
		Title:       input.Title,
		Description: input.Description,
		Order:       input.Order,
		Value:       input.Value,
	}

	if err := h.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao adicionar item na trilha"})
		return
	}

	c.JSON(http.StatusCreated, item)
}
