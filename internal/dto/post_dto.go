package dto

// CreatePostDTO representa o payload recebido ao criar um post.
// O autor NÃO vem do corpo: o handler usa o userID do token, então o app não
// precisa (nem consegue) publicar como outra pessoa.
type CreatePostDTO struct {
	ImageURL string `json:"image_url" binding:"required,url"`
	Caption  string `json:"caption"`
}

// CreateCommentDTO representa o payload recebido ao criar um comentário
type CreateCommentDTO struct {
	Text string `json:"text" binding:"required"`
}
