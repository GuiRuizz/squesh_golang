package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"squesh_golang/internal/dto"
	"squesh_golang/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"gorm.io/gorm"
)

// PaymentHandler junta as rotas que falam com o Stripe.
//
// Ficam juntas numa casa só porque são o mesmo par de dominios: a assinatura
// e o pedido compartilham cliente, moeda e webhook. A loja e os planos
// continuam nos handlers deles — quem chama estas rotas só precisa de "cobre
// isto", não de entender a vitrine.
type PaymentHandler struct {
	DB  *gorm.DB
	Pay *service.PaymentService
}

func NewPaymentHandler(db *gorm.DB, pay *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{DB: db, Pay: pay}
}

// ---------------------------------------------------------------- config

// GetConfig devolve o que o app precisa antes de mostrar pagamento.
//
// Rota PÚBLICA de propósito: o app lê isso no boot, e quem ainda não logou
// vê a vitrine de planos com os preços certainos. A chave é pública por
// definição — a Stripe manda a mesma chave no código de todo aplicativo.
func (h *PaymentHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.Pay.Config())
}

// ---------------------------------------------------------------- pedido

// PayOrder abre a cobrança de um pedido e devolve o segredo do pagamento.
//
// O corpo é vazio: o pedido já existe (POST /shop/orders) e o preço dele já
// está no banco. Chamar isto duas vezes devolve o mesmo segredo em vez de
// criar uma segunda cobrança — recarregar a tela não pode custar dinheiro ao
// usuário.
func (h *PaymentHandler) PayOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	orderID, ok := uuidParam(c, "orderId", "ID de pedido inválido")
	if !ok {
		return
	}

	payment, err := h.Pay.CreateOrderPaymentIntent(orderID.String(), userID.String())
	if err != nil {
		respondPaymentError(c, err, "Não foi possível iniciar o pagamento")
		return
	}

	c.JSON(http.StatusOK, dto.OrderPaymentDTO{
		OrderID:      orderID,
		ClientSecret: payment.ClientSecret,
		AmountCents:  payment.AmountCents,
		Currency:     payment.Currency,
	})
}

// respondPaymentError traduz os erros do PaymentService em status HTTP.
//
// O app decide o que mostrar a partir do status, então a tradução importa
// tanto quanto a mensagem: 503 é "o servidor não tem chave de Stripe" (algo
// que o usuário avisa no suporte), enquanto 409 é "este pedido já foi pago"
// (algo que o app resolve sozinho). Deixar tudo como 500 faria o app mostrar
// "tente de novo" numa situação que não passa com nova tentativa.
func respondPaymentError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrPaymentNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "O pagamento ainda não está disponível neste ambiente"})
	case errors.Is(err, service.ErrPlanNotPriced):
		// 422 e não 400: o pedido está bem formado, o plano é que ainda não
		// pode ser cobrado.
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Este plano ainda não está disponível para pagamento online"})
	case errors.Is(err, service.ErrNoSubscription):
		c.JSON(http.StatusNotFound, gin.H{"error": "Você não tem assinatura registrada"})
	case errors.Is(err, service.ErrOrderAlreadyPaid):
		c.JSON(http.StatusConflict, gin.H{"error": "Este pedido já está pago"})
	case errors.Is(err, service.ErrOrderNotPending):
		c.JSON(http.StatusConflict, gin.H{"error": "Este pedido foi cancelado"})
	case errors.Is(err, service.ErrOrderEmptyTotal):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Este pedido não tem valor a cobrar"})
	case errors.Is(err, service.ErrOrderNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Pedido não encontrado"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

// ---------------------------------------------------------------- assinatura

// Subscribe abre a cobrança de um plano.
//
// NÃO ativa o plano: a assinatura é criada "pending" no Stripe e aqui, o app
// confirma o cartão, e o webhook invoice.paid é quem vira a linha em
// "active". A troca de plano também acontece por aqui — a assinatura anterior
// é encerrada no mesmo passo.
func (h *PaymentHandler) Subscribe(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input dto.SubscribeDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id é obrigatório"})
		return
	}

	sub, err := h.Pay.CreatePlanSubscription(userID.String(), input.PlanID.String())
	if err != nil {
		respondPaymentError(c, err, "Não foi possível iniciar a assinatura")
		return
	}

	// AmountCents vem do Plan lido no servidor, não do app: a tela mostra
	// o valor que a Stripe vai cobrar.
	c.JSON(http.StatusCreated, dto.SubscribePaymentDTO{
		ClientSecret: sub.ClientSecret,
		Status:       sub.Subscription.Status,
		AmountCents:  sub.Subscription.Plan.PriceCents,
		Currency:     h.Pay.Currency(),
	})
}

// CreatePortalSession abre a página de gerenciamento da assinatura.
//
// URL de uso único e válida por pouco tempo, então é devolvida e não
// guardada: o app abre no navegador e volta pelo ReturnURL. Cancelamento e
// troca de cartão ficam com a Stripe, que precisa estar envolvida na
// cobrança recorrente de qualquer jeito.
func (h *PaymentHandler) CreatePortalSession(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input dto.PortalSessionDTO
	_ = c.ShouldBindJSON(&input)

	// Sem return_url o navegador não saberia voltar ao app. O padrão é a raiz
	// do app: o go_router devolve para a home, e o usuário refaz o caminho.
	returnURL := input.URL
	if returnURL == "" {
		returnURL = c.GetHeader("Origin")
	}
	if returnURL == "" {
		returnURL = "https://localhost"
	}

	url, err := h.Pay.CreatePortalSession(userID.String(), returnURL)
	if err != nil {
		respondPaymentError(c, err, "Não foi possível abrir o portal de assinatura")
		return
	}

	c.JSON(http.StatusOK, dto.PortalSessionDTO{URL: url})
}

// ---------------------------------------------------------------- webhook

// Webhook recebe os eventos do Stripe.
//
// Três coisas que não são negociáveis aqui:
//
//  1. NADA de autenticação por token. A Stripe não manda token nosso; ela
//     assina o corpo. Por isso a rota fica fora do grupo protegido e a
//     autenticação é a assinatura do header Stripe-Signature.
//  2. O corpo é lido CRU. O Gin já consumiu o body para o binding JSON, e a
//     assinatura é sobre os bytes exatos — um []byte re-marshaled não
//     valida. Por isso a rota não usa ShouldBindJSON em lugar nenhum.
//  3. O corpo cru precisa de limite de tamanho: sem ele, qualquer um pode
//     mandar gigabytes e o servidor fica sem memória.
func (h *PaymentHandler) Webhook(c *gin.Context) {
	const maxBody = 1 << 20 // 1 MB: evento da Stripe maior que isto é abuso

	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Corpo do webhook ilegível"})
		return
	}

	event, err := h.Pay.VerifyWebhook(payload, c.GetHeader("Stripe-Signature"))
	if err != nil {
		// 400 e não 401/403: um corpo com assinatura inválida é malformado do
		// ponto de vista do endpoint. O mais importante é NÃO processar nada.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Assinatura do webhook inválida"})
		return
	}

	if err := h.handleStripeEvent(event); err != nil {
		// 500 faz a Stripe REENTREGAR o evento. É o que queremos quando o
		// efeito falhou: o pagamento foi feito e o usuário espera o produto.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao processar o evento"})
		return
	}

	// A marca de "já processado" vem DEPOIS do efeito, nunca antes. Marcando
	// antes, uma falha no meio do caminho deixaria o evento registrado como
	// tratado e a reentrega seria ignorada — pagamento feito, produto nunca
	// entregue. Como os efeitos em si já são idempotentes, gravar depois só
	// evita trabalho repetido, sem risco de perder um pagamento.
	if _, err := h.Pay.MarkEventProcessed(event.ID, string(event.Type)); err != nil {
		// Falhou só o registro da auditoria; o efeito já aconteceu e não há
		// o que desfazer. Responder 200 evita uma reentrega inútil.
		_ = err
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

// handleStripeEvent aplica o efeito de um evento.
//
// Devolve nil para evento desconhecido de propósito: a Stripe manda dezenas
// de tipos e responder erro para um que não nos interessa faz ela reentregar
// para sempre, gastando cota e poluindo o log. O que não nos interessa só
// precisa ser reconhecido como recebido.
func (h *PaymentHandler) handleStripeEvent(event *stripe.Event) error {
	if event.Data == nil || len(event.Data.Raw) == 0 {
		return nil
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var intent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
			return fmt.Errorf("lendo payment intent do evento: %w", err)
		}
		// Só pedidos: a assinatura também passa por PaymentIntent na
		// primeira cobrança, e tratá-la aqui entregaria inventário do nada.
		if intent.Metadata["kind"] != "order" {
			return nil
		}
		_, err := h.Pay.ConfirmOrderPaid(intent.ID)
		// Já pago não é falha: o reenvio do mesmo evento é esperado e a
		// entrega já aconteceu. Devolver erro aqui faria a Stripe reentregar
		// para sempre.
		if errors.Is(err, service.ErrOrderAlreadyPaid) {
			return nil
		}
		return err

	case "invoice.paid":
		var invoice stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
			return fmt.Errorf("lendo invoice do evento: %w", err)
		}
		return h.activateSubscriptionFromInvoice(&invoice)

	case "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return fmt.Errorf("lendo subscription do evento: %w", err)
		}
		return h.Pay.SyncSubscriptionStatus(sub.ID)

	default:
		return nil
	}
}

// activateSubscriptionFromInvoice liga a fatura paga à linha local.
//
// Quem casa é a ASSINATURA (não a fatura): a linha é aberta/renovada pelo
// stripe_subscription_id, e é por isso que a renovação estende a linha
// existente em vez de criar uma segunda. O metadata da assinatura é lido
// daqui para achar de qual usuário e plano se trata.
func (h *PaymentHandler) activateSubscriptionFromInvoice(invoice *stripe.Invoice) error {
	details := invoice.Parent
	if details == nil || details.SubscriptionDetails == nil {
		// Fatura avulsa, sem assinatura por trás: nada a ativar.
		return nil
	}

	// O id da assinatura vem do próprio objeto da fatura, não do metadata: o
	// metadata é uma CÓPIA do que a assinatura tinha quando a fatura foi
	// gerada, então pode estar velho se a assinatura foi trocada no meio do
	// ciclo. A referência do objeto é o caminho direto e não envelhece.
	sub := details.SubscriptionDetails.Subscription
	if sub == nil || sub.ID == "" {
		return nil
	}

	// Já vem preenchido pela Stripe no expand implícito do webhook; quando
	// vier vazio, é preciso pedir a assinatura para ler os ids.
	if len(sub.Items.Data) == 0 || sub.Metadata["user_id"] == "" {
		full, err := h.Pay.SubscriptionDetails(sub.ID)
		if err != nil {
			return err
		}
		sub = full
	}

	userID, err := uuid.Parse(sub.Metadata["user_id"])
	if err != nil {
		// Assinatura criada direto no painel da Stripe: não é nossa para
		// ativar, e não é erro do webhook.
		return nil
	}
	planID, err := uuid.Parse(sub.Metadata["plan_id"])
	if err != nil {
		return nil
	}

	if _, err := h.Pay.ActivateSubscription(sub.ID, userID, planID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A fatura é de um plano que saiu do catálogo. Não é erro do
			// webhook: o usuário pagou algo que existia quando pagou.
			return nil
		}
		return err
	}
	return nil
}