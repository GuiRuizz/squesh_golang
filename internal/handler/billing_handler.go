package handler

import (
	"net/http"
	"strings"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"
	"squesh_golang/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BillingHandler cuida de planos, assinatura, cartões e endereços.
type BillingHandler struct {
	DB    *gorm.DB
	Notif *service.NotificationService
}

func NewBillingHandler(db *gorm.DB, notif *service.NotificationService) *BillingHandler {
	return &BillingHandler{DB: db, Notif: notif}
}

// ---------------------------------------------------------------- planos

// ListPlans devolve o catálogo de planos ativos. É público: a vitrine de planos
// não exige login.
func (h *BillingHandler) ListPlans(c *gin.Context) {
	var plans []domain.Plan
	if err := h.DB.Where("is_active = true").Order("sort_order asc, price_cents asc").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar planos"})
		return
	}

	out := make([]dto.PlanResponseDTO, 0, len(plans))
	for _, p := range plans {
		out = append(out, planDTO(p))
	}
	c.JSON(http.StatusOK, out)
}

func planDTO(p domain.Plan) dto.PlanResponseDTO {
	features := p.Features
	if features == nil {
		features = []string{}
	}
	return dto.PlanResponseDTO{
		ID:           p.ID,
		Name:         p.Name,
		Slug:         p.Slug,
		Description:  p.Description,
		PriceCents:   p.PriceCents,
		PeriodMonths: p.PeriodMonths,
		Badge:        p.Badge,
		Features:     features,
		Highlight:    p.Highlight,
		IsPopular:    p.IsPopular,
	}
}

// ---------------------------------------------------------------- assinatura

// GetMySubscription devolve a assinatura em vigor, ou subscription = null.
//
// "Em vigor" inclui a cancelada que ainda não venceu: cancelar não encerra o
// período já pago, só impede a renovação. O campo is_current diz ao app se o
// plano ainda está valendo.
func (h *BillingHandler) GetMySubscription(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	sub, err := h.currentSubscription(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar assinatura"})
		return
	}

	resp := dto.SubscriptionResponseDTO{}
	if sub != nil {
		d := subscriptionDTO(*sub)
		resp.Subscription = &d
	}
	c.JSON(http.StatusOK, resp)
}

// currentSubscription devolve a assinatura EM VIGOR do usuário, ou nil.
//
// Só existe uma linha por vez em "active"; as canceladas ainda dentro do
// período entram como fallback. Uma cancelada vencida some do retorno — foi o
// que aconteceu quando o filtro exigia status = 'active': o app passava a
// dizer "plano gratuito" no mesmo instante do cancelamento, contradizendo o
// que a própria tela de cancelamento prometia.
func (h *BillingHandler) currentSubscription(userID uuid.UUID) (*domain.UserSubscription, error) {
	return h.oneSubscription(userID, "status IN ('active', 'canceled')")
}

// renewableSubscription devolve só a assinatura que ainda pode ser cancelada
// (status "active"). Não usa o fallback das canceladas: cancelar duas vezes
// não é a mesma coisa que cancelar, e a segunda chamada precisa dizer que não
// há nada ativo para encerrar.
func (h *BillingHandler) renewableSubscription(userID uuid.UUID) (*domain.UserSubscription, error) {
	return h.oneSubscription(userID, "status = 'active'")
}

func (h *BillingHandler) oneSubscription(userID uuid.UUID, statusFilter string) (*domain.UserSubscription, error) {
	now := time.Now()

	var sub domain.UserSubscription
	err := h.DB.Preload("Plan").
		Where("user_id = ? AND "+statusFilter+" AND renews_at > ?", userID, now).
		Order("CASE WHEN status = 'active' THEN 0 ELSE 1 END").
		First(&sub).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// Subscribe contrata (ou troca) o plano. A assinatura anterior é cancelada na
// mesma transação, então nunca existem duas ativas.
func (h *BillingHandler) Subscribe(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input dto.SubscribeDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id é obrigatório"})
		return
	}

	var plan domain.Plan
	if err := h.DB.First(&plan, "id = ? AND is_active = true", input.PlanID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plano não encontrado"})
		return
	}

	now := time.Now()
	renews := now.AddDate(0, plan.PeriodMonths, 0)
	sub := domain.UserSubscription{
		UserID:    userID,
		PlanID:    plan.ID,
		Status:    "active",
		StartedAt: now,
		RenewsAt:  renews,
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.UserSubscription{}).
			Where("user_id = ? AND status = 'active'", userID).
			Updates(map[string]any{
				"status":      "canceled",
				"canceled_at": now,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		return tx.Create(&sub).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao ativar assinatura"})
		return
	}

	sub.Plan = plan
	if h.Notif != nil {
		// Categoria "shop": se o usuário desligou as novidades da loja, o
		// serviço de notificações descarta a mensagem.
		_ = h.Notif.CreateNotification(userID, "Assinatura ativada",
			"Voce esta no plano "+plan.Name+". A renovacao acontece em "+renews.Format("02/01/2006")+".", "shop")
	}

	c.JSON(http.StatusCreated, subscriptionDTO(sub))
}

// CancelSubscription encerra a assinatura. O usuário continua Pro até o fim do
// período já pago (a data de renovação) — por isso o cancelamento marca
// status = "canceled" em vez de apagar a linha.
//
// Cancelar de novo devolve 404: a segunda chamada não tem nada ativo para
// encerrar, e fingir que cancelou de novo esconderia do app que o botão
// deveria estar desabilitado.
func (h *BillingHandler) CancelSubscription(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	sub, err := h.renewableSubscription(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar assinatura"})
		return
	}
	if sub == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Voce nao tem assinatura ativa"})
		return
	}

	now := time.Now()
	if err := h.DB.Model(sub).Updates(map[string]any{
		"status":      "canceled",
		"canceled_at": now,
		"updated_at":  now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao cancelar assinatura"})
		return
	}

	sub.Status = "canceled"
	sub.CanceledAt = &now
	c.JSON(http.StatusOK, subscriptionDTO(*sub))
}

func subscriptionDTO(s domain.UserSubscription) dto.UserSubscriptionDTO {
	return dto.UserSubscriptionDTO{
		ID:         s.ID,
		Status:     s.Status,
		StartedAt:  s.StartedAt,
		RenewsAt:   s.RenewsAt,
		CanceledAt: s.CanceledAt,
		Plan:       planDTO(s.Plan),
		IsCurrent:  s.IsCurrent(time.Now()),
	}
}

// ---------------------------------------------------------------- cartoes

func (h *BillingHandler) ListPaymentMethods(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var methods []domain.PaymentMethod
	if err := h.DB.Where("user_id = ?", userID).
		Order("is_default desc, created_at asc").Find(&methods).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar cartoes"})
		return
	}
	if methods == nil {
		methods = []domain.PaymentMethod{}
	}
	c.JSON(http.StatusOK, methods)
}

func (h *BillingHandler) CreatePaymentMethod(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input dto.CreatePaymentMethodDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	digits := onlyDigits(input.CardNumber)
	if len(digits) < 13 || len(digits) > 19 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Numero de cartao invalido"})
		return
	}
	brand := cardBrand(digits)
	if brand == "outro" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bandeira de cartao nao suportada"})
		return
	}
	if input.ExpMonth < 1 || input.ExpMonth > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Mes de validade invalido"})
		return
	}

	// Cartao vencido nao entra. Aceitamos o mes corrente inteiro.
	now := time.Now()
	if input.ExpYear < now.Year() || (input.ExpYear == now.Year() && input.ExpMonth < int(now.Month())) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cartao vencido"})
		return
	}

	var total int64
	h.DB.Model(&domain.PaymentMethod{}).Where("user_id = ?", userID).Count(&total)
	method := domain.PaymentMethod{
		UserID:     userID,
		Brand:      brand,
		Last4:      digits[len(digits)-4:],
		ExpMonth:   input.ExpMonth,
		ExpYear:    input.ExpYear,
		HolderName: strings.TrimSpace(input.HolderName),
		// O primeiro cartao entra como padrao automaticamente.
		IsDefault: input.IsDefault || total == 0,
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if method.IsDefault {
			if err := tx.Model(&domain.PaymentMethod{}).Where("user_id = ?", userID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&method).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao salvar cartao"})
		return
	}

	c.JSON(http.StatusCreated, method)
}

func (h *BillingHandler) UpdatePaymentMethod(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var method domain.PaymentMethod
	if err := h.ownedPaymentMethod(c, &method, userID); err != nil {
		return
	}

	var input dto.UpdatePaymentMethodDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	changes := map[string]any{}
	if input.ExpMonth != 0 {
		changes["exp_month"] = input.ExpMonth
	}
	if input.ExpYear != 0 {
		changes["exp_year"] = input.ExpYear
	}
	if holder := strings.TrimSpace(input.HolderName); holder != "" {
		changes["holder_name"] = holder
	}
	if input.IsDefault != nil && *input.IsDefault {
		changes["is_default"] = true
	}
	if len(changes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nada para atualizar"})
		return
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if v, isDefault := changes["is_default"]; isDefault && v.(bool) {
			if err := tx.Model(&domain.PaymentMethod{}).Where("user_id = ?", userID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&method).Updates(changes).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar cartao"})
		return
	}

	if err := h.DB.First(&method, "id = ?", method.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao recarregar cartao"})
		return
	}
	c.JSON(http.StatusOK, method)
}

func (h *BillingHandler) DeletePaymentMethod(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var method domain.PaymentMethod
	if err := h.ownedPaymentMethod(c, &method, userID); err != nil {
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&method).Error; err != nil {
			return err
		}
		// Se era o padrao, o mais antigo vira o novo padrao.
		if !method.IsDefault {
			return nil
		}
		var next domain.PaymentMethod
		if err := tx.Where("user_id = ?", userID).Order("created_at asc").First(&next).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		return tx.Model(&next).Update("is_default", true).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao apagar cartao"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// ownedPaymentMethod carrega o cartao da rota garantindo que e do usuario
// logado. Ja escreve a resposta de erro quando falha.
func (h *BillingHandler) ownedPaymentMethod(c *gin.Context, out *domain.PaymentMethod, userID uuid.UUID) error {
	id, err := uuid.Parse(c.Param("methodId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de cartao invalido"})
		return err
	}
	if err := h.DB.First(out, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cartao nao encontrado"})
		return err
	}
	return nil
}

// ---------------------------------------------------------------- enderecos

func (h *BillingHandler) ListAddresses(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var addresses []domain.Address
	if err := h.DB.Where("user_id = ?", userID).
		Order("is_default desc, created_at asc").Find(&addresses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar enderecos"})
		return
	}
	if addresses == nil {
		addresses = []domain.Address{}
	}
	c.JSON(http.StatusOK, addresses)
}

func (h *BillingHandler) CreateAddress(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input dto.CreateAddressDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fields, valid := normalizedAddress(input)
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Preencha todos os campos do endereco"})
		return
	}

	var total int64
	h.DB.Model(&domain.Address{}).Where("user_id = ?", userID).Count(&total)
	address := domain.Address{
		UserID:     userID,
		Label:      strings.TrimSpace(input.Label),
		Recipient:  fields["recipient"],
		Street:     fields["street"],
		Number:     fields["number"],
		Complement: strings.TrimSpace(input.Complement),
		ZipCode:    fields["zip_code"],
		City:       fields["city"],
		State:      fields["state"],
		IsDefault:  input.IsDefault || total == 0,
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if address.IsDefault {
			if err := tx.Model(&domain.Address{}).Where("user_id = ?", userID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&address).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao salvar endereco"})
		return
	}

	c.JSON(http.StatusCreated, address)
}

func (h *BillingHandler) UpdateAddress(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var address domain.Address
	if err := h.ownedAddress(c, &address, userID); err != nil {
		return
	}

	var input dto.UpdateAddressDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	changes := map[string]any{}
	if input.Label != nil {
		changes["label"] = strings.TrimSpace(*input.Label)
	}
	for field, value := range map[string]*string{
		"recipient":  input.Recipient,
		"street":     input.Street,
		"number":     input.Number,
		"complement": input.Complement,
		"zip_code":   input.ZipCode,
		"city":       input.City,
		"state":      input.State,
	} {
		if value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*value)
		// Complemento e o unico campo que aceita ficar vazio.
		if trimmed == "" && field != "complement" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Preencha todos os campos do endereco"})
			return
		}
		if field == "state" {
			trimmed = strings.ToUpper(trimmed)
			if len(trimmed) != 2 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "UF deve ter 2 letras"})
				return
			}
		}
		changes[field] = trimmed
	}
	if input.IsDefault != nil && *input.IsDefault {
		changes["is_default"] = true
	}
	if len(changes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nada para atualizar"})
		return
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if v, isDefault := changes["is_default"]; isDefault && v.(bool) {
			if err := tx.Model(&domain.Address{}).Where("user_id = ?", userID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&address).Updates(changes).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar endereco"})
		return
	}

	if err := h.DB.First(&address, "id = ?", address.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao recarregar endereco"})
		return
	}
	c.JSON(http.StatusOK, address)
}

func (h *BillingHandler) DeleteAddress(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var address domain.Address
	if err := h.ownedAddress(c, &address, userID); err != nil {
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&address).Error; err != nil {
			return err
		}
		if !address.IsDefault {
			return nil
		}
		var next domain.Address
		if err := tx.Where("user_id = ?", userID).Order("created_at asc").First(&next).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		return tx.Model(&next).Update("is_default", true).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao apagar endereco"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// ownedAddress carrega o endereco da rota garantindo que e do usuario logado.
func (h *BillingHandler) ownedAddress(c *gin.Context, out *domain.Address, userID uuid.UUID) error {
	id, err := uuid.Parse(c.Param("addressId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de endereco invalido"})
		return err
	}
	if err := h.DB.First(out, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Endereco nao encontrado"})
		return err
	}
	return nil
}

// normalizedAddress devolve os campos obrigatorios ja sem espacos nas pontas.
func normalizedAddress(input dto.CreateAddressDTO) (map[string]string, bool) {
	out := map[string]string{
		"recipient": strings.TrimSpace(input.Recipient),
		"street":    strings.TrimSpace(input.Street),
		"number":    strings.TrimSpace(input.Number),
		"zip_code":  onlyDigits(input.ZipCode),
		"city":      strings.TrimSpace(input.City),
		"state":     strings.ToUpper(strings.TrimSpace(input.State)),
	}
	if out["zip_code"] == "" || len(out["state"]) != 2 {
		return nil, false
	}
	for _, key := range []string{"recipient", "street", "number", "city"} {
		if out[key] == "" {
			return nil, false
		}
	}
	return out, true
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cardBrand identifica a bandeira pelos prefixos. O numero completo nunca e
// gravado: so a bandeira e os 4 ultimos digitos.
func cardBrand(digits string) string {
	switch {
	case strings.HasPrefix(digits, "4"):
		return "visa"
	case strings.HasPrefix(digits, "5") || inRange(digits, 2221, 2720):
		return "mastercard"
	case strings.HasPrefix(digits, "3"):
		return "amex"
	case strings.HasPrefix(digits, "6"):
		return "elo"
	default:
		return "outro"
	}
}

// inRange compara os 4 primeiros digitos com uma faixa (a Mastercard usa faixa
// de BIN, nao um prefixo fixo).
func inRange(digits string, low, high int) bool {
	if len(digits) < 4 {
		return false
	}
	n := 0
	for _, r := range digits[:4] {
		n = n*10 + int(r-'0')
	}
	return n >= low && n <= high
}
