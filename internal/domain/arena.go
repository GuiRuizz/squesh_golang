package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Tipos de evento que rendem XP na Arena de Ligas.
//
// O XP do app NÃO é um contador solto: cada conquista vira uma linha em
// `user_xp_events`. Isso é o que permite ranquear por semana e por mês sem
// inventar número (basta filtrar as linhas pelo período) e o que impede a
// mesma faita de pagar duas vezes.
const (
	// XPEventStepCheck: uma etapa interna marcada (uma refeição do dia ou um
	// exercício da sessão).
	XPEventStepCheck = "step_check"
	// XPEventItemComplete: o item chegou a 100% (todas as etapas marcadas).
	XPEventItemComplete = "item_complete"
	// XPEventDayComplete: o dia fechou 100% de alimentação E 100% de treino.
	XPEventDayComplete = "day_complete"
	// XPEventStreakBonus: o prêmio de manter a ofensiva viva, junto do dia
	// completo. Cresce com a sequência e tem teto.
	XPEventStreakBonus = "streak_bonus"
)

// XPEvent é UMA linha do razão de XP.
//
// A chave única (user_id, kind, ref_key, day_key) é a garantia de que um
// mesmo feito nunca paga duas vezes: o app reenviando a requisição, o dia sendo
// reprocessado pela rotina de dias, ou a tela de arena chamando a sincronização
// várias vezes não criam XP extra.
type XPEvent struct {
	ID     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_xp_once,priority:1;index:idx_xp_user_day,priority:1" json:"user_id"`
	// Kind é um dos XPEvent*.
	Kind string `gorm:"type:varchar(24);not null;uniqueIndex:idx_xp_once,priority:2" json:"kind"`
	// RefKey identifica o que gerou o XP: "<itemID>:<stepIndex>" numa etapa
	// interna, "<itemID>" num item completo e VAZIO nos eventos de dia (esses
	// são únicos por dia de qualquer forma).
	RefKey string `gorm:"type:varchar(80);not null;default:'';uniqueIndex:idx_xp_once,priority:3" json:"ref_key"`
	// DayKey é o dia no fuso do app, em "2006-01-02".
	//
	// Guardar o DIA (e não o instante) é proposital: o filtro por período vira
	// comparação de string, imune ao fuso do Postgres (UTC) trocar a data às
	// 22h, que é exatamente o bug que o `day_state.go` já precisou evitar no
	// cálculo da ofensiva.
	DayKey  string `gorm:"type:varchar(10);not null;index;uniqueIndex:idx_xp_once,priority:4;index:idx_xp_user_day,priority:2" json:"day_key"`
	Points  int    `gorm:"not null" json:"points"`
	// ItemID e TrailType ajudam a explicar a linha na tela ("por que ganhei
	// isso?"); são opcionais porque dia completo e bônus não vêm de um item.
	ItemID    *uuid.UUID `gorm:"type:uuid" json:"item_id,omitempty"`
	TrailType string     `gorm:"type:varchar(20)" json:"trail_type,omitempty"`
	// CreditedAt é o instante em que o XP foi creditado (pode ser bem depois
	// do fato, quando a rotina de dias roda e descobre o dia completo).
	CreditedAt time.Time `gorm:"not null" json:"credited_at"`
}

// TableName fixa o nome da tabela.
//
// Sem isso o GORM pluralizaria `XPEvent` como `xp_events`, quebrando a
// convenção das outras tabelas que começam com `user_` (`user_subscriptions`,
// `user_trail_progresses`).
func (XPEvent) TableName() string { return "user_xp_events" }

func (e *XPEvent) BeforeCreate(tx *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return
}
