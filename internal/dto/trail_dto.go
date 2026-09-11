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