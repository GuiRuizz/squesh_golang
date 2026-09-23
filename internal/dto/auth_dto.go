package dto

type LoginDTO struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RegisterDTO struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// RefreshDTO é o corpo das requisições de refresh e logout
type RefreshDTO struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}