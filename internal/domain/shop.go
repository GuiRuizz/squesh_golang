package domain

import (
	"time"

	"github.com/google/uuid"
)

type ShopItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Price       float64   `gorm:"not null" json:"price"` // Preço em pontos/moedas
	ImageURL    string    `gorm:"type:text" json:"image_url"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserInventory struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	ItemID    uuid.UUID `gorm:"type:uuid;not null" json:"item_id"`
	Item      ShopItem  `gorm:"foreignKey:ItemID" json:"item"`
	CreatedAt time.Time `json:"created_at"`
}

func (ShopItem) TableName() string {
	return "shop_items"
}

func (UserInventory) TableName() string {
	return "user_inventory"
}