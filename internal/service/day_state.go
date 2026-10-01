package service

import (
	"os"
	"time"

	"squesh_golang/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AppLocation é o fuso que define "um dia" para o app.
//
// O container da API roda em America/Sao_Paulo (-03) mas o Postgres roda em
// UTC: qualquer DATE() no SQL devolveria o dia errado às 22h. Por isso o dia
// NUNCA é calculado no banco — calculamos aqui e passamos timestamps prontos.
var AppLocation = mustLoadLocation()

func mustLoadLocation() *time.Location {
	if name := os.Getenv("APP_TZ"); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return loc
	}
	// Imagem sem tzdata: o Brasil não tem mais horário de verão, então o
	// deslocamento fixo de -03 é correto e não depende de arquivos de fuso.
	return time.FixedZone("America/Sao_Paulo", -3*60*60)
}

// DayStart devolve a meia-noite (no fuso do app) do dia a que o instante pertence.
func DayStart(t time.Time) time.Time {
	l := t.In(AppLocation)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, AppLocation)
}

// DayStateSync é o resultado de sincronizar o estado dos dias do usuário.
type DayStateSync struct {
	// ResetDays são os dias passados em que o usuário deixou progresso parcial:
	// os "selects" daquele dia foram apagados (sem crédito pela metade).
	ResetDays []time.Time
	// Streak é a ofensiva recalculada: dias consecutivos em que o usuário
	// fechou 100% de um dia de ALIMENTAÇÃO e 100% de uma sessão de TREINO.
	Streak int
}

type progressRow struct {
	ItemID      uuid.UUID        `gorm:"column:item_id"`
	Type        domain.TrailType `gorm:"column:type"`
	Needed      int              `gorm:"column:needed"`
	CompletedAt time.Time        `gorm:"column:completed_at"`
}

// SyncAndReward consolida os dias do usuário, recalcula a ofensiva E credita o
// XP da Arena.
//
// É o ponto de entrada que o resto do app deve usar: os dois passos leem os
// mesmos dias completos, então roda-los juntos garante que não existe caminho em
// que um dia valeu para a ofensiva mas não rendeu XP (ou o contrário).
//
// A diferença para `SyncUserDays` é sutil e importa: a rotina de dias também
// DELETA o progresso parcial dos dias passados, então rodar os dois separados
// abriria uma janela em que o dia existia para um e não para o outro.
func SyncAndReward(db *gorm.DB, userID uuid.UUID) (DayStateSync, int, error) {
	state, err := SyncUserDays(db, userID)
	if err != nil {
		return state, 0, err
	}
	gained, err := SyncDayRewards(db, userID)
	if err != nil {
		return state, 0, err
	}
	return state, gained, nil
}

// SyncUserDays consolida os dias do usuário e recalcula a ofensiva.
//
// Regras:
//  1. Um dia só é COMPLETO com 100% de um dia de alimentação E 100% de uma
//     sessão de treino. Basta um dos dois faltar para o dia não valer.
//  2. Nos dias PASSADOS que não ficaram completos, todo o progresso é apagado:
//     os selects voltam a ficar desmarcados, sem crédito parcial. O dia de hoje
//     NÃO é tocado (ainda está em andamento).
//  3. A ofensiva é sempre derivada dos dias completos, contando de hoje para
//     trás. O dia de hoje ainda em aberto não quebra a ofensiva — ele apenas
//     ainda não conta.
//
// A operação é idempotente e roda de forma preguiçosa (lazy) a cada leitura ou
// escrita relevante: não depende de cron nem de o servidor estar no ar à
// meia-noite para fechar o dia.
func SyncUserDays(db *gorm.DB, userID uuid.UUID) (DayStateSync, error) {
	var out DayStateSync

	rows, err := loadProgressRows(db, userID)
	if err != nil {
		return out, err
	}
	if len(rows) == 0 {
		// Sem progresso: a ofensiva é zero e não há nada a limpar. Ainda assim
		// zera a coluna, para não sobrar streak de um estado antigo.
		return out, persistStreak(db, userID, 0)
	}

	// Agrupa por ITEM: um item está concluído quando o número de etapas
	// marcadas alcança o número de etapas exigidas. O dia do item é o dia do
	// ÚLTIMO passo, para não punir quem fecha o jantar depois da meia-noite.
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

	// completeByDay: por dia (chave "2006-01-02"), quais tipos foram fechados.
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

	perfectDay := func(key string) bool {
		m := completeByDay[key]
		return m[domain.TrailTypeNutrition] && m[domain.TrailTypeWorkout]
	}

	// Itens concluídos que pertencem a um dia completo ficam protegidos: um item
	// fechado às 23h50 e terminado 00h10 tem linhas nos dois dias, e a limpeza de
	// um deles não pode desfazer o dia que contou.
	protected := make(map[uuid.UUID]bool)
	for itemID, agg := range items {
		if agg.count < agg.needed {
			continue
		}
		if perfectDay(DayStart(agg.last).Format("2006-01-02")) {
			protected[itemID] = true
		}
	}

	today := DayStart(time.Now())

	// Dias que possuem alguma linha de progresso (cada um só aparece uma vez).
	daysWithRows := make(map[string]time.Time)
	for _, r := range rows {
		day := DayStart(r.CompletedAt)
		daysWithRows[day.Format("2006-01-02")] = day
	}

	// 1. Limpa o progresso parcial dos dias passados.
	for key, day := range daysWithRows {
		if !day.Before(today) {
			continue // hoje ainda está em andamento: não mexe
		}
		if perfectDay(key) {
			continue // dia completo: mantém tudo
		}

		q := db.Where(
			"user_id = ? AND completed_at >= ? AND completed_at < ?",
			userID, day, day.AddDate(0, 0, 1),
		)
		if len(protected) > 0 {
			ids := make([]uuid.UUID, 0, len(protected))
			for id := range protected {
				ids = append(ids, id)
			}
			q = q.Where("trail_item_id NOT IN ?", ids)
		}
		if err := q.Delete(&domain.UserTrailProgress{}).Error; err != nil {
			return out, err
		}
		out.ResetDays = append(out.ResetDays, day)
	}

	// 2. Recalcula a ofensiva a partir dos dias completos.
	out.Streak = streakFrom(completeByDay, today)

	// 3. Persiste a coluna: o ranking e telas que leem o usuário direto passam a
	// ver o mesmo número calculado aqui.
	if err := persistStreak(db, userID, out.Streak); err != nil {
		return out, err
	}

	return out, nil
}

// loadProgressRows carrega o progresso do usuário já com o tipo da trilha e
// quantas etapas o item exige.
func loadProgressRows(db *gorm.DB, userID uuid.UUID) ([]progressRow, error) {
	var rows []progressRow
	err := db.Raw(
		`SELECT utp.trail_item_id AS item_id,
		        trails.type       AS type,
		        GREATEST(1, jsonb_array_length(COALESCE(trail_items.steps, '[]'::jsonb))) AS needed,
		        utp.completed_at  AS completed_at
		 FROM user_trail_progresses utp
		 JOIN trail_items ON trail_items.id = utp.trail_item_id
		 JOIN trails      ON trails.id = trail_items.trail_id
		 WHERE utp.user_id = ?`,
		userID,
	).Scan(&rows).Error
	return rows, err
}

// streakFrom conta os dias completos consecutivos terminando hoje — ou ontem,
// quando o dia de hoje ainda não foi fechado.
func streakFrom(completeByDay map[string]map[domain.TrailType]bool, today time.Time) int {
	perfect := func(d time.Time) bool {
		m := completeByDay[d.Format("2006-01-02")]
		return m[domain.TrailTypeNutrition] && m[domain.TrailTypeWorkout]
	}

	cursor := today
	if !perfect(cursor) {
		cursor = cursor.AddDate(0, 0, -1)
	}

	streak := 0
	for perfect(cursor) {
		streak++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return streak
}

// persistStreak grava a ofensiva recalculada na coluna do usuário.
//
// Só escreve quando o número muda de verdade: esta função roda a cada leitura
// de perfil/streak/ranking, e um UPDATE incondicional acabaria marcando
// updated_at do usuário a cada requisição (e sujando o dado para nada).
func persistStreak(db *gorm.DB, userID uuid.UUID, streak int) error {
	return db.Model(&domain.User{}).
		Where("id = ? AND streak_count <> ?", userID, streak).
		Update("streak_count", streak).Error
}
