package service

import (
	"fmt"
	"sort"
	"time"

	"squesh_golang/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Valor de cada evento de XP.
//
// A escala é pequena de propósito: um dia completo rende ~60, uma semana cheia
// ~300. Os cortes de liga são calibrados para essa escala (ver ArenaTiers), e o
// teto do bônus de ofensiva impede que o prêmio de constância mascare o valor
// do dia em si.
const (
	XPPerStepCheck    = 2  // uma refeição do dia / um exercício da sessão
	XPPerItemComplete = 10 // o item (dia ou sessão) chegou a 100%
	XPPerDayComplete  = 60 // o dia fechou alimentação E treino
	XPStreakBonusStep = 3  // por dia de ofensiva
	XPStreakBonusMax  = 30 // teto do bônus de ofensiva
)

// AwardXP registra UM evento no razão de XP.
//
// É idempotente: a chave única (user_id, kind, ref_key, day_key) faz o INSERT
// conflitar e nada ser gravado quando o mesmo feito já foi pago. Isso
// resolve o caso chato de "marquei a etapa, desmarquei, marquei de novo": o
// segundo toque não paga XP de novo, porque o dia continua o mesmo.
//
// Devolve quantos pontos foramcreditados NESTA chamada (0 = nada novo).
func AwardXP(
	tx *gorm.DB,
	userID uuid.UUID,
	kind, refKey string,
	itemID *uuid.UUID,
	trailType string,
	at time.Time,
	points int,
) (int, error) {
	if points <= 0 {
		return 0, nil
	}

	event := domain.XPEvent{
		UserID:     userID,
		Kind:       kind,
		RefKey:     refKey,
		DayKey:     DayStart(at).Format("2006-01-02"),
		Points:     points,
		ItemID:     itemID,
		TrailType:  trailType,
		CreditedAt: at,
	}

	// DoNothing: se a chave única já existe, o evento é descartado em silêncio.
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, nil
	}

	return points, nil
}

// awardDayComplete credita o dia completo (e o bônus de ofensiva) uma única vez
// por dia. `streakDays` é a ofensiva JÁ CONTADA depois deste dia fechar, então o
// bônus sobe conforme a sequência engorda.
func awardDayComplete(tx *gorm.DB, userID uuid.UUID, day time.Time, streakDays int) (int, error) {
	dayKey := DayStart(day).Format("2006-01-02")

	gained, err := AwardXP(
		tx, userID, domain.XPEventDayComplete, dayKey, nil, "", day, XPPerDayComplete,
	)
	if err != nil {
		return 0, err
	}

	bonus := streakDays * XPStreakBonusStep
	if bonus > XPStreakBonusMax {
		bonus = XPStreakBonusMax
	}
	bonusGained, err := AwardXP(
		tx, userID, domain.XPEventStreakBonus, dayKey, nil, "", day, bonus,
	)
	if err != nil {
		return gained, err
	}

	return gained + bonusGained, nil
}

// SyncDayRewards credita o XP dos dias completos que ainda não foram pagos.
//
// Roda junto com a consolidação de dias (`SyncUserDays`): como ambos leem os
// MESMOS dias completos, o crédito fica sempre coerente com a ofensiva — não
// existe caminho em que o dia vale para a ofensiva e não rendeu XP.
//
// Idempotente por natureza: um dia já creditado é ignorado na segunda passada.
func SyncDayRewards(db *gorm.DB, userID uuid.UUID) (int, error) {
	rows, err := loadProgressRows(db, userID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	// Mesma agregação do day_state: um item fecha quando alcança as etapas
	// exigidas, e o dia do item é o dia do ÚLTIMO passo.
	type itemAgg struct {
		trailType domain.TrailType
		needed    int
		count     int
		last      time.Time
	}
	items := make(map[uuid.UUID]*itemAgg, len(rows))
	for _, r := range rows {
		agg, ok := items[r.ItemID]
		if !ok {
			agg = &itemAgg{trailType: r.Type, needed: r.Needed}
			items[r.ItemID] = agg
		}
		agg.count++
		if r.CompletedAt.After(agg.last) {
			agg.last = r.CompletedAt
		}
	}

	completeByDay := make(map[string]map[domain.TrailType]bool)
	for _, agg := range items {
		if agg.count < agg.needed {
			continue
		}
		day := DayStart(agg.last).Format("2006-01-02")
		if completeByDay[day] == nil {
			completeByDay[day] = make(map[domain.TrailType]bool, 2)
		}
		completeByDay[day][agg.trailType] = true
	}
	perfect := func(key string) bool {
		m := completeByDay[key]
		return m[domain.TrailTypeNutrition] && m[domain.TrailTypeWorkout]
	}

	// Sequência de dias completos terminando em cada dia: é o que define a
	// ofensiva no momento em que o dia fechou (e não a ofensiva atual, que pode
	// ser bem maior e inflaria o bônus antigo).
	total := 0
	err = db.Transaction(func(tx *gorm.DB) error {
		for key := range completeByDay {
			if !perfect(key) {
				continue
			}
			day := parseDayKey(key)
			gained, err := awardDayComplete(tx, userID, day, streakEndingAt(completeByDay, day))
			if err != nil {
				return err
			}
			total += gained
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

// streakEndingAt conta quantos dias completos consecutivos terminam em `day`
// (contando o próprio dia). É a ofensiva que o usuário tinha no momento em que
// fechou aquele dia.
func streakEndingAt(completeByDay map[string]map[domain.TrailType]bool, day time.Time) int {
	perfect := func(d time.Time) bool {
		m := completeByDay[d.Format("2006-01-02")]
		return m[domain.TrailTypeNutrition] && m[domain.TrailTypeWorkout]
	}

	count := 0
	for cursor := day; perfect(cursor); cursor = cursor.AddDate(0, 0, -1) {
		count++
	}
	return count
}

func parseDayKey(key string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", key, AppLocation)
	if err != nil {
		return DayStart(time.Now())
	}
	return t
}

// ---- Período ----

// ArenaPeriod é o recorte do ranking: a semana, o mês ou a carreira inteira.
type ArenaPeriod string

const (
	PeriodWeek  ArenaPeriod = "week"
	PeriodMonth ArenaPeriod = "month"
	PeriodAll   ArenaPeriod = "all"
)

// ParsePeriod converte o que veio da query string. Devolve false para valor
// desconhecido, e o handler responde 400 em vez de adivinhar.
func ParsePeriod(raw string) (ArenaPeriod, bool) {
	switch raw {
	case "", "week", "weekly":
		return PeriodWeek, true
	case "month", "monthly":
		return PeriodMonth, true
	case "all", "alltime", "career":
		return PeriodAll, true
	default:
		return "", false
	}
}

// PeriodWindow devolve o primeiro e o último instante do período (inclusivo),
// já no fuso do app.
//
// A semana começa na segunda, que é como o brasileiro conta a semana de
// trabalho.
func PeriodWindow(period ArenaPeriod, now time.Time) (time.Time, time.Time) {
	today := DayStart(now)

	switch period {
	case PeriodAll:
		return time.Time{}, time.Time{}
	case PeriodMonth:
		start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, AppLocation)
		return start, start.AddDate(0, 1, 0).Add(-time.Millisecond)
	default: // PeriodWeek
		offset := (int(today.Weekday()) + 6) % 7 // segunda = 0
		start := today.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 7).Add(-time.Millisecond)
	}
}

// dayKeyRange devolve o intervalo de DayKey (string) do período, para filtrar
// o razão com comparação de string. Vazio = todos os dias.
func dayKeyRange(period ArenaPeriod, now time.Time) (from, to string) {
	if period == PeriodAll {
		return "", ""
	}
	start, end := PeriodWindow(period, now)
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

// ---- Cortes de liga ----

// ArenaTier é um corte de liga. O app recebe a lista inteira e monta os
// ícones/cores a partir do slug — o servidor é quem decide onde cada corte
// começa.
type ArenaTier struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// MinPoints é o XP do período que o corte exige para entrar nele.
	MinPoints int `json:"min_points"`
	// PromotionSlots é quantos do topo de cada liga sobem de corte.
	PromotionSlots int `json:"promotion_slots"`
	// DemotionSlots é quantos da base caem de corte.
	DemotionSlots int `json:"demotion_slots"`
}

// ArenaTiers é a escada de ligas, calibrada para a escala de XP acima.
//
// Um dia completo rende 60 + bônus (até 30) ≈ 90. Uma semana de 5 dias cheios
// fica em torno de 350, então o corte de Prata (150) é alcançável na segunda
// semana e o de Platina exige constância real.
func ArenaTiers() []ArenaTier {
	return []ArenaTier{
		{Slug: "bronze", Name: "Bronze", MinPoints: 0, PromotionSlots: 3, DemotionSlots: 3},
		{Slug: "silver", Name: "Prata", MinPoints: 150, PromotionSlots: 3, DemotionSlots: 3},
		{Slug: "gold", Name: "Ouro", MinPoints: 320, PromotionSlots: 3, DemotionSlots: 3},
		{Slug: "platinum", Name: "Platina", MinPoints: 560, PromotionSlots: 3, DemotionSlots: 3},
		{Slug: "diamond", Name: "Diamante", MinPoints: 900, PromotionSlots: 3, DemotionSlots: 0},
	}
}

// LeagueForXP devolve o corte em que o XP cai.
func LeagueForXP(points int) ArenaTier {
	tiers := ArenaTiers()
	out := tiers[0]
	for _, tier := range tiers {
		if points >= tier.MinPoints {
			out = tier
		}
	}
	return out
}

// NextTier devolve o corte logo acima (nil quando já está no topo).
func NextTier(slug string) *ArenaTier {
	tiers := ArenaTiers()
	for i, tier := range tiers {
		if tier.Slug == slug && i+1 < len(tiers) {
			return &tiers[i+1]
		}
	}
	return nil
}

// ---- Ranking ----

// ArenaStanding é uma linha do ranking.
type ArenaStanding struct {
	UserID   uuid.UUID
	Name     string
	AvatarURL string
	// Points é o XP do período; TotalPoints é a carreira toda.
	Points      int
	TotalPoints int
	Streak      int
	// Rank é a posição geral no recorte (1 = primeiro).
	Rank int
	// Tier e a posição dentro da liga.
	Tier        ArenaTier
	TierRank    int
	IsPromoting bool
	IsDemoting  bool
	// IsMe marca a linha do próprio usuário (o app usa para destacar).
	IsMe bool
}

// ArenaBoard é o ranking inteiro do período, já com cortes e zonas aplicadas.
type ArenaBoard struct {
	Period ArenaPeriod
	// From/To delimitam o período (zero em PeriodAll).
	From time.Time
	To   time.Time
	// Standings vem ordenado por XP (maior primeiro).
	Standings []ArenaStanding
	// Me indexa o próprio usuário dentro de Standings (-1 se não estiver).
	MeIndex int
}

// BuildArena monta o ranking do período.
//
// O XP vem do razão (`user_xp_events`) agregado por usuário no intervalo — uma
// consulta só, sem N+1. A LISTA de usuários vem do próprio SQL (LEFT JOIN),
// então quem ainda não pontuou aparece no fim da liga em vez de sumir.
//
// Quem aparece: todo mundo com conta. Esconder quem não pontuou só criaria a
// ilusão de liga mais cheia do que é.
func BuildArena(db *gorm.DB, period ArenaPeriod, me uuid.UUID) (*ArenaBoard, error) {
	now := time.Now()
	board := &ArenaBoard{Period: period, MeIndex: -1}

	from, to := dayKeyRange(period, now)
	if from != "" {
		start, end := PeriodWindow(period, now)
		board.From, board.To = start, end
	}

	type row struct {
		UserID      uuid.UUID
		Name        string
		AvatarURL   string
		Streak      int
		PeriodXP    int
		TotalXP     int
	}

	// O filtro do período vai no SUM (e não no WHERE) para que o LEFT JOIN
	// continue devolvendo quem não tem nenhum evento no recorte.
	sql := `
		SELECT u.id AS user_id,
		       u.name AS name,
		       COALESCE(u.avatar_url, '') AS avatar_url,
		       u.streak_count AS streak,
		       COALESCE(SUM(e.points) FILTER (WHERE e.day_key >= ? AND e.day_key <= ?), 0)::int AS period_xp,
		       COALESCE(SUM(e.points), 0)::int AS total_xp
		FROM users u
		LEFT JOIN user_xp_events e ON e.user_id = u.id
		`
	args := []any{from, to}
	if from == "" {
		// PeriodAll: o mesmo total serve para os dois campos, mas o filtro
		// vazio casaria tudo de novo, então somamos direto.
		sql = `
		SELECT u.id AS user_id,
		       u.name AS name,
		       COALESCE(u.avatar_url, '') AS avatar_url,
		       u.streak_count AS streak,
		       COALESCE(SUM(e.points), 0)::int AS period_xp,
		       COALESCE(SUM(e.points), 0)::int AS total_xp
		FROM users u
		LEFT JOIN user_xp_events e ON e.user_id = u.id
		`
		args = nil
	}
	sql += ` GROUP BY u.id, u.name, u.avatar_url, u.streak_count`

	var rows []row
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].PeriodXP != rows[j].PeriodXP {
			return rows[i].PeriodXP > rows[j].PeriodXP
		}
		if rows[i].Streak != rows[j].Streak {
			return rows[i].Streak > rows[j].Streak
		}
		// Empate total: o nome em ordem alfabética deixa a lista estável entre
		// requisições, senão o mesmo par trocaria de posição a cada chamada.
		return rows[i].Name < rows[j].Name
	})

	standings := make([]ArenaStanding, 0, len(rows))
	for i, r := range rows {
		st := ArenaStanding{
			UserID:     r.UserID,
			Name:       r.Name,
			AvatarURL:  r.AvatarURL,
			Points:     r.PeriodXP,
			TotalPoints: r.TotalXP,
			Streak:     r.Streak,
			Rank:       i + 1,
			Tier:       LeagueForXP(r.PeriodXP),
			IsMe:       r.UserID == me,
		}
		if st.IsMe {
			board.MeIndex = len(standings)
		}
		standings = append(standings, st)
	}

	applyTierZones(standings)
	board.Standings = standings
	return board, nil
}

// applyTierZones numera cada posição dentro da liga e marca a zona de
// promoção (topo) e a de rebaixamento (base).
//
// As zonas são por POSIÇÃO DENTRO DA LIGA, não por XP absoluto: o corte já
// separou por XP, então "sou o 1º da minha liga" é literalmente verdade. Assim
// a tela consegue desenhar as faixas coloridas sem o cliente recalcular nada.
func applyTierZones(standings []ArenaStanding) {
	if len(standings) == 0 {
		return
	}

	// Agrupa por corte preservando a ordem global (que já é por XP).
	groups := make([][]int, 0, 5)
	var currentSlug string
	for i, st := range standings {
		if st.Tier.Slug != currentSlug {
			currentSlug = st.Tier.Slug
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], i)
	}

	for _, idx := range groups {
		// O primeiro elemento da lista decide promotion/demotion da liga: usa o
		// do primeiro usuário, já que todos do grupo têm o mesmo corte.
		tier := standings[idx[0]].Tier
		for pos, boardIdx := range idx {
			standings[boardIdx].TierRank = pos + 1
			standings[boardIdx].IsPromoting = tier.PromotionSlots > 0 && pos < tier.PromotionSlots
			standings[boardIdx].IsDemoting = tier.DemotionSlots > 0 && pos >= len(idx)-tier.DemotionSlots
		}
	}
}

// ---- Resumo pessoal ----

// ArenaSummary é o "como eu estou" que a tela mostra em cima: posição, liga,
// quanto falta para a próxima e quem está logo acima.
type ArenaSummary struct {
	Position int
	// InTierPosition é a posição dentro da liga (1 = líder da liga).
	InTierPosition int
	Tier           ArenaTier
	Points         int
	TotalPoints    int
	Streak         int

	// NextTier/PointsToNext: quanto falta para o corte acima. Nil quando já
	// está no topo.
	NextTier       *ArenaTier
	PointsToNext   int
	TierProgress   float64 // 0..1 dentro do corte atual
	Promoting      bool
	Demoting       bool

	// Ahead é quem está imediatamente acima (o "rival"). Nil no primeiro lugar.
	Ahead *ArenaRival
	// Behind é quem está imediatamente abaixo. Nil no último lugar.
	Behind *ArenaRival
}

// ArenaRival é o vizinho de posição usado no card de desafio.
type ArenaRival struct {
	UserID uuid.UUID
	Name   string
	// Gap é a diferença de XP (sempre > 0): quanto falta para alcançar.
	Gap int
}

// Summarize monta o resumo do usuário dentro de um board já calculado.
func Summarize(board *ArenaBoard, me uuid.UUID) *ArenaSummary {
	if board.MeIndex < 0 || board.MeIndex >= len(board.Standings) {
		return nil
	}
	mine := board.Standings[board.MeIndex]

	summary := &ArenaSummary{
		Position:      mine.Rank,
		InTierPosition: mine.TierRank,
		Tier:          mine.Tier,
		Points:        mine.Points,
		TotalPoints:   mine.TotalPoints,
		Streak:        mine.Streak,
		Promoting:     mine.IsPromoting,
		Demoting:      mine.IsDemoting,
	}

	summary.NextTier = NextTier(mine.Tier.Slug)
	if summary.NextTier != nil {
		summary.PointsToNext = summary.NextTier.MinPoints - mine.Points
		if summary.PointsToNext < 0 {
			summary.PointsToNext = 0
		}
		// Progresso dentro do corte atual: de onde o corte começa até onde o
		// próximo começa. Uma liga sem teto (Diamante) fica em 100% quando o
		// usuário já a alcançou.
		tiers := ArenaTiers()
		tierIdx := 0
		for i, tier := range tiers {
			if tier.Slug == mine.Tier.Slug {
				tierIdx = i
			}
		}
		span := summary.NextTier.MinPoints - tiers[tierIdx].MinPoints
		if span <= 0 {
			summary.TierProgress = 1
		} else {
			summary.TierProgress = float64(mine.Points-tiers[tierIdx].MinPoints) / float64(span)
			if summary.TierProgress > 1 {
				summary.TierProgress = 1
			}
			if summary.TierProgress < 0 {
				summary.TierProgress = 0
			}
		}
	}

	// Vizinho imediatamente acima.
	if board.MeIndex > 0 {
		above := board.Standings[board.MeIndex-1]
		gap := above.Points - mine.Points
		if gap <= 0 {
			// Empate: o desempate é a ofensiva, então um empate de XP ainda tem
			// um "falta" honesto de ponto.
			gap = 1
		}
		summary.Ahead = &ArenaRival{UserID: above.UserID, Name: above.Name, Gap: gap}
	}
	// Vizinho imediatamente abaixo.
	if board.MeIndex+1 < len(board.Standings) {
		below := board.Standings[board.MeIndex+1]
		gap := mine.Points - below.Points
		if gap <= 0 {
			gap = 1
		}
		summary.Behind = &ArenaRival{UserID: below.UserID, Name: below.Name, Gap: gap}
	}

	return summary
}

// ---- Texto ----

// GrantMessage é o texto que o app mostra quando o XP cai. Fica aqui para a
// frase ser a mesma em qualquer handler que creditar XP.
func GrantMessage(points int) string {
	if points <= 0 {
		return ""
	}
	return fmt.Sprintf("+%d XP", points)
}
