package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdminHandler struct {
	DB *gorm.DB
}

func NewAdminHandler(db *gorm.DB) *AdminHandler {
	return &AdminHandler{DB: db}
}

func (h *AdminHandler) Overview(c *gin.Context) {
	now := time.Now()
	out := dto.AdminOverviewDTO{}

	out.PaidOrdersCount = countWhere(h.DB, &domain.ShopOrder{}, "status = ?", "paid")
	out.PendingOrdersCount = countWhere(h.DB, &domain.ShopOrder{}, "status = ?", "pending")
	out.CanceledOrdersCount = countWhere(h.DB, &domain.ShopOrder{}, "status = ?", "canceled")
	out.TrailCount = countWhere(h.DB, &domain.Trail{}, "", nil)
	out.ActiveCatalogItemCount = countWhere(h.DB, &domain.ShopItem{}, "is_active = ?", true)

	h.DB.Model(&domain.ShopOrder{}).Where("status = ?", "paid").
		Select("COALESCE(SUM(total_cents), 0)").Scan(&out.ShopRevenueCents)

	var subs []domain.UserSubscription
	if err := h.DB.Preload("Plan").
		Where("status IN ('active', 'canceled') AND renews_at > ?", now).
		Order("CASE WHEN status = 'active' THEN 0 ELSE 1 END, started_at DESC").
		Find(&subs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular assinaturas"})
		return
	}

	seen := map[uuid.UUID]bool{}
	for _, sub := range subs {
		if seen[sub.UserID] || !sub.IsCurrent(now) {
			continue
		}
		seen[sub.UserID] = true
		out.ActiveSubscriptions++
		if sub.Plan.PeriodMonths > 0 {
			out.EstimatedMrrCents += sub.Plan.PriceCents / sub.Plan.PeriodMonths
		}
	}

	c.JSON(http.StatusOK, out)
}

func (h *AdminHandler) ListOrders(c *gin.Context) {
	status := c.Query("status")
	page, limit := pageLimit(c, 20)

	query := h.DB.Model(&domain.ShopOrder{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao contar pedidos"})
		return
	}

	var orders []domain.ShopOrder
	if err := query.Preload("Items").Preload("User").
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao listar pedidos"})
		return
	}

	userIDs := make([]uuid.UUID, 0, len(orders))
	for _, order := range orders {
		userIDs = append(userIDs, order.UserID)
	}

	addresses := map[uuid.UUID]domain.Address{}
	if len(userIDs) > 0 {
		var rows []domain.Address
		h.DB.Where("user_id IN ? AND is_default = ?", userIDs, true).Find(&rows)
		for _, row := range rows {
			addresses[row.UserID] = row
		}
	}

	out := make([]dto.AdminOrderDTO, 0, len(orders))
	for _, order := range orders {
		item := adminOrderDTO(order)
		if addr, ok := addresses[order.UserID]; ok {
			item.Address = &dto.AdminAddressDTO{
				Recipient:  addr.Recipient,
				Street:     addr.Street,
				Number:     addr.Number,
				Complement: addr.Complement,
				ZipCode:    addr.ZipCode,
				City:       addr.City,
				State:      addr.State,
				Label:      addr.Label,
			}
		}
		out = append(out, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"total_items": total,
			"page":        page,
			"limit":       limit,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

func (h *AdminHandler) ListShopItems(c *gin.Context) {
	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	page, limit := pageLimit(c, 20)

	query := h.DB.Model(&domain.ShopItem{})
	if search != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao contar itens"})
		return
	}

	var items []domain.ShopItem
	if err := query.Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao listar o catálogo"})
		return
	}

	out := make([]dto.AdminShopItemDTO, 0, len(items))
	for _, item := range items {
		out = append(out, dto.AdminShopItemDTO{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			PriceCents:  item.PriceCents,
			ImageURL:    item.ImageURL,
			Category:    item.Category,
			Rating:      item.Rating,
			IsActive:    item.IsActive,
			CreatedAt:   item.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"total_items": total,
			"page":        page,
			"limit":       limit,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

func (h *AdminHandler) ListSubscriptions(c *gin.Context) {
	now := time.Now()
	var subs []domain.UserSubscription
	if err := h.DB.Preload("Plan").Preload("User").
		Where("status IN ('active', 'canceled') AND renews_at > ?", now).
		Order("CASE WHEN status = 'active' THEN 0 ELSE 1 END, started_at DESC").
		Find(&subs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao listar assinaturas"})
		return
	}

	seen := map[uuid.UUID]bool{}
	out := make([]dto.AdminSubscriptionDTO, 0, len(subs))
	for _, sub := range subs {
		if seen[sub.UserID] || !sub.IsCurrent(now) {
			continue
		}
		seen[sub.UserID] = true
		out = append(out, dto.AdminSubscriptionDTO{
			ID:         sub.ID,
			Status:     sub.Status,
			StartedAt:  sub.StartedAt,
			RenewsAt:   sub.RenewsAt,
			CanceledAt: sub.CanceledAt,
			IsCurrent:  true,
			Plan:       planDTO(sub.Plan),
			Customer: dto.AdminCustomerDTO{
				ID:    sub.UserID,
				Name:  sub.User.Name,
				Email: sub.User.Email,
			},
		})
	}

	c.JSON(http.StatusOK, out)
}

func adminOrderDTO(order domain.ShopOrder) dto.AdminOrderDTO {
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
	return dto.AdminOrderDTO{
		ID:         order.ID,
		Status:     order.Status,
		TotalCents: order.TotalCents,
		PaidAt:     order.PaidAt,
		CanceledAt: order.CanceledAt,
		CreatedAt:  order.CreatedAt,
		Items:      items,
		Customer: dto.AdminCustomerDTO{
			ID:    order.UserID,
			Name:  order.User.Name,
			Email: order.User.Email,
		},
	}
}

func countWhere(db *gorm.DB, model any, query string, args ...any) int {
	var total int64
	q := db.Model(model)
	if query != "" {
		q = q.Where(query, args...)
	}
	_ = q.Count(&total)
	return int(total)
}

func pageLimit(c *gin.Context, fallback int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(fallback)))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = fallback
	}
	return page, limit
}
