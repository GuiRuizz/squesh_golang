package domain

import "time"

// StripeEvent marca um evento do Stripe que JÁ foi processado.
//
// O webhook do Stripe NÃO entrega uma vez só: a Stripe reenvia o mesmo evento
// (mesmo evt_xxx) quando não recebe 200, e a ordem de entrega não é
// garantida. Sem esta tabela, um reenvio poderia entregar o item no
// inventário duas vezes. A chave primaria é o id do próprio evento, então o
// INSERT com DO NOTHING é o bloqueio: se ja existir, devolve 0 linhas e o
// handler responde 200 sem repetir o efeito.
type StripeEvent struct {
	ID        string    `gorm:"size:80;primaryKey" json:"id"`
	Type      string    `gorm:"size:60;not null" json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// IsSubscriptionStatus informa se um status do Stripe equivale a "o plano vale
// agora". O Stripe tem varios nomes para a mesma coisa (active, trialing,
// past_due ainda vale até a cobrança falhar de vez) e a assinatura aqui
// quando o pagamento da primeira fatura é confirmado.
func IsSubscriptionStatus(status string) bool {
	switch status {
	case "active", "trialing", "past_due", "unpaid":
		return true
	default:
		return false
	}
}

// CancelledSubscriptionStatus sao os status do Stripe que encerram o acesso.
func CancelledSubscriptionStatus(status string) bool {
	return status == "canceled" || status == "incomplete_expired"
}

// Registration do model: sem isto o GORM pluralizaria para stripe_events com
// o nome errado (e a tabela da arena ja ensina o caminho).
func (StripeEvent) TableName() string { return "stripe_events" }