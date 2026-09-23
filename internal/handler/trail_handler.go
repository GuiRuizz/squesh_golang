package handler

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

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

	// 3. Executa as validações e gravações dentro de uma Transação do Banco
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Valida se o item da trilha realmente existe
		var item domain.TrailItem
		if err := tx.First(&item, "id = ?", itemID).Error; err != nil {
			return fmt.Errorf("item da trilha não encontrado")
		}

		// Verifica se o usuário já concluiu esse item
		var count int64
		tx.Model(&domain.UserTrailProgress{}).
			Where("user_id = ? AND trail_item_id = ?", userID, itemID).
			Count(&count)

		if count > 0 {
			return fmt.Errorf("este item já foi concluído anteriormente")
		}

		// Registra a conclusão na tabela de progresso
		progress := domain.UserTrailProgress{
			UserID:      userID,
			TrailItemID: itemID,
			CompletedAt: time.Now(),
		}
		if err := tx.Create(&progress).Error; err != nil {
			return err
		}

		// Busca os dados atuais do usuário com Lock (FOR UPDATE) para evitar race conditions
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&updatedUser, "id = ?", userID).Error; err != nil {
			return err
		}

		// Lógica de cálculo da Streak diária
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		if updatedUser.LastActiveDate == nil {
			// Primeira conclusão registrada no app
			updatedUser.StreakCount = 1
		} else {
			lastActive := *updatedUser.LastActiveDate
			lastActiveDay := time.Date(lastActive.Year(), lastActive.Month(), lastActive.Day(), 0, 0, 0, 0, lastActive.Location())

			daysDiff := int(today.Sub(lastActiveDay).Hours() / 24)

			switch {
			case daysDiff == 1:
				// Atividade no dia consecutivo -> Incrementa Streak
				updatedUser.StreakCount++
			case daysDiff > 1:
				// Quebra na sequência de dias -> Reseta para 1
				updatedUser.StreakCount = 1
				// daysDiff == 0 -> Já completou algum item hoje, mantém a streak igual
			}
		}

		// Atualiza o LastActiveDate e pontuação
		updatedUser.LastActiveDate = &now
		updatedUser.Points += 10

		if err := tx.Save(&updatedUser).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 4. Retorna a resposta ao frontend
	c.JSON(http.StatusOK, gin.H{
		"message":      "Item concluído com sucesso!",
		"streak_count": updatedUser.StreakCount,
		"points":       updatedUser.Points,
	})
}

// ToggleMealCheck alterna o status de conclusão de uma refeição/item de nutrição
func (h *TrailHandler) ToggleMealCheck(c *gin.Context) {
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

	itemIDParam := c.Param("mealId")
	itemID, err := uuid.Parse(itemIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do item inválido"})
		return
	}

	var isChecked bool

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Busca o item juntamente com a trilha pai para validar o tipo
		var item domain.TrailItem
		if err := tx.Preload("Trail").First(&item, "id = ?", itemID).Error; err != nil {
			return fmt.Errorf("item não encontrado")
		}

		// Verifica se o item pertence a uma trilha de nutrição
		var trail domain.Trail
		if err := tx.First(&trail, "id = ?", item.TrailID).Error; err != nil {
			return fmt.Errorf("trilha associada não encontrada")
		}

		if trail.Type != domain.TrailTypeNutrition {
			return fmt.Errorf("o item informado não pertence a uma trilha de nutrição")
		}

		// Verifica se já existe o registro de conclusão
		var progress domain.UserTrailProgress
		err := tx.Where("user_id = ? AND trail_item_id = ?", userID, itemID).First(&progress).Error

		if err == nil {
			// Já existe -> desmarca (deleta o registro)
			if err := tx.Delete(&progress).Error; err != nil {
				return err
			}
			isChecked = false
		} else if err == gorm.ErrRecordNotFound {
			// Não existe -> marca como concluído
			newProgress := domain.UserTrailProgress{
				UserID:      userID,
				TrailItemID: itemID,
				CompletedAt: time.Now(),
			}
			if err := tx.Create(&newProgress).Error; err != nil {
				return err
			}
			isChecked = true
		} else {
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Status da refeição alterado com sucesso",
		"checked": isChecked,
	})
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
