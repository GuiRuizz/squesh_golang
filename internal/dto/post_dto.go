package dto

// CreatePostDTO representa o payload recebido ao criar um post
type CreatePostDTO struct {
	UserID   string `json:"user_id" binding:"required,uuid"`
	ImageURL string `json:"image_url" binding:"required,url"`
	Caption  string `json:"caption"`
}

// CreateCommentDTO representa o payload recebido ao criar um comentário
type CreateCommentDTO struct {
	Text string `json:"text" binding:"required"`
}
