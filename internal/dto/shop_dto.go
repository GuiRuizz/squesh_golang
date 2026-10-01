package dto

import (
	"time"

	"github.com/google/uuid"
)

// ---- Catálogo ----

// ShopItemResponseDTO é o item como o app consome. Preço sempre em centavos e
// rating já vem como número (o app formata em estrelas).
type ShopItemResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceCents  int       `json:"price_cents"`
	ImageURL    string    `json:"image_url"`
	Category    string    `json:"category"`
	Rating      float64   `json:"rating"`
}

// CreateShopItemDTO (Admin). Preço em centavos: 11990 = R$ 119,90.
type CreateShopItemDTO struct {
	Name        string  `json:"name" binding:"required,min=2,max=100"`
	Description string  `json:"description" binding:"omitempty,max=2000"`
	PriceCents  int     `json:"price_cents" binding:"gte=0"`
	ImageURL    string  `json:"image_url" binding:"omitempty,url"`
	Category    string  `json:"category" binding:"omitempty,max=40"`
	Rating      float64 `json:"rating" binding:"omitempty,gte=0,lte=5"`
}

// UpdateShopItemDTO (Admin). Ponteiros para permitir update parcial.
type UpdateShopItemDTO struct {
	Name        *string  `json:"name,omitempty" binding:"omitempty,min=3,max=100"`
	Description *string  `json:"description,omitempty" binding:"omitempty,max=2000"`
	PriceCents  *int     `json:"price_cents,omitempty" binding:"omitempty,gte=0"`
	ImageURL    *string  `json:"image_url,omitempty" binding:"omitempty,url"`
	Category    *string  `json:"category,omitempty" binding:"omitempty,max=40"`
	Rating      *float64 `json:"rating,omitempty" binding:"omitempty,gte=0,lte=5"`
	IsActive    *bool    `json:"is_active,omitempty"`
}

// ---- Pedido do carrinho ----

type OrderLineDTO struct {
	ItemID   uuid.UUID `json:"item_id" binding:"required"`
	Quantity int       `json:"quantity" binding:"required,min=1,max=99"`
}

// CreateOrderDTO é o checkout do carrinho. O cliente manda só o que quer e
// quanto: o PREÇO NUNCA vem do app — o servidor recalcula a partir do catálogo,
// senão dava para comprar qualquer coisa por R$ 0,01.
type CreateOrderDTO struct {
	Items []OrderLineDTO `json:"items" binding:"required,min=1,max=50,dive"`
}

type ShopOrderItemDTO struct {
	ID             uuid.UUID `json:"id"`
	ItemID         uuid.UUID `json:"item_id"`
	Name           string    `json:"name"`
	UnitPriceCents int       `json:"unit_price_cents"`
	Quantity       int       `json:"quantity"`
	LineTotalCents int       `json:"line_total_cents"`
}

type ShopOrderResponseDTO struct {
	ID          uuid.UUID          `json:"id"`
	Status      string             `json:"status"` // pending | paid | canceled
	TotalCents  int                `json:"total_cents"`
	CheckoutURL string             `json:"checkout_url"`
	PaidAt      *time.Time         `json:"paid_at"`
	CanceledAt  *time.Time         `json:"canceled_at"`
	Items       []ShopOrderItemDTO `json:"items"`
	CreatedAt   time.Time          `json:"created_at"`
}

// ---- Inventário ----

type InventoryItemDTO struct {
	ID         uuid.UUID `json:"id"`
	ItemID     uuid.UUID `json:"item_id"`
	Name       string    `json:"name"`
	ImageURL   string    `json:"image_url"`
	PriceCents int       `json:"price_cents"`
	Category   string    `json:"category"`
	Rating     float64   `json:"rating"`
	AcquiredAt time.Time `json:"acquired_at"`
}
