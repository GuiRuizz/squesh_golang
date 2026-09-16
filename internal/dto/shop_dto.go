package dto

import "github.com/google/uuid"

type PurchaseItemDTO struct {
	ItemID uuid.UUID `json:"item_id" binding:"required"`
}

type ShopItemResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	ImageURL    string    `json:"image_url"`
}

// Struct para criação de um novo item da loja (Admin)
type CreateShopItemDTO struct {
	Name        string `json:"name" binding:"required,min=2,max=100"`
	Description string `json:"description"`
	Price       int    `json:"price" binding:"required,gte=0"`
	ImageURL    string `json:"image_url"`
}

// Struct para atualização de um item (Admin)
type UpdateShopItemDTO struct {
	Name        *string  `json:"name,omitempty" binding:"omitempty,min=3,max=100"`
	Description *string  `json:"description,omitempty"`
	Price       *float64 `json:"price,omitempty" binding:"omitempty,gt=0"` // Use float64 e ponteiro para permitir updates parciais
	ImageURL    *string  `json:"image_url,omitempty" binding:"omitempty,url"`
	IsActive    *bool    `json:"is_active,omitempty"`
}