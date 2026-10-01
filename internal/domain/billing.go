package domain

import (
	"time"

	"github.com/google/uuid"
)

// Plan é um plano de assinatura Pro oferecido na loja de planos.
// Preço em CENTAVOS (inteiro) para não errar arredondamento de float.
type Plan struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name         string    `gorm:"size:60;not null" json:"name"`
	Slug         string    `gorm:"size:40;uniqueIndex;not null" json:"slug"`
	Description  string    `gorm:"size:200" json:"description"`
	PriceCents   int       `gorm:"not null" json:"price_cents"`
	PeriodMonths int       `gorm:"not null;default:1" json:"period_months"`
	Badge        string    `gorm:"size:20" json:"badge"`
	// Features é a lista de benefícios exibida no cartão do plano (jsonb).
	Features    []string  `gorm:"type:jsonb;serializer:json" json:"features"`
	Highlight   string    `gorm:"size:80" json:"highlight"`
	IsPopular   bool      `gorm:"default:false" json:"is_popular"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	SortOrder   int       `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserSubscription é o plano que o usuário contratou. Só existe uma assinatura
// ATIVA por usuário: as antigas ficam com status "canceled" para o histórico.
type UserSubscription struct {
	ID         uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	PlanID     uuid.UUID  `gorm:"type:uuid;not null" json:"plan_id"`
	Plan       Plan       `gorm:"foreignKey:PlanID" json:"plan"`
	Status     string     `gorm:"size:20;default:'active';not null" json:"status"` // active | canceled
	StartedAt  time.Time  `json:"started_at"`
	RenewsAt   time.Time  `json:"renews_at"`
	CanceledAt *time.Time `json:"canceled_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// PaymentMethod é um cartão salvo. Guardamos só a bandeira e os 4 últimos
// dígitos — nunca o número completo.
type PaymentMethod struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Brand       string    `gorm:"size:30;not null" json:"brand"` // visa | mastercard | elo | amex
	Last4       string    `gorm:"size:4;not null" json:"last4"`
	ExpMonth    int       `gorm:"not null" json:"exp_month"`
	ExpYear     int       `gorm:"not null" json:"exp_year"`
	HolderName  string    `gorm:"size:100" json:"holder_name"`
	IsDefault   bool      `gorm:"default:false" json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Address é um endereço de entrega salvo no perfil.
type Address struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Label      string    `gorm:"size:40" json:"label"` // Casa, Trabalho...
	Recipient  string    `gorm:"size:100;not null" json:"recipient"`
	Street     string    `gorm:"size:150;not null" json:"street"`
	Number     string    `gorm:"size:20;not null" json:"number"`
	Complement string    `gorm:"size:120" json:"complement"`
	ZipCode    string    `gorm:"size:12;not null" json:"zip_code"`
	City       string    `gorm:"size:80;not null" json:"city"`
	State      string    `gorm:"size:2;not null" json:"state"`
	IsDefault  bool      `gorm:"default:false" json:"is_default"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Linha única ("Como você quer receber: 1234-5678, Rua das Flores, 90, Apto 42").
func (a Address) FullAddress() string {
	out := a.Street + ", " + a.Number
	if a.Complement != "" {
		out += " - " + a.Complement
	}
	return out + " - " + a.City + "/" + a.State
}

func (Plan) TableName() string { return "plans" }

// IsCurrent diz se o plano ainda vale AGORA.
//
// Cancelar não tira o acesso na hora: o usuário já pagou o período, então
// continua Pro até RenewsAt. Depois disso, uma assinatura cancelada deixa de
// valer — mas continua no histórico (status "canceled").
func (s UserSubscription) IsCurrent(now time.Time) bool {
	if s.Status == "active" {
		return true
	}
	return s.Status == "canceled" && s.RenewsAt.After(now)
}

func (UserSubscription) TableName() string { return "user_subscriptions" }

func (PaymentMethod) TableName() string { return "payment_methods" }

func (Address) TableName() string { return "addresses" }
