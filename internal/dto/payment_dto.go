package dto

import (
	"github.com/google/uuid"
)

// ---- Configuração ----

// PaymentConfigDTO é lido pelo app ANTES de qualquer tela de pagamento: o
// publishable_key é o que permite montar o formulário de cartão do Stripe, e
// o enabled decide se o botão "PAGAR" existe ou se a tela avisa que o
// pagamento ainda não está disponível.
type PaymentConfigDTO struct {
	Enabled        bool   `json:"enabled"`
	PublishableKey string `json:"publishable_key"`
	Currency       string `json:"currency"`
}

// ---- Pagamento de pedido ----

// OrderPaymentDTO é o segredo de pagamento de UM pedido.
//
// ClientSecret vai para o app e a Stripe devolve o pagamento pronto. É seguro
// expor: ele só autoriza este intent, e o valor dele foi lido do banco pelo
// servidor — o app não escolhe quanto vale.
type OrderPaymentDTO struct {
	OrderID      uuid.UUID `json:"order_id"`
	ClientSecret string    `json:"client_secret"`
	AmountCents  int       `json:"amount_cents"`
	Currency     string    `json:"currency"`
}

// ---- Assinatura ----

// SubscribePaymentDTO é a resposta de POST /users/me/subscription.
//
// O plano NÃO vem ativo aqui: a assinatura é criada "pending" e só o webhook
// invoice.paid a ativa. Por isso a resposta carrega o segredo para o app
// confirmar o cartão e nada mais — o app mostra "processando" e só troca
// para "Pro" quando GET /users/me/subscription confirmar.
type SubscribePaymentDTO struct {
	ClientSecret string `json:"client_secret"`
	// Status segue o vocabulary do banco (pending | active | canceled) para o
	// app conseguir usar a mesma lógica de exibição dos outros lugares.
	Status string `json:"status"`
	// AmountCents vem do servidor para a tela mostrar o valor que será
	// cobrado, e não o que o app calculou.
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
}

// ---- Portal ----

// PortalSessionDTO carrega a URL do Billing Portal. O app abre no navegador
// externo: trocar cartão e ver faturas é uma tarefa de navegador, não uma
// tela que valha a pena recriar.
type PortalSessionDTO struct {
	URL string `json:"url"`
}

// ---- Admin ----

// SetPlanPriceDTO liga um plano a um price_xxx do Stripe.
//
// Existe como rota (e não como UPDATE genérico de plano) porque este é o
// único campo do plano que muda fora do app: o price_xxx nasce no painel da
// Stripe e alguém precisa colar aqui.
type SetPlanPriceDTO struct {
	StripePriceID string `json:"stripe_price_id" binding:"required,max=120"`
}