package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"
	"squesh_golang/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// errOrderAlreadyConfirmed marca "outra requisição confirmou este pedido
// primeiro". Não dá para usar gorm.ErrInvalidTransaction: o GORM entra em panic
// com esse erro dentro de uma transação.
var errOrderAlreadyConfirmed = errors.New("pedido ja confirmado por outra requisicao")

type ShopHandler struct {
	DB                  *gorm.DB
	NotificationService *service.NotificationService
}

func NewShopHandler(db *gorm.DB, notifService *service.NotificationService) *ShopHandler {
	return &ShopHandler{
		DB:                  db,
		NotificationService: notifService,
	}
}

// ---------------------------------------------------------------- catalogo

// CREATE: Criar um novo item na loja (Admin)
func (h *ShopHandler) CreateItem(c *gin.Context) {
	var input dto.CreateShopItemDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item := domain.ShopItem{
		Name:        strings.TrimSpace(input.Name),
		Description: input.Description,
		PriceCents:  input.PriceCents,
		ImageURL:    input.ImageURL,
		Category:    strings.TrimSpace(input.Category),
		Rating:      input.Rating,
		IsActive:    true,
	}

	if err := h.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar item na loja"})
		return
	}

	c.JSON(http.StatusCreated, shopItemDTO(item))
}

// GetItems lista o catálogo com busca por nome e paginação.
//
// O app deriva os chips de categoria do que vem aqui (todo item carrega seu
// category), então não existe endpoint separado de categorias: uma categoria
// nova aparece sozinha assim que algum item a usa.
func (h *ShopHandler) GetItems(c *gin.Context) {
	searchQuery := c.Query("search")
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	query := h.DB.Model(&domain.ShopItem{}).Where("is_active = ?", true)

	if strings.TrimSpace(searchQuery) != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(searchQuery)+"%")
	}

	var totalRecords int64
	if err := query.Count(&totalRecords).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao contar total de itens"})
		return
	}

	offset := (page - 1) * limit
	var items []domain.ShopItem

	// sort_order nao existe em ShopItem: o mais novo primeiro, como antes.
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar itens do catálogo"})
		return
	}

	totalPages := int((totalRecords + int64(limit) - 1) / int64(limit))

	out := make([]dto.ShopItemResponseDTO, 0, len(items))
	for _, item := range items {
		out = append(out, shopItemDTO(item))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"total_items": totalRecords,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// GetItemByID devolve um item do catálogo.
func (h *ShopHandler) GetItemByID(c *gin.Context) {
	itemID, ok := uuidParam(c, "id", "ID em formato inválido")
	if !ok {
		return
	}

	var item domain.ShopItem
	if err := h.DB.First(&item, "id = ?", itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item não encontrado"})
		return
	}

	c.JSON(http.StatusOK, shopItemDTO(item))
}

// UpdateItem altera um item existente (Admin)
func (h *ShopHandler) UpdateItem(c *gin.Context) {
	itemID, ok := uuidParam(c, "id", "ID em formato inválido")
	if !ok {
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

	changes := map[string]any{}
	if input.Name != nil {
		changes["name"] = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		changes["description"] = *input.Description
	}
	if input.PriceCents != nil {
		changes["price_cents"] = *input.PriceCents
	}
	if input.ImageURL != nil {
		changes["image_url"] = *input.ImageURL
	}
	if input.Category != nil {
		changes["category"] = strings.TrimSpace(*input.Category)
	}
	if input.Rating != nil {
		changes["rating"] = *input.Rating
	}
	if input.IsActive != nil {
		changes["is_active"] = *input.IsActive
	}
	if len(changes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nada para atualizar"})
		return
	}

	if err := h.DB.Model(&item).Updates(changes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar item"})
		return
	}

	if err := h.DB.First(&item, "id = ?", item.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao recarregar item"})
		return
	}

	c.JSON(http.StatusOK, shopItemDTO(item))
}

// DeleteItem remove um item (Admin).
//
// Item que alguém já comprou não pode ser apagado: apagá-lo quebraria o
// inventário e o histórico de quem comprou (o item é referenciado por
// user_inventory). Nesse caso a resposta é 409 mandando desativar — desativar
// some da loja e preserva o que já foi vendido.
func (h *ShopHandler) DeleteItem(c *gin.Context) {
	itemID, ok := uuidParam(c, "id", "ID em formato inválido")
	if !ok {
		return
	}

	var item domain.ShopItem
	if err := h.DB.First(&item, "id = ?", itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item não encontrado"})
		return
	}

	var donos int64
	h.DB.Model(&domain.UserInventory{}).Where("item_id = ?", itemID).Count(&donos)
	if donos > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":   fmt.Sprintf("Este item já está no inventário de %d pessoa(s). Desative-o em vez de apagar.", donos),
			"owners":  donos,
		})
		return
	}

	if err := h.DB.Delete(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao deletar item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item removido com sucesso"})
}

func shopItemDTO(item domain.ShopItem) dto.ShopItemResponseDTO {
	return dto.ShopItemResponseDTO{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		PriceCents:  item.PriceCents,
		ImageURL:    item.ImageURL,
		Category:    item.Category,
		Rating:      item.Rating,
	}
}

// ---------------------------------------------------------------- pedidos

// CreateOrder fecha o carrinho.
//
// O app manda apenas QUANTOS quer de cada item. Os preços são lidos do catálogo
// aqui dentro: se o total viesse do cliente, dava para pedir R$ 300 de whey por
// R$ 0,01.
//
// O pedido nasce "pending" e os itens só entram no inventário quando o
// pagamento é confirmado (MarkOrderPaid — hoje via admin, quando o Stripe
// entrar será o webhook chamando o mesmo caminho).
func (h *ShopHandler) CreateOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input dto.CreateOrderDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Carrinho inválido: envie os itens e a quantidade"})
		return
	}

	// Um pedido aguardando pagamento por vez. Sem isso o usuário acumularia
	// pedidos abertos e não saberia qual pagar — e a entrega dependeria de
	// adivinhação.
	var open int64
	h.DB.Model(&domain.ShopOrder{}).
		Where("user_id = ? AND status = ?", userID, "pending").
		Count(&open)
	if open > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Você já tem um pedido aguardando pagamento"})
		return
	}

	// Junta item_ids repetidos: o carrinho pode mandar o mesmo item duas vezes
	// e o pedido precisa de uma linha só.
	quantities := map[uuid.UUID]int{}
	for _, line := range input.Items {
		quantities[line.ItemID] += line.Quantity
	}
	for _, qty := range quantities {
		if qty > 99 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Quantidade máxima de 99 por item"})
			return
		}
	}

	var catalog []domain.ShopItem
	if err := h.DB.Where("id IN ? AND is_active = ?", keysOf(quantities), true).
		Find(&catalog).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar os itens do carrinho"})
		return
	}
	if len(catalog) != len(quantities) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Um ou mais itens do carrinho não estão mais disponíveis"})
		return
	}

	// index por id: as linhas do pedido saem na ordem em que o app mandou, e
	// não na ordem do banco.
	byID := make(map[uuid.UUID]domain.ShopItem, len(catalog))
	for _, item := range catalog {
		byID[item.ID] = item
	}

	order := domain.ShopOrder{
		UserID: userID,
		Status: "pending",
	}
	for _, line := range input.Items {
		// O item pode ter vindo repetido; a linha seguinte usa a soma.
		if _, done := seenItem(order.Items, line.ItemID); done {
			continue
		}
		item := byID[line.ItemID]
		qty := quantities[line.ItemID]
		order.Items = append(order.Items, domain.ShopOrderItem{
			ItemID:         item.ID,
			Name:           item.Name,
			UnitPriceCents: item.PriceCents,
			Quantity:       qty,
		})
		order.TotalCents += item.PriceCents * qty
	}

	if err := h.DB.Create(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar o pedido"})
		return
	}

	c.JSON(http.StatusCreated, orderDTO(order))
}

// ListMyOrders é o histórico de pedidos do usuário, do mais novo para o mais
// antigo.
func (h *ShopHandler) ListMyOrders(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var orders []domain.ShopOrder
	if err := h.DB.Preload("Items").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar os pedidos"})
		return
	}

	out := make([]dto.ShopOrderResponseDTO, 0, len(orders))
	for _, order := range orders {
		out = append(out, orderDTO(order))
	}
	c.JSON(http.StatusOK, out)
}

// GetMyOrder devolve um pedido do usuário logado.
func (h *ShopHandler) GetMyOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	order, err := h.ownedOrder(c, userID)
	if err != nil {
		return
	}
	c.JSON(http.StatusOK, orderDTO(*order))
}

// CancelOrder descarta um pedido que ainda não foi pago. Pedido pago não se
// cancela aqui: ele já virou item no inventário, então o caminho é outro.
func (h *ShopHandler) CancelOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	order, err := h.ownedOrder(c, userID)
	if err != nil {
		return
	}
	if !order.IsPending() {
		c.JSON(http.StatusConflict, gin.H{"error": "Este pedido não está aguardando pagamento"})
		return
	}

	now := time.Now()
	if err := h.DB.Model(order).Updates(map[string]any{
		"status":      "canceled",
		"canceled_at": now,
		"updated_at":  now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao cancelar o pedido"})
		return
	}

	order.Status = "canceled"
	order.CanceledAt = &now
	c.JSON(http.StatusOK, orderDTO(*order))
}

// MarkOrderPaid confirma o pagamento e entrega os itens no inventário (Admin).
//
// Este é o único caminho que gera inventário, e o mesmo que o webhook do Stripe
// vai usar quando o pagamento existir de verdade: pagamento confirmado uma vez
// só (segunda chamada devolve 409) e entrega dentro da mesma transação.
func (h *ShopHandler) MarkOrderPaid(c *gin.Context) {
	orderID, ok := uuidParam(c, "orderId", "ID de pedido inválido")
	if !ok {
		return
	}

	var order domain.ShopOrder
	if err := h.DB.Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pedido não encontrado"})
		return
	}
	if order.IsPaid() {
		c.JSON(http.StatusConflict, gin.H{"error": "Este pedido já está pago"})
		return
	}
	if !order.IsPending() {
		c.JSON(http.StatusConflict, gin.H{"error": "Este pedido foi cancelado"})
		return
	}

	now := time.Now()
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// Trava a linha: duas confirmações simultaneas não podem passar
		// juntas e entregar o item em dobro.
		res := tx.Model(&domain.ShopOrder{}).
			Where("id = ? AND status = ?", order.ID, "pending").
			Updates(map[string]any{
				"status":     "paid",
				"paid_at":    now,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Outra requisição já confirmou entre o SELECT e o UPDATE.
			return errOrderAlreadyConfirmed
		}

		inventory := make([]domain.UserInventory, 0, len(order.Items))
		for _, line := range order.Items {
			for i := 0; i < line.Quantity; i++ {
				inventory = append(inventory, domain.UserInventory{
					UserID: order.UserID,
					ItemID: line.ItemID,
				})
			}
		}
		if len(inventory) == 0 {
			return nil
		}
		return tx.Create(&inventory).Error
	})
	if err == errOrderAlreadyConfirmed {
		c.JSON(http.StatusConflict, gin.H{"error": "Este pedido foi confirmado em outro pedido"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao confirmar o pagamento"})
		return
	}

	order.Status = "paid"
	order.PaidAt = &now

	// Categoria "shop": se o usuário desligou as novidades da loja, o serviço
	// de notificações descarta a mensagem.
	if h.NotificationService != nil {
		_ = h.NotificationService.CreateNotification(order.UserID, "Pedido confirmado!",
			"Recebemos o pagamento do seu pedido. Os itens já estão no seu inventário.", "shop")
	}

	c.JSON(http.StatusOK, orderDTO(order))
}

// ownedOrder carrega o pedido da rota garantindo que é do usuário logado. Já
// escreve a resposta de erro quando falha.
func (h *ShopHandler) ownedOrder(c *gin.Context, userID uuid.UUID) (*domain.ShopOrder, error) {
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de pedido inválido"})
		return nil, err
	}
	var order domain.ShopOrder
	if err := h.DB.Preload("Items").First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pedido não encontrado"})
		return nil, err
	}
	return &order, nil
}

func orderDTO(order domain.ShopOrder) dto.ShopOrderResponseDTO {
	items := make([]dto.ShopOrderItemDTO, 0, len(order.Items))
	for _, line := range order.Items {
		items = append(items, dto.ShopOrderItemDTO{
			ID:             line.ID,
			ItemID:         line.ItemID,
			Name:           line.Name,
			UnitPriceCents: line.UnitPriceCents,
			Quantity:       line.Quantity,
			LineTotalCents: line.UnitPriceCents * line.Quantity,
		})
	}
	return dto.ShopOrderResponseDTO{
		ID:          order.ID,
		Status:      order.Status,
		TotalCents:  order.TotalCents,
		CheckoutURL: order.CheckoutURL,
		PaidAt:      order.PaidAt,
		CanceledAt:  order.CanceledAt,
		Items:       items,
		CreatedAt:   order.CreatedAt,
	}
}

// ---------------------------------------------------------------- inventario

// GetUserInventory lista o que o usuário já possui: uma linha por unidade
// entregue por pedido pago.
func (h *ShopHandler) GetUserInventory(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var rows []domain.UserInventory
	if err := h.DB.Preload("Item").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar inventário"})
		return
	}

	// Item deletado pelo admin some da lista em vez de virar linha quebrada.
	out := make([]dto.InventoryItemDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.InventoryItemDTO{
			ID:         row.ID,
			ItemID:     row.ItemID,
			Name:       row.Item.Name,
			ImageURL:   row.Item.ImageURL,
			PriceCents: row.Item.PriceCents,
			Category:   row.Item.Category,
			Rating:     row.Item.Rating,
			AcquiredAt: row.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  out,
		"total": len(out),
	})
}

// ---------------------------------------------------------------- auxiliares

// uuidParam lê um parâmetro de rota como UUID, escrevendo a resposta de erro
// quando não é. Evita repetir a mesma checagem em cinco handlers.
func uuidParam(c *gin.Context, name, msg string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return uuid.Nil, false
	}
	return id, true
}

// keysOf devolve as chaves de um map[uuid.UUID]int como slice, para o "IN ?".
func keysOf(m map[uuid.UUID]int) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// seenItem procura o item entre as linhas já montadas do pedido.
func seenItem(items []domain.ShopOrderItem, id uuid.UUID) (int, bool) {
	for i, line := range items {
		if line.ItemID == id {
			return i, true
		}
	}
	return 0, false
}
