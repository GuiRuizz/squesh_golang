package handler

import (
	"net/http"
	"strconv"
	"strings"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShopHandler struct {
	DB *gorm.DB
}

func NewShopHandler(db *gorm.DB) *ShopHandler {
	return &ShopHandler{DB: db}
}

// CREATE: Criar um novo item na loja (Admin)
func (h *ShopHandler) CreateItem(c *gin.Context) {
	var input dto.CreateShopItemDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item := domain.ShopItem{
		Name:        input.Name,
		Description: input.Description,
		Price:       float64(input.Price),
		ImageURL:    input.ImageURL,
		IsActive:    true,
	}

	if err := h.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar item na loja"})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// READ (List): Listar todos os itens ativos (Usuários/Admin)
func (h *ShopHandler) ListItems(c *gin.Context) {
	var items []domain.ShopItem
	if err := h.DB.Where("is_active = ?", true).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar itens da loja"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  items,
		"total": len(items),
	})
}

// GetItems lista itens da loja com busca por nome e paginação
func (h *ShopHandler) GetItems(c *gin.Context) {
	// 1. Captura os Query Parameters da URL
	searchQuery := c.Query("search") // Ex: /shop?search=neon
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	// 2. Converte page e limit para inteiros com valores padrão
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 { // Limite máximo de 100 por segurança
		limit = 10
	}

	// 3. Monta a query base (apenas itens ativos)
	query := h.DB.Model(&domain.ShopItem{}).Where("is_active = ?", true)

	// 4. Aplica o filtro de busca por nome (case-insensitive) se fornecido
	if strings.TrimSpace(searchQuery) != "" {
		// PostgreSQL suporta ILIKE. Para MySQL/SQLite padrão, use LIKE com LOWER():
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(searchQuery)+"%")
	}

	// 5. Conta o total de registros que satisfazem a busca (para metadados da paginação)
	var totalRecords int64
	if err := query.Count(&totalRecords).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao contar total de itens"})
		return
	}

	// 6. Aplica o Offset e Limit na consulta principal
	offset := (page - 1) * limit
	var items []domain.ShopItem

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar itens do catálogo"})
		return
	}

	// 7. Calcula o total de páginas
	totalPages := int((totalRecords + int64(limit) - 1) / int64(limit))

	// 8. Retorna a resposta paginada
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"meta": gin.H{
			"total_items": totalRecords,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// READ (Single): Obter detalhes de um item por ID
func (h *ShopHandler) GetItemByID(c *gin.Context) {
	idParam := c.Param("id")
	itemID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID em formato inválido"})
		return
	}

	var item domain.ShopItem
	if err := h.DB.First(&item, "id = ?", itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item não encontrado"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// UPDATE: Atualizar um item existente (Admin)
func (h *ShopHandler) UpdateItem(c *gin.Context) {
	idParam := c.Param("id")
	itemID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID em formato inválido"})
		return
	}

	var input dto.UpdateShopItemDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var item domain.ShopItem
	if err := h.DB.First(&item, "id = ?", itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item não encontrado"})
		return
	}

	if input.Name != nil {
		item.Name = *input.Name
	}
	if input.Description != nil {
		item.Description = *input.Description
	}
	if input.Price != nil {
		item.Price = float64(*input.Price)
	}
	if input.ImageURL != nil {
		item.ImageURL = *input.ImageURL
	}
	if input.IsActive != nil {
		item.IsActive = *input.IsActive
	}

	if err := h.DB.Save(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar item"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// DELETE: Remover um item (Soft Delete / Desativação ou Hard Delete) (Admin)
func (h *ShopHandler) DeleteItem(c *gin.Context) {
	idParam := c.Param("id")
	itemID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID em formato inválido"})
		return
	}

	var item domain.ShopItem
	if err := h.DB.First(&item, "id = ?", itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item não encontrado"})
		return
	}

	if err := h.DB.Delete(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao deletar item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item removido com sucesso"})
}

// BUY: Compra de um item
func (h *ShopHandler) BuyItem(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return
	}

	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return
	}

	var input dto.PurchaseItemDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var item domain.ShopItem
	if err := h.DB.First(&item, "id = ? AND is_active = ?", input.ItemID, true).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item não encontrado ou indisponível"})
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		inventory := domain.UserInventory{
			UserID: userID,
			ItemID: item.ID,
		}

		return tx.Create(&inventory).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao processar compra"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item adquirido com sucesso!"})
}

// INVENTORY: Listar inventário do usuário
func (h *ShopHandler) GetUserInventory(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return
	}

	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return
	}

	var inventory []domain.UserInventory
	if err := h.DB.Preload("Item").Where("user_id = ?", userID).Find(&inventory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar inventário"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  inventory,
		"total": len(inventory),
	})
}
