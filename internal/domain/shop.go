package domain

import (
	"time"

	"github.com/google/uuid"
)

type ShopItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	// Preço em CENTAVOS (inteiro), igual ao Plan. Guardar em float deixava
	// 119,90 susceptible a erro de arredondamento na soma do carrinho.
	PriceCents int    `gorm:"not null;default:0" json:"price_cents"`
	ImageURL   string `gorm:"type:text" json:"image_url"`
	// Category agrupa o item nos chips de filtro da loja ("Suplementos",
	// "Roupas", "Acessórios"). O app deriva os chips do catálogo, então não
	// existe tabela de categorias.
	Category string `gorm:"size:40;index" json:"category"`
	// Rating de 0 a 5. Zero significa "sem avaliação" — o app esconde as
	// estrelas nesse caso em vez de mostrar 0,0.
	Rating    float64   `gorm:"default:0" json:"rating"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// ShopOrder é o pedido do carrinho.
//
// Nasce com status "pending": nada entra no inventário enquanto o pagamento
// não for confirmado. A confirmação acontece por rota de admin hoje (é o que
// fecha o pedido nos testes e no desenvolvimento); quando o Stripe entrar, o
// mesmo efeito vem do webhook chamando o mesmo método.
type ShopOrder struct {
	ID     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	User   User      `gorm:"foreignKey:UserID" json:"-"`
	// Status: pending (aguardando pagamento) | paid | canceled.
	Status     string `gorm:"size:20;default:'pending';not null" json:"status"`
	TotalCents int    `gorm:"not null" json:"total_cents"`
	// CheckoutURL saiu: o pagamento acontece no app (Payment Intent + Stripe
	// Elements), não num redirect. A coluna continua no banco — removê-la
	// seria migração sem ganho —, mas nada a escreve nem a lê. O que liga o
	// pedido ao provedor são os dois campos abaixo.
	CheckoutURL string `gorm:"type:text" json:"-"`
	// PaymentProvider ("stripe") e ExternalPaymentID ligam o pedido ao
	// provedor. Vazios agora, preenchidos no checkout de verdade.
	PaymentProvider   string          `gorm:"size:20" json:"payment_provider"`
	ExternalPaymentID string          `gorm:"size:120" json:"external_payment_id"`
	PaidAt            *time.Time      `json:"paid_at"`
	CanceledAt        *time.Time      `json:"canceled_at"`
	Items             []ShopOrderItem `gorm:"foreignKey:OrderID" json:"items"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// ShopOrderItem é a linha do pedido. Nome e preço são COPIAS do momento da
// compra: se o admin mudar o preço ou renomear o item depois, o histórico do
// pedido continua mostrando o que foi realmente cobrado.
type ShopOrderItem struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID        uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	ItemID         uuid.UUID `gorm:"type:uuid;not null" json:"item_id"`
	Name           string    `gorm:"size:100;not null" json:"name"`
	UnitPriceCents int       `gorm:"not null" json:"unit_price_cents"`
	Quantity       int       `gorm:"not null" json:"quantity"`
}

// UserInventory é o item que o usuário já possui (entregue por um pedido pago).
type UserInventory struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	ItemID    uuid.UUID `gorm:"type:uuid;not null" json:"item_id"`
	Item      ShopItem  `gorm:"foreignKey:ItemID" json:"item"`
	CreatedAt time.Time `json:"created_at"`
}

// IsPaid diz se o pedido já foi pago e os itens já entraram no inventário.
func (o ShopOrder) IsPaid() bool { return o.Status == "paid" }

// IsPending diz se o pedido ainda espera pagamento (e pode ser cancelado).
func (o ShopOrder) IsPending() bool { return o.Status == "pending" }

func (ShopItem) TableName() string {
	return "shop_items"
}

func (UserInventory) TableName() string {
	return "user_inventory"
}

func (ShopOrder) TableName() string {
	return "shop_orders"
}

func (ShopOrderItem) TableName() string {
	return "shop_order_items"
}
