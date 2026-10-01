package handler

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"
	"squesh_golang/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TrailHandler struct {
	DB *gorm.DB
}

func NewTrailHandler(db *gorm.DB) *TrailHandler {
	return &TrailHandler{DB: db}
}

// syncDays consolida os dias do usuário antes de ler/gravar progresso: apaga o
// que ficou pela metade nos dias passados (os selects voltam a ficar
// desmarcados) e recalcula a ofensiva. É idempotente e barato quando não há
// nada a limpar.
// syncDays consolida os dias do usuário E credita o XP da Arena.
//
// Um único ponto de chamada de propósito: os dois passos dependem dos mesmos
// dias completos, e a rotina de dias deleta o progresso parcial do passado —
// se algum chamador usasse `SyncUserDays` direto, o dia poderia valer para a
// ofensiva sem ter rendido XP.
func (h *TrailHandler) syncDays(userID uuid.UUID) error {
	_, _, err := service.SyncAndReward(h.DB, userID)
	return err
}

// ListTrails busca todas as trilhas ou filtra por tipo (?type=workout ou ?type=nutrition)
func (h *TrailHandler) ListTrails(c *gin.Context) {
	trailType := c.Query("type")

	var trails []domain.Trail
	query := h.DB.Preload("Items")

	if trailType != "" {
		query = query.Where("type = ?", trailType)
	}

	if err := query.Find(&trails).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar trilhas"})
		return
	}

	c.JSON(http.StatusOK, trails)
}

// GetTrailByID busca os detalhes de uma trilha específica
func (h *TrailHandler) GetTrailByID(c *gin.Context) {
	idParam := c.Param("id")
	trailID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de trilha inválido"})
		return
	}

	var trail domain.Trail
	if err := h.DB.Preload("Items").First(&trail, "id = ?", trailID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trilha não encontrada"})
		return
	}

	// Auth OPCIONAL: se o cliente enviar um Bearer token válido, a resposta
	// ganha o progresso do usuário nessa trilha (quantos já concluídos, quantos
	// faltam e a flag "completed" em cada item). Sem token, o GET continua
	// público como antes.
	if userID, ok := optionalUserID(c); ok {
		if err := h.syncDays(userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao consolidar o progresso diário"})
			return
		}

		checks, err := h.userStepChecks(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular progresso"})
			return
		}

		completed := 0
		for i := range trail.Items {
			if applyItemProgress(&trail.Items[i], checks[trail.Items[i].ID]) {
				completed++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"trail": trail,
			"progress": gin.H{
				"completed_items": completed,
				"total_items":     len(trail.Items),
				"remaining_items": len(trail.Items) - completed,
				"percent":         trailPercent(completed, len(trail.Items)),
			},
		})
		return
	}

	c.JSON(http.StatusOK, trail)
}

// CreateTrail cria uma nova trilha
func (h *TrailHandler) CreateTrail(c *gin.Context) {
	var input dto.CreateTrailDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trail := domain.Trail{
		Title:       input.Title,
		Description: input.Description,
		Type:        domain.TrailType(input.Type),
		Level:       input.Level,
	}

	if err := h.DB.Create(&trail).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar trilha"})
		return
	}

	c.JSON(http.StatusCreated, trail)
}

// AddItemToTrail insere uma nova etapa/item na trilha
func (h *TrailHandler) AddItemToTrail(c *gin.Context) {
	idParam := c.Param("id")
	trailID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de trilha inválido"})
		return
	}

	var input dto.CreateTrailItemDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item := domain.TrailItem{
		TrailID:     trailID,
		Title:       input.Title,
		Description: input.Description,
		Order:       input.Order,
		Value:       input.Value,
	}

	if err := h.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao adicionar item na trilha"})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// GenerateNextTrailBlock gera os próximos N itens de uma trilha específica
func GenerateNextTrailBlock(db *gorm.DB, trailID uuid.UUID, amount int) ([]domain.TrailItem, error) {
	var lastItem domain.TrailItem

	// 1. Busca o último item cadastrado na trilha para continuar a sequência do campo Order
	err := db.Where("trail_id = ?", trailID).Order("order DESC").First(&lastItem).Error
	nextOrder := 1
	if err == nil {
		nextOrder = lastItem.Order + 1
	}

	var newItems []domain.TrailItem

	// 2. Cria os novos itens respeitando os campos da struct TrailItem
	for i := 0; i < amount; i++ {
		item := domain.TrailItem{
			TrailID:     trailID,
			Order:       nextOrder,
			Title:       fmt.Sprintf("Etapa %d", nextOrder),
			Description: fmt.Sprintf("Meta gerada automaticamente para a sequência #%d da sua jornada.", nextOrder),
			Value:       fmt.Sprintf("%d repetições / meta diária", 10+(nextOrder%5)*5), // Exemplo de valor dinâmico
		}
		newItems = append(newItems, item)
		nextOrder++
	}

	// 3. Salva os novos registros em lote
	if err := db.Create(&newItems).Error; err != nil {
		return nil, err
	}

	return newItems, nil
}

// GenerateInfiniteItems é a rota que expõe a criação sob demanda
func (h *TrailHandler) GenerateInfiniteItems(c *gin.Context) {
	trailIDParam := c.Param("id")
	trailID, err := uuid.Parse(trailIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID da trilha em formato inválido"})
		return
	}

	amountStr := c.DefaultQuery("amount", "5")
	amount, err := strconv.Atoi(amountStr)
	if err != nil || amount < 1 || amount > 50 {
		amount = 5
	}

	var trail domain.Trail
	if err := h.DB.First(&trail, "id = ?", trailID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trilha não encontrada"})
		return
	}

	newItems, err := GenerateNextTrailBlock(h.DB, trail.ID, amount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gerar novos itens para a trilha"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": fmt.Sprintf("%d novos itens gerados com sucesso", len(newItems)),
		"data":    newItems,
	})
}

// CompleteTrailItem marca um item como concluído, atualiza a streak e adiciona pontos ao usuário
func (h *TrailHandler) CompleteTrailItem(c *gin.Context) {
	// 1. Obtém userID do contexto (gerado pelo AuthMiddleware)
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	var userID uuid.UUID
	var err error

	switch v := userIDVal.(type) {
	case uuid.UUID:
		userID = v
	case string:
		userID, err = uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ID de usuário no token é inválido"})
			return
		}
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Formato de userID inválido"})
		return
	}

	// 2. Extrai e valida o itemId da URL
	itemIDParam := c.Param("itemId")
	itemID, err := uuid.Parse(itemIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do item em formato inválido"})
		return
	}

	var updatedUser domain.User
	// itemXP é o XP creditado pela conclusão deste item (0 se já estava pago).
	var itemXP int

	// 3. Executa as validações e gravações dentro de uma Transação do Banco
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Valida se o item da trilha realmente existe
		var item domain.TrailItem
		if err := tx.First(&item, "id = ?", itemID).Error; err != nil {
			return fmt.Errorf("item da trilha não encontrado")
		}

		// Itens com etapas internas (dia de alimentação / sessão de treino) são
		// concluídos parte a parte em /trails/items/:id/steps, não por aqui.
		// A guarda vem ANTES das demais para o erro ser sempre o correto.
		if len(item.Steps) > 0 {
			return fmt.Errorf(
				"este item tem %d etapas: marque cada uma em /trails/items/%s/steps",
				len(item.Steps), itemID,
			)
		}

		// Verifica se o usuário já concluiu esse item
		var count int64
		if err := tx.Model(&domain.UserTrailProgress{}).
			Where("user_id = ? AND trail_item_id = ? AND step_index = -1", userID, itemID).
			Count(&count).Error; err != nil {
			return err
		}

		if count > 0 {
			return fmt.Errorf("este item já foi concluído anteriormente")
		}

		// Regra de negócio: máximo de 1 conclusão por dia por TIPO de trilha
		// (1 treino/dia + 1 dia de alimentação/dia, independentes entre si).
		var itemTrail domain.Trail
		if err := tx.First(&itemTrail, "id = ?", item.TrailID).Error; err != nil {
			return fmt.Errorf("trilha do item não encontrada")
		}

		reached, err := dailyLimitReachedExcept(tx, userID, itemTrail.Type, item.ID)
		if err != nil {
			return err
		}
		if reached {
			return fmt.Errorf(
				"limite diário atingido: você já concluiu 1 %s hoje. Volte amanhã!",
				dailyLabel(itemTrail.Type),
			)
		}

		// Registra a conclusão na tabela de progresso.
		// StepIndex = -1: item simples, sem etapas internas.
		progress := domain.UserTrailProgress{
			UserID:      userID,
			TrailItemID: itemID,
			StepIndex:   -1,
			CompletedAt: time.Now(),
		}
		if err := tx.Create(&progress).Error; err != nil {
			return err
		}

		// XP do item concluído. A chave única (usuário, tipo, item, dia) impede
		// que o mesmo item pague de novo no mesmo dia.
		itemXP, err = service.AwardXP(
			tx, userID,
			domain.XPEventItemComplete,
			itemID.String(),
			&itemID, string(itemTrail.Type),
			time.Now(),
			service.XPPerItemComplete,
		)
		if err != nil {
			return err
		}

		// Busca os dados atuais do usuário com Lock (FOR UPDATE) para evitar race conditions
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&updatedUser, "id = ?", userID).Error; err != nil {
			return err
		}

		// A ofensiva NÃO é incrementada aqui: ela é derivada dos dias completos
		// (alimentação E treino 100%), recalculada em service.SyncUserDays. Este
		// handler só registra a atividade; o XP da Arena vem do razão em
		// `user_xp_events`, não desta coluna.
		//
		// `points` continua existindo no usuário (o perfil mostra), mas a
		// Ranking antigo lia daqui. Deixamos de somar para não haver dois
		// contadores de XP discordando — o canônico é o razão.
		now := time.Now()
		updatedUser.LastActiveDate = &now

		if err := tx.Save(&updatedUser).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Recalcula a ofensiva E credita o XP da Arena já considerando a conclusão
	// recém-gravada (esta conclusão pode ter fechado um dia completo, que paga
	// o XP do dia + bônus de ofensiva).
	streak := updatedUser.StreakCount
	dayXP := 0
	if state, gained, syncErr := service.SyncAndReward(h.DB, userID); syncErr == nil {
		streak = state.Streak
		dayXP = gained
	}

	// Total do usuário no razão: é o número que a Arena soma, então o app
	// recebe o mesmo valor que o ranking vai mostrar.
	var totalXP int
	h.DB.Model(&domain.XPEvent{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(points), 0)").Scan(&totalXP)

	// 4. Retorna a resposta ao frontend
	gained := itemXP + dayXP
	response := gin.H{
		"message":      "Item concluído com sucesso!",
		"streak_count": streak,
		"points":       totalXP,
		"xp":           gained,
	}
	if message := service.GrantMessage(gained); message != "" {
		response["xp_message"] = message
	}
	c.JSON(http.StatusOK, response)
}

// ToggleStepCheck marca/desmarca UMA etapa interna de um item: uma refeição do
// dia (nutrição) ou um exercício da sessão (treino). A etapa (dia/sessão) só
// conta como concluída quando todas as suas partes estão marcadas.
func (h *TrailHandler) ToggleStepCheck(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	var userID uuid.UUID
	switch v := userIDVal.(type) {
	case uuid.UUID:
		userID = v
	case string:
		parsedID, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ID de usuário inválido"})
			return
		}
		userID = parsedID
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Formato de userID inválido"})
		return
	}

	itemIDParam := c.Param("itemId")
	itemID, err := uuid.Parse(itemIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do item inválido"})
		return
	}

	var isChecked bool
	// xpGained acumula o XP creditado nesta transação para voltar na resposta.
	var xpGained int

	// Corpo OPCIONAL: { "step_index": 0, "checked": true }
	//   - step_index: qual etapa do dia/sessão (obrigatório quando há etapas)
	//   - checked:    estado desejado; se ausente, alterna (toggle)
	var payload struct {
		StepIndex *int  `json:"step_index"`
		MealIndex *int  `json:"meal_index"` // aceito por compatibilidade com o app antigo
		Checked   *bool `json:"checked"`
	}
	_ = c.ShouldBindJSON(&payload)
	if payload.StepIndex == nil {
		payload.StepIndex = payload.MealIndex
	}

	// Fecha os dias anteriores antes de mexer no progresso de hoje: se ontem
	// ficou pela metade, os selects de ontem voltam a ficar desmarcados.
	if err := h.syncDays(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao consolidar o progresso diário"})
		return
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Busca o item juntamente com a trilha pai (para o limite diário)
		var item domain.TrailItem
		if err := tx.Preload("Trail").First(&item, "id = ?", itemID).Error; err != nil {
			return fmt.Errorf("item não encontrado")
		}

		var trail domain.Trail
		if err := tx.First(&trail, "id = ?", item.TrailID).Error; err != nil {
			return fmt.Errorf("trilha associada não encontrada")
		}

		// Resolve a etapa alvo: um item com Steps exige step_index; itens
		// antigos (sem etapas) usam o modo simples (-1).
		stepIndex := -1
		if len(item.Steps) > 0 {
			if payload.StepIndex == nil {
				return fmt.Errorf("informe step_index com a etapa que deseja marcar")
			}
			stepIndex = *payload.StepIndex
			if stepIndex < 0 || stepIndex >= len(item.Steps) {
				return fmt.Errorf(
					"step_index inválido: %s tem %d etapas",
					trail.Type, len(item.Steps),
				)
			}
		}

		// Estado atual daquela etapa específica
		var progress domain.UserTrailProgress
		findErr := tx.Where(
			"user_id = ? AND trail_item_id = ? AND step_index = ?",
			userID, itemID, stepIndex,
		).First(&progress).Error
		if findErr != nil && findErr != gorm.ErrRecordNotFound {
			return findErr
		}
		found := findErr == nil

		wantChecked := !found
		if payload.Checked != nil {
			wantChecked = *payload.Checked
		}

		if wantChecked && !found {
			// Regra de negócio: 1 etapa do dia por tipo de trilha. Seguir marcando
			// as demais etapas do MESMO dia/sessão é livre; começar OUTRO hoje, não.
			reached, dailyErr := dailyLimitReachedExcept(tx, userID, trail.Type, item.ID)
			if dailyErr != nil {
				return dailyErr
			}
			if reached {
				return fmt.Errorf(
					"limite diário atingido: você já começou o %s de hoje. Volte amanhã!",
					dailyStepLabel(trail.Type),
				)
			}

			newProgress := domain.UserTrailProgress{
				UserID:      userID,
				TrailItemID: itemID,
				StepIndex:   stepIndex,
				CompletedAt: time.Now(),
			}
			if err := tx.Create(&newProgress).Error; err != nil {
				return err
			}

			// XP da etapa marcada. A chave única (usuário, tipo, item:etapa, dia)
			// garante que desmarcar e remarcar no mesmo dia não pague de novo.
			granted, grantErr := service.AwardXP(
				tx, userID,
				domain.XPEventStepCheck,
				fmt.Sprintf("%s:%d", itemID, stepIndex),
				&itemID, string(trail.Type),
				time.Now(),
				service.XPPerStepCheck,
			)
			if grantErr != nil {
				return grantErr
			}
			xpGained += granted

			// Se esta era a ÚLTIMA etapa que faltava, o item (dia/sessão) fechou:
			// vale o bônus de item completo também.
			var doneCount int64
			if err := tx.Model(&domain.UserTrailProgress{}).
				Where("user_id = ? AND trail_item_id = ?", userID, itemID).
				Count(&doneCount).Error; err != nil {
				return err
			}
			needed := len(item.Steps)
			if needed == 0 {
				needed = 1
			}
			if int(doneCount) >= needed {
				granted, grantErr = service.AwardXP(
					tx, userID,
					domain.XPEventItemComplete,
					itemID.String(),
					&itemID, string(trail.Type),
					time.Now(),
					service.XPPerItemComplete,
				)
				if grantErr != nil {
					return grantErr
				}
				xpGained += granted
			}
		} else if !wantChecked && found {
			// Desmarcar a etapa (sempre permitido) libera a marcação
			if err := tx.Delete(&progress).Error; err != nil {
				return err
			}
		}

		isChecked = wantChecked
		return nil
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Consolida os dias DEPOIS da transação: pode ser que esta etapa tenha
	// fechado o dia (100% alimentação + treino), e aí o XP do dia e o bônus de
	// ofensiva também entram antes de responder.
	streak := 0
	if state, syncErr := service.SyncUserDays(h.DB, userID); syncErr == nil {
		streak = state.Streak
	}
	if dayXP, rewardErr := service.SyncDayRewards(h.DB, userID); rewardErr == nil {
		xpGained += dayXP
	}

	response := gin.H{
		"message": "Status da etapa alterado com sucesso",
		"checked": isChecked,
		"streak":  streak,
		"xp":      xpGained,
	}
	// Só manda a frase de "+N XP" quando houve XP novo: repetir "+0 XP" toda
	// vez que o usuário desmarca uma etapa vira ruído.
	if message := service.GrantMessage(xpGained); message != "" {
		response["xp_message"] = message
	}
	c.JSON(http.StatusOK, response)
}

// BuildNewTrail cria uma trilha COMPLETA (trail + itens) remixando o conteúdo
// de trilhas existentes do mesmo tipo/nível — estilo Duolingo, permitindo gerar
// trilhas infinitas sem criar conteúdo manualmente a cada vez.
//
// Regras:
//   - Se SourceTrailID for informado, usa o tipo/nível dessa trilha como modelo;
//     caso contrário, usa Type/Level do input (pelo menos um precisa ser dado).
//   - O pool de itens vem de outras trilhas do mesmo tipo/nível (excluindo a
//     trilha modelo). Se estiver vazio mas a trilha modelo existe, usa os itens
//     dela como origem do remix. Se não houver nada, gera itens genéricos.
//   - Copia ItemCount itens embaralhados; se o pool for menor, repete com
//     "(Variação)" no título para nunca faltar conteúdo.
func BuildNewTrail(db *gorm.DB, input dto.GenerateTrailDTO) (*domain.Trail, error) {
	var resolvedType, resolvedLevel string
	var sourceID uuid.UUID
	var sourceItems []domain.TrailItem

	// 1. Descobre o tipo/nível da nova trilha
	if input.SourceTrailID != "" {
		var src domain.Trail
		if err := db.Preload("Items").First(&src, "id = ?", input.SourceTrailID).Error; err != nil {
			return nil, fmt.Errorf("trilha de origem não encontrada")
		}
		resolvedType = string(src.Type)
		resolvedLevel = src.Level
		sourceID = src.ID
		sourceItems = src.Items
	} else {
		resolvedType = input.Type
		resolvedLevel = input.Level
		if resolvedType == "" {
			return nil, fmt.Errorf("informe source_trail_id ou type para gerar a trilha")
		}
	}

	itemCount := input.ItemCount
	if itemCount < 1 {
		itemCount = 5
	}
	if itemCount > 20 {
		itemCount = 20
	}

	// 2. Monta o pool de itens vindos de trilhas existentes do mesmo tipo/nível
	var exemplars []domain.Trail
	query := db.Preload("Items").Where("type = ?", resolvedType)
	if resolvedLevel != "" {
		query = query.Where("level = ?", resolvedLevel)
	}
	if sourceID != uuid.Nil {
		query = query.Where("id <> ?", sourceID)
	}
	if err := query.Find(&exemplars).Error; err != nil {
		return nil, err
	}

	pool := make([]domain.TrailItem, 0)
	for i := range exemplars {
		pool = append(pool, exemplars[i].Items...)
	}

	// Sem outras trilhas do mesmo tipo -> usa a própria trilha modelo como pool
	if len(pool) == 0 && len(sourceItems) > 0 {
		pool = sourceItems
	}

	// 3. Embaralha o pool e monta os itens da nova trilha (com repetição c/ variação)
	shuffled := make([]domain.TrailItem, len(pool))
	copy(shuffled, pool)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	newItems := make([]domain.TrailItem, 0, itemCount)
	for i := 0; i < itemCount; i++ {
		order := i + 1

		if len(shuffled) == 0 {
			// Fallback: nenhum conteúdo existente -> metas genéricas
			newItems = append(newItems, domain.TrailItem{
				Order:       order,
				Title:       fmt.Sprintf("Etapa %d", order),
				Description: fmt.Sprintf("Meta gerada automaticamente para a sequência #%d da sua jornada.", order),
				Value:       fmt.Sprintf("%d repetições / meta diária", 10+(order%5)*5),
			})
			continue
		}

		source := shuffled[i%len(shuffled)]
		title := source.Title
		if i >= len(shuffled) {
			// Repetição do pool -> varia pequeno para parecer novo
			title = fmt.Sprintf("%s (Variação)", source.Title)
		}

		newItems = append(newItems, domain.TrailItem{
			Order:       order,
			Title:       title,
			Description: source.Description,
			Value:       source.Value,
		})
	}

	// 4. Título automático numerado caso não informado
	title := input.Title
	if title == "" {
		var count int64
		db.Model(&domain.Trail{}).Where("type = ?", resolvedType).Count(&count)

		label := "Trilha"
		if resolvedLevel != "" {
			label = resolvedLevel
		}
		switch domain.TrailType(resolvedType) {
		case domain.TrailTypeWorkout:
			title = fmt.Sprintf("Treino Gerado #%d (%s)", count+1, label)
		case domain.TrailTypeNutrition:
			title = fmt.Sprintf("Nutrição Gerada #%d (%s)", count+1, label)
		default:
			title = fmt.Sprintf("Trilha Gerada #%d (%s)", count+1, label)
		}
	}

	levelLabel := "personalizado"
	if resolvedLevel != "" {
		levelLabel = resolvedLevel
	}

	trail := domain.Trail{
		Title:       title,
		Description: fmt.Sprintf("Trilha %s %s gerada automaticamente a partir do conteúdo existente.", resolvedType, levelLabel),
		Type:        domain.TrailType(resolvedType),
		Level:       resolvedLevel,
	}

	// 5. Persiste trilha + itens numa transação
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&trail).Error; err != nil {
			return err
		}
		for i := range newItems {
			newItems[i].TrailID = trail.ID
		}
		return tx.Create(&newItems).Error
	})
	if err != nil {
		return nil, err
	}

	trail.Items = newItems
	return &trail, nil
}

// GenerateCompleteTrail é a rota que gera uma trilha completa a partir das existentes
func (h *TrailHandler) GenerateCompleteTrail(c *gin.Context) {
	var input dto.GenerateTrailDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trail, err := BuildNewTrail(h.DB, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Trilha completa gerada com sucesso",
		"data":    trail,
	})
}

// trailUserProgress agrega o progresso do usuário em uma trilha
type trailUserProgress struct {
	trail     domain.Trail
	completed int
	total     int
	nextItem  *domain.TrailItem
}

// userStepChecks devolve, por item, o conjunto de índices de etapa já
// marcados pelo usuário. Para itens sem etapas internas, o único índice válido
// é -1. É a base de todo o cálculo de progresso do app.
func (h *TrailHandler) userStepChecks(userID uuid.UUID) (map[uuid.UUID]map[int]bool, error) {
	var progresses []domain.UserTrailProgress
	if err := h.DB.Where("user_id = ?", userID).Find(&progresses).Error; err != nil {
		return nil, err
	}

	checks := make(map[uuid.UUID]map[int]bool, len(progresses))
	for _, p := range progresses {
		set, ok := checks[p.TrailItemID]
		if !ok {
			set = make(map[int]bool, 1)
			checks[p.TrailItemID] = set
		}
		set[p.StepIndex] = true
	}
	return checks, nil
}

// itemStepsRequired devolve quantas "conclusões" o item exige: 1 para itens
// simples e uma por etapa interna (refeições do dia / exercícios da sessão).
func itemStepsRequired(item *domain.TrailItem) int {
	if len(item.Steps) == 0 {
		return 1
	}
	return len(item.Steps)
}

// itemProgressCount conta quantas etapas do item já foram marcadas.
func itemProgressCount(item *domain.TrailItem, checks map[int]bool) int {
	if len(item.Steps) == 0 {
		if checks[-1] {
			return 1
		}
		return 0
	}
	count := 0
	for i := range item.Steps {
		if checks[i] {
			count++
		}
	}
	return count
}

// applyItemProgress preenche as flags transitórias do item (completed e o done
// de cada etapa) e devolve se o item está concluído.
func applyItemProgress(item *domain.TrailItem, checks map[int]bool) bool {
	required := itemStepsRequired(item)
	count := itemProgressCount(item, checks)
	done := count >= required

	completed := done
	item.Completed = &completed

	for i := range item.Steps {
		stepDone := checks[i]
		item.Steps[i].Done = &stepDone
	}
	return done
}

// computeUserProgress calcula, para cada trilha existente, quantos itens o
// usuário já concluiu, o total e o próximo item não concluído. Em itens com
// etapas internas, só conta como concluído com TODAS as etapas marcadas.
func (h *TrailHandler) computeUserProgress(userID uuid.UUID) ([]trailUserProgress, error) {
	// Consolida os dias passados antes de ler: o progresso parcial de um dia que
	// não fechou é descartado, então os selects do app já voltam desmarcados.
	if _, err := service.SyncUserDays(h.DB, userID); err != nil {
		return nil, err
	}

	checks, err := h.userStepChecks(userID)
	if err != nil {
		return nil, err
	}

	var trails []domain.Trail
	if err := h.DB.Preload("Items", func(db *gorm.DB) *gorm.DB {
		return db.Order("trail_items.order asc")
	}).Order("trails.created_at asc").Find(&trails).Error; err != nil {
		return nil, err
	}

	results := make([]trailUserProgress, 0, len(trails))
	for i := range trails {
		tr := trails[i]
		comp := 0
		var next *domain.TrailItem
		for j := range tr.Items {
			// Preenche as flags transitórias (completed + consumed por refeição)
			// para o App já renderizar o caminho sem outra chamada.
			if applyItemProgress(&tr.Items[j], checks[tr.Items[j].ID]) {
				comp++
			} else if next == nil {
				it := tr.Items[j]
				next = &it
			}
		}
		results = append(results, trailUserProgress{
			trail:     tr,
			completed: comp,
			total:     len(tr.Items),
			nextItem:  next,
		})
	}
	return results, nil
}

func trailPercent(completed, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(completed) / float64(total) * 100)
}

// GetMyActiveTrail (GET /trails/me/active) devolve a trilha atual do usuário:
// primeiro a que está em progresso e, se nenhuma, a próxima a ser iniciada.
// Aceita o filtro opcional ?type=workout|nutrition para o App pedir cada
// coluna da home separadamente. Inclui o próximo item a concluir — é o que
// renderiza o caminho estilo Duolingo.
func (h *TrailHandler) GetMyActiveTrail(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	trailType := c.Query("type")
	matchesType := func(p *trailUserProgress) bool {
		return trailType == "" || string(p.trail.Type) == trailType
	}

	progs, err := h.computeUserProgress(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular progresso"})
		return
	}

	// 1. Traz a trilha que já está em progresso
	var active *trailUserProgress
	for i := range progs {
		p := &progs[i]
		if !matchesType(p) {
			continue
		}
		if p.total > 0 && p.completed > 0 && p.completed < p.total {
			active = p
			break
		}
	}
	// 2. Se nenhuma iniciada, sugere a primeira trilha disponível
	if active == nil {
		for i := range progs {
			p := &progs[i]
			if !matchesType(p) {
				continue
			}
			if p.total > 0 && p.completed == 0 {
				active = p
				break
			}
		}
	}
	// 3. Tudo concluído -> orienta a gerar uma trilha nova
	if active == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Você concluiu todas as trilhas disponíveis. Gere uma nova com POST /trails/generate.",
			"hint":  "generate",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"active": active.trail,
		"progress": gin.H{
			"completed_items": active.completed,
			"total_items":     active.total,
			"percent":         trailPercent(active.completed, active.total),
		},
		"next_item": active.nextItem,
	})
}

// GetMyCompletedTrails (GET /trails/me/completed) lista as trilhas 100% concluídas
func (h *TrailHandler) GetMyCompletedTrails(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	progs, err := h.computeUserProgress(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular progresso"})
		return
	}

	completed := make([]gin.H, 0)
	for i := range progs {
		p := &progs[i]
		if p.total > 0 && p.completed == p.total {
			completed = append(completed, gin.H{
				"trail": p.trail,
				"progress": gin.H{
					"completed_items": p.completed,
					"total_items":     p.total,
					"percent":         100,
				},
			})
		}
	}

	c.JSON(http.StatusOK, completed)
}

// dailyLimitReachedExcept devolve true se o usuário JÁ começou hoje uma etapa
// do tipo informado, ignorando o item indicado (política: 1 conclusão por dia
// por tipo de trilha; continuar o mesmo dia é permitido, começar outro não).
func dailyLimitReachedExcept(
	tx *gorm.DB,
	userID uuid.UUID,
	trailType domain.TrailType,
	exceptItemID uuid.UUID,
) (bool, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// COUNT(DISTINCT item): um dia de alimentação tem várias refeições, mas
	// conta como UMA etapa do dia.
	var count int64
	err := tx.Model(&domain.UserTrailProgress{}).
		Select("COUNT(DISTINCT user_trail_progresses.trail_item_id)").
		Joins("JOIN trail_items ON trail_items.id = user_trail_progresses.trail_item_id").
		Joins("JOIN trails ON trails.id = trail_items.trail_id").
		Where(
			"user_trail_progresses.user_id = ? AND trails.type = ? AND user_trail_progresses.completed_at >= ? AND user_trail_progresses.trail_item_id <> ?",
			userID, trailType, start, exceptItemID,
		).
		Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count >= 1, nil
}

// dailyLabel devolve o rótulo amigável do tipo para as mensagens de limite.
func dailyLabel(trailType domain.TrailType) string {
	if trailType == domain.TrailTypeWorkout {
		return "treino"
	}
	return "alimentação"
}

// dailyStepLabel devolve o rótulo da ETAPA do dia (a sessão de treino ou o dia
// de alimentação) usado nas mensagens do toggle interno.
func dailyStepLabel(trailType domain.TrailType) string {
	if trailType == domain.TrailTypeWorkout {
		return "treino"
	}
	return "dia de alimentação"
}

// completionsTodayByType devolve quantas etapas o usuário CONCLUIU hoje
// (um dia de alimentação ou uma sessão de treino só conta com todas as partes
// marcadas), agrupado por tipo de trilha. Alimenta o `today_completed` da Home.
func (h *TrailHandler) completionsTodayByType(userID uuid.UUID) (map[domain.TrailType]int, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Um item está concluído quando tem linhas de progresso >= GREATEST(1, nº de
	// etapas internas). Item simples -> precisa de 1 linha; dia/sessão com 5
	// etapas -> precisa das 5.
	var rows []struct {
		Type   domain.TrailType
		ItemID uuid.UUID
	}
	err := h.DB.Raw(
		`SELECT trails.type AS type, utp.trail_item_id AS item_id
		 FROM user_trail_progresses utp
		 JOIN trail_items ON trail_items.id = utp.trail_item_id
		 JOIN trails ON trails.id = trail_items.trail_id
		 WHERE utp.user_id = ?
		 GROUP BY trails.type, utp.trail_item_id, trail_items.steps
		 HAVING COUNT(*) >= GREATEST(1, jsonb_array_length(COALESCE(trail_items.steps, '[]'::jsonb)))
		    AND MAX(utp.completed_at) >= ?`,
		userID, start,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[domain.TrailType]int, len(rows))
	for _, r := range rows {
		counts[r.Type]++
	}
	return counts, nil
}

// GetMyTrails (GET /trails/me) lista TODAS as trilhas do usuário com o
// progresso individual (itens com a flag `completed`) e informa se o limite
// diário de cada tipo já foi atingido hoje — é o que alimenta a Home com um
// botão/card por trilha e as etapas dentro. Aceita ?type=workout|nutrition.
func (h *TrailHandler) GetMyTrails(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	trailType := c.Query("type")
	matchesType := func(p *trailUserProgress) bool {
		return trailType == "" || string(p.trail.Type) == trailType
	}

	progs, err := h.computeUserProgress(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular progresso"})
		return
	}

	trails := make([]gin.H, 0, len(progs))
	for i := range progs {
		p := &progs[i]
		if !matchesType(p) {
			continue
		}
		trails = append(trails, gin.H{
			"trail": p.trail,
			"progress": gin.H{
				"completed_items": p.completed,
				"total_items":     p.total,
				"percent":         trailPercent(p.completed, p.total),
			},
		})
	}

	counts, err := h.completionsTodayByType(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular limite diário"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"trails": trails,
		"today_completed": gin.H{
			"workout":   counts[domain.TrailTypeWorkout] > 0,
			"nutrition": counts[domain.TrailTypeNutrition] > 0,
		},
	})
}
