package dto

type CreateTrailDTO struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Type        string `json:"type" binding:"required,oneof=workout nutrition"`
	Level       string `json:"level" binding:"required"`
}

type CreateTrailItemDTO struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Order       int    `json:"order" binding:"required"`
	Value       string `json:"value"`
}

// GenerateTrailDTO gera uma trilha COMPLETA a partir do conteúdo existente.
// Informe source_trail_id (usa o tipo/nível da trilha modelo) OU type/level.
type GenerateTrailDTO struct {
	SourceTrailID string `json:"source_trail_id"`
	Type          string `json:"type" binding:"omitempty,oneof=workout nutrition"`
	Level         string `json:"level"`
	Title         string `json:"title"` // opcional; senão gera um título numerado
	ItemCount     int    `json:"item_count"`
}
