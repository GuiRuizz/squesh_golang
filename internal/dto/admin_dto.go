package dto

import (
	"time"

	"github.com/google/uuid"
)

type AdminOverviewDTO struct {
	ShopRevenueCents       int `json:"shop_revenue_cents"`
	PaidOrdersCount        int `json:"paid_orders_count"`
	PendingOrdersCount     int `json:"pending_orders_count"`
	CanceledOrdersCount    int `json:"canceled_orders_count"`
	ActiveSubscriptions    int `json:"active_subscriptions"`
	EstimatedMrrCents      int `json:"estimated_mrr_cents"`
	TrailCount             int `json:"trail_count"`
	ActiveCatalogItemCount int `json:"active_catalog_item_count"`
}

type AdminCustomerDTO struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type AdminAddressDTO struct {
	Recipient  string `json:"recipient"`
	Street     string `json:"street"`
	Number     string `json:"number"`
	Complement string `json:"complement"`
	ZipCode    string `json:"zip_code"`
	City       string `json:"city"`
	State      string `json:"state"`
	Label      string `json:"label"`
}

type AdminOrderDTO struct {
	ID          uuid.UUID          `json:"id"`
	Status      string             `json:"status"`
	TotalCents  int                `json:"total_cents"`
	PaidAt      *time.Time         `json:"paid_at"`
	CanceledAt  *time.Time         `json:"canceled_at"`
	CreatedAt   time.Time          `json:"created_at"`
	Items       []ShopOrderItemDTO `json:"items"`
	Customer    AdminCustomerDTO   `json:"customer"`
	Address     *AdminAddressDTO   `json:"address,omitempty"`
}

type AdminShopItemDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceCents  int       `json:"price_cents"`
	ImageURL    string    `json:"image_url"`
	Category    string    `json:"category"`
	Rating      float64   `json:"rating"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

type AdminSubscriptionDTO struct {
	ID         uuid.UUID       `json:"id"`
	Status     string          `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	RenewsAt   time.Time       `json:"renews_at"`
	CanceledAt *time.Time      `json:"canceled_at,omitempty"`
	IsCurrent  bool            `json:"is_current"`
	Plan       PlanResponseDTO `json:"plan"`
	Customer   AdminCustomerDTO `json:"customer"`
}
