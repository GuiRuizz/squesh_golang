package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"squesh_golang/internal/domain"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PaymentService é a única porta de saída para o Stripe.
//
// Duas decisões de arquitetura que valem explicar:
//
//  1. O VALOR NUNCA VEM DO APP. O app manda um plan_id ou um order_id e o
//    preço é lido do banco aqui dentro. Se o valor viesse do cliente, daria
//    para pagar R$ 0,01 num plano de R$ 300: a Stripe cobra o que o
//    PaymentIntent diz, não o que a vitrine mostrou.
//
//  2. A ENTREGA NÃO ACONTECE NA RESPOSTA DA API. Criar o PaymentIntent
//    devolve um client_secret e o app confirma o cartão no Stripe; só depois
//    o webhook payment_intent.succeeded chama ConfirmOrderPaid. O caminho do
//    webhook é o MESMO do admin, então não existe uma segunda rota de entrega
//    capaz de divergir da primeira.
type PaymentService struct {
	DB     *gorm.DB
	Notif  *NotificationService
	stripe *stripe.Client

	publishableKey string
	webhookSecret  string
	currency       string
}

// Erros que o handler traduz em respostas HTTP próprias. O app olha o
// código, não a frase: "pagamento indisponível" (503) é chave faltando no
// servidor — algo que o usuário avisa no suporte — e não "seu cartão foi
// recusado".
var (
	ErrPaymentNotConfigured = errors.New("pagamento nao configurado no servidor")
	ErrPlanNotPriced        = errors.New("plano sem preco cadastrado no stripe")
	ErrNoSubscription       = errors.New("voce nao tem assinatura registrada no stripe")

	ErrOrderAlreadyPaid = errors.New("pedido ja pago")
	ErrOrderNotPending  = errors.New("pedido cancelado")
	ErrOrderEmptyTotal  = errors.New("pedido sem valor")
	ErrOrderNotFound    = errors.New("pedido nao encontrado")
)

// NewPaymentService lê a configuração do Stripe do ambiente.
//
// A chave secreta pode faltar de propósito: loja, vitrine de planos e perfil
// continuam funcionando e só as rotas de cobrança respondem 503. Isso
// importa em dois casos — desenvolvimento local sem conta Stripe, e o teste
// de interface, que não quer depender de rede externa.
func NewPaymentService(db *gorm.DB, notif *NotificationService) *PaymentService {
	s := &PaymentService{
		DB:             db,
		Notif:          notif,
		publishableKey: strings.TrimSpace(os.Getenv("STRIPE_PUBLISHABLE_KEY")),
		webhookSecret:  strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
		currency:       envOr("STRIPE_CURRENCY", "brl"),
	}
	if key := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")); key != "" {
		// Duas retentativas em erro de rede: a Stripe recusa o request logo
		// depois de criar o PaymentIntent, e um webhook reentregue por causa
		// disso perderia a entrega do produto.
		s.stripe = stripe.NewClient(key, stripe.WithBackends(
			stripe.NewBackendsWithConfig(&stripe.BackendConfig{
				MaxNetworkRetries: stripe.Int64(stripe.DefaultMaxNetworkRetries),
			}),
		))
	}
	return s
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// IsConfigured diz se dá para cobrar de verdade agora.
func (s *PaymentService) IsConfigured() bool { return s.stripe != nil }

// PublicConfig é o que o app precisa antes de mostrar qualquer formulário.
type PublicConfig struct {
	// Enabled diz ao app se ele deve mostrar "PAGAR" ou uma mensagem de
	// indisponibilidade. Sem isso o usuário descobre que não pode pagar só
	// depois de montar o cartão.
	Enabled bool `json:"enabled"`
	// PublishableKey é a chave que monta o formulário de cartão. Não é
	// segredo — a Stripe a publica em todo aplicativo —, mas mesmo assim vem
	// do servidor em vez do .env do app: assim trocar de conta (teste ->
	// produção) é uma variável no backend, não um rebuild.
	PublishableKey string `json:"publishable_key"`
	Currency       string `json:"currency"`
}

// Config monta a resposta de GET /payments/config.
func (s *PaymentService) Config() PublicConfig {
	return PublicConfig{
		Enabled:        s.IsConfigured() && s.publishableKey != "",
		PublishableKey: s.publishableKey,
		Currency:       s.currency,
	}
}

// Currency é a moeda em que tudo é cobrado.
func (s *PaymentService) Currency() string { return s.currency }

// ---------------------------------------------------------------- customer

// EnsureCustomer devolve o cus_xxx do usuário, criando um se ainda não existir.
//
// O cliente é criado uma vez e reaproveitado: é nele que o cartão fica salvo
// para as renovações. Criar um cliente novo a cada compra espalharia o
// histórico de cartões em vários objetos e o Portal não teria o que mostrar.
func (s *PaymentService) EnsureCustomer(userID string) (string, error) {
	if s.stripe == nil {
		return "", ErrPaymentNotConfigured
	}

	var user domain.User
	if err := s.DB.First(&user, "id = ?", userID).Error; err != nil {
		return "", err
	}
	if user.StripeCustomerID != "" {
		return user.StripeCustomerID, nil
	}

	cus, err := s.stripe.V1Customers.Create(context.Background(), &stripe.CustomerCreateParams{
		Email: stripe.String(user.Email),
		Name:  stripe.String(user.Name),
		// O metadata deixa o cus_xxx rastreável de volta para o usuário,
		// mesmo que um dia o cadastro seja corrigido e o email mude.
		Metadata: map[string]string{"squesh_user_id": userID},
	})
	if err != nil {
		return "", fmt.Errorf("criando cliente no stripe: %w", err)
	}

	if err := s.DB.Model(&domain.User{}).
		Where("id = ?", userID).
		Update("stripe_customer_id", cus.ID).Error; err != nil {
		return "", fmt.Errorf("salvando stripe_customer_id: %w", err)
	}
	return cus.ID, nil
}

// ErrPriceMismatch: o price_xxx existe, mas cobra um valor diferente do que
// a vitrine mostra.
var ErrPriceMismatch = errors.New("valor do price diverge do plano")

// ValidatePrice confere um price_xxx antes de ligá-lo a um plano.
//
// Existe porque o erro mais caro e mais silencioso desta integração é linkar o
// preço errado: a vitrine mostra R$ 29,90 e a Stripe cobra R$ 297, e o
// usuário só descobre no extrato do cartão — quando já não dá para corrigir
// sem estorno. Três coisas são conferidas, e cada uma cobre um erro real:
//
//   - o price existe e está ATIVO (um price arquivado não assina ninguém);
//   - é RECORRENTE (plano que cobra uma vez só não é assinatura);
//   - o valor em centavos bate com o que está no banco.
//
// A moeda também é conferida: um price em USD ligado a um plano em BRL é o
// mesmo erro de valor, com um fator de câmbio de brinde.
func (s *PaymentService) ValidatePrice(priceID string, expectedCents int, planName string) error {
	if s.stripe == nil {
		return ErrPaymentNotConfigured
	}

	price, err := s.stripe.V1Prices.Retrieve(context.Background(), priceID, nil)
	if err != nil {
		return fmt.Errorf("buscando price no stripe: %w", err)
	}
	if !price.Active {
		return fmt.Errorf("price %s esta arquivado no stripe", priceID)
	}
	if price.Recurring == nil {
		return fmt.Errorf("price %s nao e recorrente: plano %q e assinatura", priceID, planName)
	}
	if !strings.EqualFold(string(price.Currency), s.currency) {
		return fmt.Errorf("price %s esta em %s e o sistema cobra em %s", priceID, price.Currency, s.currency)
	}
	if int(price.UnitAmount) != expectedCents {
		return fmt.Errorf("%w: price cobra %d e o plano cobra %d", ErrPriceMismatch, price.UnitAmount, expectedCents)
	}
	return nil
}

// ---------------------------------------------------------------- pedido

// OrderPayment é o que o app precisa para cobrar um pedido.
type OrderPayment struct {
	ClientSecret string `json:"client_secret"`
	AmountCents  int    `json:"amount_cents"`
	Currency     string `json:"currency"`
}

// CreateOrderPaymentIntent abre a cobrança de um pedido.
//
// Idempotente por pedido: havendo um intent anterior ainda aproveitável, o
// client_secret dele é devolvido em vez de criar outro. Sem isso, cada toque
// em "PAGAR" — ou cada cartão recusado seguido de nova tentativa — deixaria
// um PaymentIntent órfão no painel da Stripe, e cobrar duas vezes no mesmo
// pedido é um reembolso para o usuário.
func (s *PaymentService) CreateOrderPaymentIntent(orderID string, userID string) (*OrderPayment, error) {
	if s.stripe == nil {
		return nil, ErrPaymentNotConfigured
	}

	var order domain.ShopOrder
	if err := s.DB.First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	switch {
	case order.IsPaid():
		return nil, ErrOrderAlreadyPaid
	case !order.IsPending():
		return nil, ErrOrderNotPending
	case order.TotalCents <= 0:
		return nil, ErrOrderEmptyTotal
	}

	if order.ExternalPaymentID != "" {
		pi, err := s.retrieveIntent(order.ExternalPaymentID)
		if err == nil && pi.ClientSecret != "" && pi.Status != stripe.PaymentIntentStatusCanceled {
			return &OrderPayment{
				ClientSecret: pi.ClientSecret,
				AmountCents:  order.TotalCents,
				Currency:     s.currency,
			}, nil
		}
		// Intent cancelado ou expirado: cai aqui e cria um novo.
	}

	customerID, err := s.EnsureCustomer(order.UserID.String())
	if err != nil {
		return nil, err
	}

	pi, err := s.stripe.V1PaymentIntents.Create(context.Background(), &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(int64(order.TotalCents)),
		Currency: stripe.String(s.currency),
		Customer: stripe.String(customerID),
		// Só cartão, e NÃO automatic_payment_methods.
		//
		// O automatic_payment_methods deixa a Stripe aceitar tudo que o Dashboard
		// tem ligado, e numa conta BR isso inclui Boleto, Pix e OXXO. Os que
		// redirecionam o pagador exigem return_url, e sem ela a confirmação
		// volta com "you must provide a return_url" — o app que só desenha
		// CardFormField não tem para onde redirecionar. Fixar payment_method_types
		// deixa o Intent cartão puro, que é o que o app sabe cobrar.
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		// O metadata é o elo entre o painel da Stripe e este banco: o webhook
		// descobre o pedido por aqui, sem depender de coluna sincronizada.
		Metadata: map[string]string{
			"kind":     "order",
			"order_id": order.ID.String(),
			"user_id":  order.UserID.String(),
		},
		Description: stripe.String("Squesh - pedido " + order.ID.String()[:8]),
	})
	if err != nil {
		return nil, fmt.Errorf("criando payment intent: %w", err)
	}

	if err := s.DB.Model(&domain.ShopOrder{}).
		Where("id = ?", order.ID).
		Updates(map[string]interface{}{
			"external_payment_id": pi.ID,
			"payment_provider":    "stripe",
		}).Error; err != nil {
		return nil, fmt.Errorf("salvando payment intent do pedido: %w", err)
	}

	return &OrderPayment{
		ClientSecret: pi.ClientSecret,
		AmountCents:  order.TotalCents,
		Currency:     s.currency,
	}, nil
}

// retrieveIntent busca um PaymentIntent já criado.
func (s *PaymentService) retrieveIntent(id string) (*stripe.PaymentIntent, error) {
	return s.stripe.V1PaymentIntents.Retrieve(context.Background(), id, nil)
}

// ConfirmOrderPaid entrega um pedido cujo pagamento a Stripe já confirmou.
//
// Só chega aqui depois de conferir na Stripe que o intent está "succeeded":
// um intent criado nunca é pago por existir, e o metadata do intent é quem
// diz qual pedido foi cobrado. A entrega em si é FulfillOrderPaid, o mesmo
// caminho da confirmação manual.
func (s *PaymentService) ConfirmOrderPaid(piID string) (*domain.ShopOrder, error) {
	if s.stripe == nil {
		return nil, ErrPaymentNotConfigured
	}

	pi, err := s.retrieveIntent(piID)
	if err != nil {
		return nil, fmt.Errorf("buscando payment intent: %w", err)
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded {
		return nil, fmt.Errorf("payment intent %s nao foi pago (status %s)", piID, pi.Status)
	}

	// O pedido vem do METADATA do intent e não de uma busca por
	// external_payment_id: a Stripe é a fonte da verdade sobre o que foi
	// cobrado, e o metadata é gravado junto com a cobrança.
	orderID := pi.Metadata["order_id"]
	if orderID == "" {
		return nil, errors.New("payment intent sem order_id no metadata")
	}
	id, err := uuid.Parse(orderID)
	if err != nil {
		return nil, fmt.Errorf("order_id invalido no metadata do intent: %w", err)
	}

	// A entrega em si é a MESMA função que a confirmação manual do admin usa
	// (FulfillOrderPaid). Um único caminho significa que uma correção futura
	// vale para os dois — o que não aconteceria com duas cópias.
	return FulfillOrderPaid(s.DB, s.Notif, id)
}

// ---------------------------------------------------------------- assinatura

// PlanSubscription é o que o app precisa para cobrar um plano.
type PlanSubscription struct {
	ClientSecret string
	ExternalID   string
	Subscription domain.UserSubscription
}

// CreatePlanSubscription abre a assinatura de um plano.
//
// O fluxo é o de "save payment details during subscription creation": a
// assinatura nasce INCOMPLETA (payment_behavior=default_incomplete), a Stripe
// gera a primeira fatura com um PaymentIntent, e o app confirma esse intent
// com o cartão. Só o webhook invoice.paid vira a linha local em "active".
// Marcar a assinatura ativa logo aqui seria o mesmo erro de entregar o
// pedido na resposta da API: o usuário receberia o plano sem pagar.
func (s *PaymentService) CreatePlanSubscription(userID, planID string) (*PlanSubscription, error) {
	if s.stripe == nil {
		return nil, ErrPaymentNotConfigured
	}

	var plan domain.Plan
	if err := s.DB.First(&plan, "id = ? AND is_active = true", planID).Error; err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	if plan.StripePriceID == "" {
		return nil, ErrPlanNotPriced
	}

	var user domain.User
	if err := s.DB.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}

	customerID, err := s.EnsureCustomer(userID)
	if err != nil {
		return nil, err
	}

	sub, err := s.stripe.V1Subscriptions.Create(context.Background(), &stripe.SubscriptionCreateParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionCreateItemParams{
			{Price: stripe.String(plan.StripePriceID)},
		},
		PaymentBehavior: stripe.String("default_incomplete"),
		PaymentSettings: &stripe.SubscriptionCreatePaymentSettingsParams{
			// Salva o cartão como padrão quando o pagamento passa: é o que
			// faz a RENOVAÇÃO automática funcionar sem o usuário estar com o
			// app aberto.
			SaveDefaultPaymentMethod: stripe.String("on_subscription"),
		},
		Metadata: map[string]string{
			"kind":    "subscription",
			"user_id": userID,
			"plan_id": planID,
		},
		// A fatura mais recente volta com o segredo de confirmação do
		// PaymentIntent, que é o que o app precisa confirmar. Sem este expand
		// a assinatura chega sem segredo e o app não tem o que cobrar.
		Expand: []*string{stripe.String("latest_invoice.confirmation_secret")},
	})
	if err != nil {
		return nil, fmt.Errorf("criando assinatura no stripe: %w", err)
	}

	// A linha local nasce "pending": existe para o webhook casar, mas não
	// conta como plano válido (IsCurrent só aceita active/canceled).
	now := time.Now()
	local := domain.UserSubscription{
		UserID:               user.ID,
		PlanID:               plan.ID,
		Status:               "pending",
		StartedAt:            now,
		RenewsAt:             now.AddDate(0, plan.PeriodMonths, 0),
		StripeSubscriptionID: sub.ID,
	}
	if err := s.DB.Create(&local).Error; err != nil {
		return nil, fmt.Errorf("salvando assinatura pendente: %w", err)
	}

	clientSecret := subscriptionClientSecret(sub)
	if clientSecret == "" {
		// A assinatura ficou criada na Stripe sem volta para cobrar. A linha
		// pendente não dá acesso a ninguém, e cancela-se na troca de plano
		// seguinte — é o motivo de a checagem vir antes de derrubar a
		// assinatura atual do usuário.
		return nil, errors.New("stripe nao devolveu o segredo da primeira fatura")
	}

	// Troca de plano: as assinaturas anteriores são encerradas só DEPOIS que a
	// nova existe e pode ser cobrada. Na ordem invertida, uma falha da
	// Stripe ao criar a nova deixaria o usuário sem plano nenhum — paying
	// mistake caro. Aqui o pior caso é o contrário: sobrar uma assinatura
	// antiga até a próxima renovação, e ainda sem cobrar nada dela.
	if err := s.cancelLocalSubscriptions(userID, local.ID); err != nil {
		return nil, err
	}

	local.Plan = plan
	return &PlanSubscription{
		ClientSecret: clientSecret,
		ExternalID:   sub.ID,
		Subscription: local,
	}, nil
}

// subscriptionClientSecret extrai o segredo de pagamento do que a Stripe
// devolveu.
//
// A partir da versão basil da API a fatura não expõe mais o campo
// payment_intent: o segredo vem em latest_invoice.confirmation_secret. Só
// esse caminho é lido — inventar um segundo (o campo antigo, que a versão
// atual não devolve) seria código que nunca roda e parece funcionar.
func subscriptionClientSecret(sub *stripe.Subscription) string {
	if sub.LatestInvoice == nil || sub.LatestInvoice.ConfirmationSecret == nil {
		return ""
	}
	return sub.LatestInvoice.ConfirmationSecret.ClientSecret
}

// ActivateSubscription marca a assinatura local como ativa.
//
// Chamado pelo webhook invoice.paid. Faz UPSERT na linha casada pelo
// stripe_subscription_id: RENOVAR NÃO ABRE LINHA NOVA, e é por isso que a
// coluna existe. Uma renovação que abrisse outra linha faria o app mostrar
// "duas assinaturas Pro" e o fim do período viria da linha errada.
func (s *PaymentService) ActivateSubscription(externalID string, userID, planID uuid.UUID) (*domain.UserSubscription, error) {
	now := time.Now()

	var plan domain.Plan
	if err := s.DB.First(&plan, "id = ?", planID).Error; err != nil {
		return nil, err
	}

	sub, err := s.stripe.V1Subscriptions.Retrieve(context.Background(), externalID, nil)
	if err != nil {
		return nil, fmt.Errorf("buscando assinatura no stripe: %w", err)
	}

	var local domain.UserSubscription
	err = s.DB.Where("stripe_subscription_id = ?", externalID).First(&local).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// A linha foi apagada ou nunca existiu (assinatura criada direto no
		// painel). Recria — o pagamento é a verdade.
		local = domain.UserSubscription{
			UserID:               userID,
			StripeSubscriptionID: externalID,
			StartedAt:            now,
		}
	case err != nil:
		return nil, err
	}

	// O fim do período vem do ITEM da assinatura: a partir da basil o
	// current_period_end migrou do objeto Subscription para o item. Usar o
	// campo antigo daria zero e a linha nunca "vigeria".
	renews := now.AddDate(0, plan.PeriodMonths, 0)
	if len(sub.Items.Data) > 0 && sub.Items.Data[0].CurrentPeriodEnd > 0 {
		renews = time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0)
	}

	local.PlanID = planID
	local.Status = "active"
	local.RenewsAt = renews
	local.CanceledAt = nil
	local.Plan = plan

	if err := s.DB.Save(&local).Error; err != nil {
		return nil, fmt.Errorf("ativando assinatura: %w", err)
	}
	return &local, nil
}

// MarkSubscriptionCanceled encerra a assinatura local de quem cancelou.
//
// Usado pelo app (botão cancelar) e pelo webhook. Cancelar não tira o acesso
// na hora: o período já pago continua valendo até RenewsAt, e é por isso que
// o status vira "canceled" em vez de a linha sumir.
func (s *PaymentService) MarkSubscriptionCanceled(userID, externalID string) error {
	now := time.Now()
	q := s.DB.Model(&domain.UserSubscription{}).
		Where("user_id = ? AND status = 'active'", userID)
	if externalID != "" {
		q = q.Where("stripe_subscription_id = ?", externalID)
	}
	return q.Updates(map[string]interface{}{
		"status":      "canceled",
		"canceled_at": now,
		"updated_at":  now,
	}).Error
}

// CancelSubscriptionWithStripe cancela a assinatura no Stripe.
//
// O que muda de verdade é que não haverá PRÓXIMA cobrança — que é o que o
// botão promete. A Stripe continua cobrando o período já pago.
func (s *PaymentService) CancelSubscriptionWithStripe(externalID string) error {
	if s.stripe == nil {
		return ErrPaymentNotConfigured
	}
	if externalID == "" {
		return ErrNoSubscription
	}
	_, err := s.stripe.V1Subscriptions.Update(context.Background(), externalID, &stripe.SubscriptionUpdateParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("cancelando assinatura no stripe: %w", err)
	}
	return nil
}

// cancelLocalSubscriptions encerra as linhas "pending" e "active" do usuário,
// e avisa a Stripe para não continuar gerando faturas das assinaturas antigas.
//
// exceptID é a linha que está sendo criada AGORA (troca de plano): sem essa
// exclusão o passo novo cancelaria a si mesmo logo depois de criá-lo.
func (s *PaymentService) cancelLocalSubscriptions(userID string, exceptID uuid.UUID) error {
	var subs []domain.UserSubscription
	if err := s.DB.Where("user_id = ? AND status IN ('active', 'pending')", userID).Find(&subs).Error; err != nil {
		return err
	}

	now := time.Now()
	for _, sub := range subs {
		if sub.ID == exceptID {
			continue
		}
		if sub.StripeSubscriptionID != "" {
			// Best effort: se a Stripe falhar aqui, a linha local é encerrada
			// assim mesmo e o cancelamento chega depois por webhook.
			_, _ = s.stripe.V1Subscriptions.Update(context.Background(), sub.StripeSubscriptionID, &stripe.SubscriptionUpdateParams{
				CancelAtPeriodEnd: stripe.Bool(true),
			})
		}
		if err := s.DB.Model(&domain.UserSubscription{}).Where("id = ?", sub.ID).
			Updates(map[string]interface{}{
				"status":      "canceled",
				"canceled_at": now,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

// SubscriptionDetails busca a assinatura na Stripe.
//
// Existe à parte de ActivateSubscription porque o webhook da fatura recebe a
// assinatura como referência — que vem só com o id — e precisa dos metadados
// (usuário e plano) que não vieram junto.
func (s *PaymentService) SubscriptionDetails(externalID string) (*stripe.Subscription, error) {
	if s.stripe == nil {
		return nil, ErrPaymentNotConfigured
	}
	return s.stripe.V1Subscriptions.Retrieve(context.Background(), externalID, nil)
}

// SyncSubscriptionStatus espelha o status da Stripe na linha local.
//
// Chamado nos eventos customer.subscription.updated e .deleted. Sem isto o
// app continuaria exibindo "Plano Pro ativo" depois de a assinatura ter sido
// cancelada direto no painel da Stripe.
func (s *PaymentService) SyncSubscriptionStatus(externalID string) error {
	if s.stripe == nil {
		return ErrPaymentNotConfigured
	}

	sub, err := s.stripe.V1Subscriptions.Retrieve(context.Background(), externalID, nil)
	if err != nil {
		return err
	}

	var local domain.UserSubscription
	if err := s.DB.Where("stripe_subscription_id = ?", externalID).First(&local).Error; err != nil {
		// Assinatura criada direto no painel da Stripe, sem passar pelo app:
		// não há linha local e isso não é erro.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	switch {
	case domain.CancelledSubscriptionStatus(string(sub.Status)):
		updates["status"] = "canceled"
		// CanceledAt e não EndedAt: com cancel_at_period_end o canceled_at
		// marca o pedido de cancelamento, que é o que o app mostra como
		// "você cancelou", enquanto ended_at só existe quando o período
		// acabou de fato.
		if local.CanceledAt == nil && sub.CanceledAt > 0 {
			updates["canceled_at"] = time.Unix(sub.CanceledAt, 0)
		}
	case !domain.IsSubscriptionStatus(string(sub.Status)):
		// incomplete: ainda pagando a primeira fatura. Segue pending.
		updates["status"] = "pending"
	default:
		updates["status"] = "active"
		updates["canceled_at"] = nil
		if len(sub.Items.Data) > 0 && sub.Items.Data[0].CurrentPeriodEnd > 0 {
			updates["renews_at"] = time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0)
		}
	}

	return s.DB.Model(&domain.UserSubscription{}).Where("id = ?", local.ID).Updates(updates).Error
}

// ---------------------------------------------------------------- portal

// CreatePortalSession abre a página de gerenciamento da assinatura.
//
// O Portal é da Stripe, não nosso: trocar cartão, ver faturas e cancelar
// cabem numa página hospedada que já funciona em qualquer navegador.
// Reproduzir essas telas aqui seria reimplementar o que a Stripe faz melhor —
// e, sem passar por ela, não teríamos como cobrar a nova mensalidade.
func (s *PaymentService) CreatePortalSession(userID, returnURL string) (string, error) {
	if s.stripe == nil {
		return "", ErrPaymentNotConfigured
	}

	var user domain.User
	if err := s.DB.First(&user, "id = ?", userID).Error; err != nil {
		return "", err
	}
	if user.StripeCustomerID == "" {
		return "", ErrNoSubscription
	}

	session, err := s.stripe.V1BillingPortalSessions.Create(context.Background(), &stripe.BillingPortalSessionCreateParams{
		Customer:  stripe.String(user.StripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	})
	if err != nil {
		return "", fmt.Errorf("criando sessao do portal: %w", err)
	}
	return session.URL, nil
}

// ---------------------------------------------------------------- webhook

// ErrWebhookUnconfigured: falta STRIPE_WEBHOOK_SECRET, então não dá para
// conferir a assinatura de nenhum evento. Aceitar sem conferir abriria porta
// para qualquer um se anunciar como a Stripe.
var ErrWebhookUnconfigured = errors.New("STRIPE_WEBHOOK_SECRET nao configurado")

// VerifyWebhook confere a assinatura do corpo recebido.
//
// Sem esta conferência, qualquer um que descobrisse a URL poderia mandar um
// POST falsificado com payment_intent.succeeded e ganhar produto de graça. A
// Stripe assina o corpo com STRIPE_WEBHOOK_SECRET e manda o resultado no
// header Stripe-Signature.
func (s *PaymentService) VerifyWebhook(payload []byte, sigHeader string) (*stripe.Event, error) {
	if s.webhookSecret == "" {
		return nil, ErrWebhookUnconfigured
	}
	event, err := webhook.ConstructEvent(payload, sigHeader, s.webhookSecret)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// MarkEventProcessed registra o evento como tratado.
//
// Devolve false quando o evento JÁ tinha sido processado — aí o handler
// responde 200 sem repetir o efeito. A gravação vem DEPOIS do efeito: se o
// efeito falhar, a Stripe reenvia e tentamos de novo, que é o comportamento
// desejado.
func (s *PaymentService) MarkEventProcessed(eventID, eventType string) (bool, error) {
	res := s.DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&domain.StripeEvent{ID: eventID, Type: eventType, CreatedAt: time.Now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}