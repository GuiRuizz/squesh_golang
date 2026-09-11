package dto

// CreateUserDTO representa o payload recebido ao criar um usuário
type CreateUserDTO struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// CreatePostDTO representa o payload recebido ao criar um post
type CreatePostDTO struct {
	UserID   string `json:"user_id" binding:"required,uuid"`
	ImageURL string `json:"image_url" binding:"required,url"`
	Caption  string `json:"caption"`
}

type LoginDTO struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RegisterDTO struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}
