package handler

import (
	"github.com/google/uuid"
	"net/http"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PostHandler struct {
	DB *gorm.DB
}

func NewPostHandler(db *gorm.DB) *PostHandler {
	return &PostHandler{DB: db}
}

// enrichPosts preenche likes_count (e liked_by_me quando há viewer logado)
// de vários posts de uma vez, evitando N+1 queries.
// viewerID nil → resposta pública: só o contador de curtidas aparece.
func (h *PostHandler) enrichPosts(posts []*domain.Post, viewerID *uuid.UUID) {
	if len(posts) == 0 {
		return
	}

	ids := make([]uuid.UUID, 0, len(posts))
	for _, p := range posts {
		ids = append(ids, p.ID)
	}

	// 1. Contador de curtidas por post (1 query agrupada)
	var counts []struct {
		PostID uuid.UUID
		Cnt    int
	}
	if err := h.DB.Model(&domain.PostLike{}).
		Select("post_id, count(*) as cnt").
		Where("post_id IN ?", ids).
		Group("post_id").
		Scan(&counts).Error; err != nil {
		return
	}
	likeCounts := make(map[uuid.UUID]int, len(counts))
	for _, row := range counts {
		likeCounts[row.PostID] = row.Cnt
	}

	// 2. Quais destes posts o viewer já curtiu (só quando logado)
	likedSet := map[uuid.UUID]bool{}
	if viewerID != nil {
		var myLikes []uuid.UUID
		if err := h.DB.Model(&domain.PostLike{}).
			Where("user_id = ? AND post_id IN ?", *viewerID, ids).
			Pluck("post_id", &myLikes).Error; err != nil {
			return
		}
		for _, id := range myLikes {
			likedSet[id] = true
		}
	}

	// 3. Aplica nos posts
	for _, p := range posts {
		p.LikesCount = likeCounts[p.ID]
		if viewerID != nil {
			liked := likedSet[p.ID]
			p.LikedByMe = &liked
		}
	}
}

func (h *PostHandler) GetFeed(c *gin.Context) {
	var posts []domain.Post
	if err := h.DB.Preload("User").Preload("Comments.User").Order("created_at desc").Find(&posts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar posts do feed"})
		return
	}

	// Auth OPCIONAL: se houver Bearer token, cada post ganha liked_by_me.
	var viewer *uuid.UUID
	if id, ok := optionalUserID(c); ok {
		viewer = &id
	}
	pp := make([]*domain.Post, len(posts))
	for i := range posts {
		pp[i] = &posts[i]
	}
	h.enrichPosts(pp, viewer)

	c.JSON(http.StatusOK, posts)
}

// GetPersonalizedFeed (GET /posts/feed) devolve os posts de quem o usuário
// segue + os próprios posts, com curtidas e autor preenchidos.
func (h *PostHandler) GetPersonalizedFeed(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	// Ids de quem eu sigo + eu mesmo (para o feed não ficar vazio no começo)
	var following []uuid.UUID
	if err := h.DB.Model(&domain.Follow{}).
		Where("follower_id = ?", userID).
		Pluck("following_id", &following).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar seguidos"})
		return
	}
	following = append(following, userID)

	var posts []domain.Post
	if err := h.DB.Preload("User").Preload("Comments.User").
		Where("user_id IN ?", following).
		Order("created_at desc").
		Find(&posts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar feed personalizado"})
		return
	}

	pp := make([]*domain.Post, len(posts))
	for i := range posts {
		pp[i] = &posts[i]
	}
	h.enrichPosts(pp, &userID)

	c.JSON(http.StatusOK, posts)
}

func (h *PostHandler) GetUserPosts(c *gin.Context) {
	userID := c.Param("user_id")
	var posts []domain.Post
	if err := h.DB.Where("user_id = ?", userID).Preload("Comments.User").Order("created_at desc").Find(&posts).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Posts não encontrados para este usuário"})
		return
	}
	c.JSON(http.StatusOK, posts)
}

type UpdateCaptionDTO struct {
	Caption string `json:"caption" binding:"required"`
}

func (h *PostHandler) UpdatePostCaption(c *gin.Context) {
	postID := c.Param("id")
	var dto UpdateCaptionDTO

	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	h.DB.Model(&post).Update("caption", dto.Caption)
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	postID := c.Param("id")
	result := h.DB.Delete(&domain.Post{}, "id = ?", postID)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao apagar o post"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	var input dto.CreatePostDTO

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	parsedUserID, err := uuid.Parse(input.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de usuário inválido"})
		return
	}

	// Valida se o usuário existe no banco
	var user domain.User
	if err := h.DB.First(&user, "id = ?", parsedUserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	post := domain.Post{
		UserID:   parsedUserID,
		ImageURL: input.ImageURL,
		Caption:  input.Caption,
	}

	if result := h.DB.Create(&post); result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar post: " + result.Error.Error()})
		return
	}

	// Recarrega com o autor para o app já ter os dados de exibição
	h.DB.Preload("User").First(&post, "id = ?", post.ID)

	// Preenche curtidas do post recém-criado (0 likes, liked_by_me = false)
	h.enrichPosts([]*domain.Post{&post}, &parsedUserID)

	c.JSON(http.StatusCreated, post)
}

// ListComments retorna os comentários de um post (público, como o feed)
func (h *PostHandler) ListComments(c *gin.Context) {
	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	var comments []domain.Comment
	if err := h.DB.Where("post_id = ?", postID).Preload("User").Order("created_at asc").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar comentários"})
		return
	}

	c.JSON(http.StatusOK, comments)
}

// CreateComment adiciona um comentário no post (autenticado)
func (h *PostHandler) CreateComment(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	var input dto.CreateCommentDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text é obrigatório"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	comment := domain.Comment{
		PostID: postID,
		UserID: userID,
		Text:   input.Text,
	}
	if err := h.DB.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar comentário"})
		return
	}

	// Recarrega com o autor para o app já ter os dados de exibição
	h.DB.Preload("User").First(&comment, "id = ?", comment.ID)
	c.JSON(http.StatusCreated, comment)
}

// DeleteComment apaga um comentário (apenas o autor ou um admin)
func (h *PostHandler) DeleteComment(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}
	role, _ := c.Get("userRole")

	commentID, err := uuid.Parse(c.Param("commentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de comentário inválido"})
		return
	}

	var comment domain.Comment
	if err := h.DB.First(&comment, "id = ?", commentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comentário não encontrado"})
		return
	}

	if comment.UserID != userID && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Você só pode apagar os próprios comentários"})
		return
	}

	if err := h.DB.Delete(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao apagar comentário"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// LikePost curte um post (POST /posts/:id/like). Idempotente: curtir de novo
// não é erro — retorna o estado atual.
func (h *PostHandler) LikePost(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	var count int64
	if err := h.DB.Model(&domain.PostLike{}).
		Where("user_id = ? AND post_id = ?", userID, postID).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao verificar curtida"})
		return
	}

	if count == 0 {
		like := domain.PostLike{UserID: userID, PostID: postID}
		if err := h.DB.Create(&like).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao curtir o post"})
			return
		}
	}

	// Total de curtidas para retornar junto com o estado
	var total int64
	if err := h.DB.Model(&domain.PostLike{}).
		Where("post_id = ?", postID).
		Count(&total).Error; err != nil {
		total = count + 1
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Post curtido",
		"liked":       true,
		"likes_count": total,
	})
}

// UnlikePost remove a curtida (DELETE /posts/:id/like). Idempotente: retorna
// 204 mesmo que o usuário não tenha curtido antes.
func (h *PostHandler) UnlikePost(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	if err := h.DB.Where("user_id = ? AND post_id = ?", userID, postID).
		Delete(&domain.PostLike{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao remover curtida"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// GetPostLikes lista os usuários que curtiram um post (público, como os comentários)
func (h *PostHandler) GetPostLikes(c *gin.Context) {
	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	var post domain.Post
	if err := h.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	var likes []domain.PostLike
	if err := h.DB.Where("post_id = ?", postID).Order("created_at asc").Find(&likes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar curtidas"})
		return
	}

	ids := make([]uuid.UUID, 0, len(likes))
	for _, l := range likes {
		ids = append(ids, l.UserID)
	}

	var users []domain.User
	if len(ids) > 0 {
		if err := h.DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar usuários"})
			return
		}
	}

	c.JSON(http.StatusOK, users)
}
