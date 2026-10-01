package dto

import (
	"time"

	"github.com/google/uuid"
)

// ArenaStandingResponseDTO é uma linha do ranking da Arena.
//
// O corte e as zonas chegam JÁ resolvidos do servidor: o app nunca recalcula
// "quem promove e quem cai", ele só desenha. `is_me` existe para o app destacar
// a própria linha sem comparar id em dois lugares.
type ArenaStandingResponseDTO struct {
	Position    int    `json:"position"`      // posição geral no período (1 = líder)
	TierPosition int   `json:"tier_position"` // posição dentro da liga
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatar_url"`
	Points      int       `json:"points"`       // XP do período
	TotalPoints int       `json:"total_points"` // XP da carreira
	Streak      int       `json:"streak"`
	Tier        string    `json:"tier"`     // bronze | silver | gold | platinum | diamond
	IsPromoting bool      `json:"is_promoting"`
	IsDemoting  bool      `json:"is_demoting"`
	IsMe        bool      `json:"is_me"`
}

// ArenaRivalDTO é o vizinho de posição (logo acima / logo abaixo).
type ArenaRivalDTO struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
	Gap    int       `json:"gap"` // XP que falta para alcançar
}

// ArenaSummaryDTO é o card "como eu estou" no topo da tela.
type ArenaSummaryDTO struct {
	Position      int            `json:"position"`
	TierPosition  int            `json:"tier_position"`
	Tier          string         `json:"tier"`
	Points        int            `json:"points"`
	TotalPoints   int            `json:"total_points"`
	Streak        int            `json:"streak"`
	NextTier      *string        `json:"next_tier"`        // vazio = já no topo
	PointsToNext  int            `json:"points_to_next"`   // 0 quando não há próxima
	TierProgress  float64        `json:"tier_progress"`    // 0..1 dentro do corte
	IsPromoting   bool           `json:"is_promoting"`
	IsDemoting    bool           `json:"is_demoting"`
	Ahead         *ArenaRivalDTO `json:"ahead"`  // null = está em 1º
	Behind        *ArenaRivalDTO `json:"behind"` // null = está em último
}

// ArenaBoardResponseDTO é a resposta de `GET /arena`.
//
// `tiers` vai junto para o app montar a legenda e a barra de progresso sem
// duplicar os cortes em código Dart (a mesma regra que a vitrine de planos).
type ArenaBoardResponseDTO struct {
	Period    string                       `json:"period"` // week | month | all
	From      *time.Time                   `json:"from"`   // null em "all"
	To        *time.Time                   `json:"to"`     // null em "all"
	Tiers     []ArenaTierDTO               `json:"tiers"`
	Me        *ArenaSummaryDTO             `json:"me"`         // null se eu não estiver
	Standings []ArenaStandingResponseDTO   `json:"standings"`
	Total     int                          `json:"total"`
}

// ArenaTierDTO é um corte de liga com as zonas de promoção/rebaixamento.
type ArenaTierDTO struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	MinPoints      int    `json:"min_points"`
	PromotionSlots int    `json:"promotion_slots"`
	DemotionSlots  int    `json:"demotion_slots"`
}

// XPGrantDTO volta em quem ganha XP na hora (item concluído, etapa marcada), para
// o app mostrar o "+N XP" sem adivinhar.
type XPGrantDTO struct {
	XP     int    `json:"xp"`
	Streak int    `json:"streak"`
	Message string `json:"message,omitempty"`
}
