package dto

import (
	"time"

	"github.com/google/uuid"
)

// ---- Planos ----

type PlanResponseDTO struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description"`
	PriceCents   int       `json:"price_cents"`
	PeriodMonths int       `json:"period_months"`
	Badge        string    `json:"badge"`
	Features     []string  `json:"features"`
	Highlight    string    `json:"highlight"`
	IsPopular    bool      `json:"is_popular"`
}

// ---- Assinatura ----

type SubscribeDTO struct {
	PlanID uuid.UUID `json:"plan_id" binding:"required"`
}

// SubscriptionResponseDTO embrulha a assinatura porque pode não existir ainda:
// o app distingue "sem assinatura" (subscription = null) de "assinada".
type SubscriptionResponseDTO struct {
	Subscription *UserSubscriptionDTO `json:"subscription"`
}

type UserSubscriptionDTO struct {
	ID         uuid.UUID      `json:"id"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	RenewsAt   time.Time      `json:"renews_at"`
	CanceledAt *time.Time     `json:"canceled_at,omitempty"`
	Plan       PlanResponseDTO `json:"plan"`
	// IsCurrent = o plano ainda vale agora. Uma assinatura cancelada continua
	// valendo até RenewsAt, então o app não pode usar Status == "active" para
	// decidir se mostra "PRO" — precisa deste campo.
	IsCurrent bool `json:"is_current"`
}

// ---- Formas de pagamento ----

type CreatePaymentMethodDTO struct {
	// CardNumber chega completo, mas só os 4 últimos dígitos são gravados
	// (ver PaymentMethod no domain).
	CardNumber string `json:"card_number" binding:"required,min=13,max=19"`
	ExpMonth   int    `json:"exp_month" binding:"required,min=1,max=12"`
	ExpYear    int    `json:"exp_year" binding:"required,min=2024,max=2099"`
	HolderName string `json:"holder_name" binding:"omitempty,max=100"`
	IsDefault  bool   `json:"is_default"`
}

type UpdatePaymentMethodDTO struct {
	ExpMonth   int    `json:"exp_month" binding:"omitempty,min=1,max=12"`
	ExpYear    int    `json:"exp_year" binding:"omitempty,min=2024,max=2099"`
	HolderName string `json:"holder_name" binding:"omitempty,max=100"`
	IsDefault  *bool  `json:"is_default"`
}

// ---- Endereços ----

type CreateAddressDTO struct {
	Label      string `json:"label" binding:"omitempty,max=40"`
	Recipient  string `json:"recipient" binding:"required,max=100"`
	Street     string `json:"street" binding:"required,max=150"`
	Number     string `json:"number" binding:"required,max=20"`
	Complement string `json:"complement" binding:"omitempty,max=120"`
	ZipCode    string `json:"zip_code" binding:"required,max=12"`
	City       string `json:"city" binding:"required,max=80"`
	State      string `json:"state" binding:"required,len=2"`
	IsDefault  bool   `json:"is_default"`
}

type UpdateAddressDTO struct {
	Label      *string `json:"label" binding:"omitempty,max=40"`
	Recipient  *string `json:"recipient" binding:"omitempty,max=100"`
	Street     *string `json:"street" binding:"omitempty,max=150"`
	Number     *string `json:"number" binding:"omitempty,max=20"`
	Complement *string `json:"complement" binding:"omitempty,max=120"`
	ZipCode    *string `json:"zip_code" binding:"omitempty,max=12"`
	City       *string `json:"city" binding:"omitempty,max=80"`
	State      *string `json:"state" binding:"omitempty,len=2"`
	IsDefault  *bool   `json:"is_default"`
}
