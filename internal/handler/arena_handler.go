package handler

import (
	"net/http"

	"squesh_golang/internal/dto"
	"squesh_golang/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ArenaHandler struct {
	DB *gorm.DB
}

func NewArenaHandler(db *gorm.DB) *ArenaHandler {
	return &ArenaHandler{DB: db}
}

// GetBoard devolve o ranking da Arena no período pedido.
//
// Toda a montagem é feita no service: aqui só traduz para DTO e escolhe o
// status HTTP. Um `period` inválido devolve 400 em vez de cair silenciosamente
// na semana — esconder o erro faria o app mostrar "semanal" quando pediu
// "mensal".
func (h *ArenaHandler) GetBoard(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	period, valid := service.ParsePeriod(c.Query("period"))
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "period inválido: use week, month ou all",
		})
		return
	}

	// Antes de ler o ranking, consolida o estado do MEU usuário: é o que credita
	// o XP dos dias completos e recalcula a ofensiva. Sem isso, o dia fechado
	// agora só apareceria no ranking na próxima leitura de qualquer tela.
	service.SyncUserDays(h.DB, userID)
	service.SyncDayRewards(h.DB, userID)

	board, err := service.BuildArena(h.DB, period, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Erro ao montar o ranking da arena",
		})
		return
	}

	c.JSON(http.StatusOK, arenaResponse(board, period, userID))
}

// GetMyStatus devolve só o resumo pessoal, sem o ranking inteiro.
//
// Existe separado do board porque a tela de Configurações quer o "como eu
// estou" (posição, liga, falta quanto) sem baixar a lista de todos os usuários
// só para desenhar um card.
func (h *ArenaHandler) GetMyStatus(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	period, valid := service.ParsePeriod(c.Query("period"))
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "period inválido: use week, month ou all",
		})
		return
	}

	service.SyncUserDays(h.DB, userID)
	service.SyncDayRewards(h.DB, userID)

	board, err := service.BuildArena(h.DB, period, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Erro ao montar o ranking da arena",
		})
		return
	}

	summary := service.Summarize(board, userID)
	if summary == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  arenaSummaryDTO(summary),
		"tiers": arenaTierDTOs(),
		"period": string(period),
	})
}

// ---- Tradução ----

func arenaResponse(
	board *service.ArenaBoard,
	period service.ArenaPeriod,
	me uuid.UUID,
) dto.ArenaBoardResponseDTO {
	out := dto.ArenaBoardResponseDTO{
		Period: string(period),
		Tiers:  arenaTierDTOs(),
		Total:  len(board.Standings),
	}

	if period != service.PeriodAll {
		from, to := board.From, board.To
		out.From, out.To = &from, &to
	}

	if summary := service.Summarize(board, me); summary != nil {
		s := arenaSummaryDTO(summary)
		out.Me = &s
	}

	out.Standings = make([]dto.ArenaStandingResponseDTO, 0, len(board.Standings))
	for _, st := range board.Standings {
		out.Standings = append(out.Standings, dto.ArenaStandingResponseDTO{
			Position:     st.Rank,
			TierPosition: st.TierRank,
			UserID:       st.UserID,
			Name:         st.Name,
			AvatarURL:    st.AvatarURL,
			Points:       st.Points,
			TotalPoints:  st.TotalPoints,
			Streak:       st.Streak,
			Tier:         st.Tier.Slug,
			IsPromoting:  st.IsPromoting,
			IsDemoting:   st.IsDemoting,
			IsMe:         st.IsMe,
		})
	}

	return out
}

func arenaSummaryDTO(s *service.ArenaSummary) dto.ArenaSummaryDTO {
	out := dto.ArenaSummaryDTO{
		Position:     s.Position,
		TierPosition: s.InTierPosition,
		Tier:         s.Tier.Slug,
		Points:       s.Points,
		TotalPoints:  s.TotalPoints,
		Streak:       s.Streak,
		PointsToNext: s.PointsToNext,
		TierProgress: s.TierProgress,
		IsPromoting:  s.Promoting,
		IsDemoting:   s.Demoting,
	}

	if s.NextTier != nil {
		next := s.NextTier.Slug
		out.NextTier = &next
	}

	if s.Ahead != nil {
		out.Ahead = &dto.ArenaRivalDTO{
			UserID: s.Ahead.UserID, Name: s.Ahead.Name, Gap: s.Ahead.Gap,
		}
	}
	if s.Behind != nil {
		out.Behind = &dto.ArenaRivalDTO{
			UserID: s.Behind.UserID, Name: s.Behind.Name, Gap: s.Behind.Gap,
		}
	}

	return out
}

func arenaTierDTOs() []dto.ArenaTierDTO {
	tiers := service.ArenaTiers()
	out := make([]dto.ArenaTierDTO, 0, len(tiers))
	for _, tier := range tiers {
		out = append(out, dto.ArenaTierDTO{
			Slug:           tier.Slug,
			Name:           tier.Name,
			MinPoints:      tier.MinPoints,
			PromotionSlots: tier.PromotionSlots,
			DemotionSlots:  tier.DemotionSlots,
		})
	}
	return out
}


