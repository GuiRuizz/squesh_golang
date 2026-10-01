package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

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

// GetMyPosts (GET /users/me/posts) alimenta a tela "Meus Posts" das
// configurações. O dono vem do token — nunca de parâmetro — e o resultado
// traz os comentários com o autor, para a tela conseguir listar/apagar.
func (h *PostHandler) GetMyPosts(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var posts []domain.Post
	if err := h.DB.Where("user_id = ?", userID).
		Preload("User").Preload("Comments.User").
		Order("created_at desc").Find(&posts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar seus posts"})
		return
	}
	if posts == nil {
		posts = []domain.Post{}
	}

	pp := make([]*domain.Post, len(posts))
	for i := range posts {
		pp[i] = &posts[i]
	}
	// Feed sem curtidas é diferente: o card de "Meus Posts" também mostra likes.
	h.enrichPosts(pp, &userID)

	c.JSON(http.StatusOK, posts)
}

// UpdateCaptionDTO aceita legenda vazia de propósito: limpar a legenda é
// uma edição legítima (o app permite). O limite é o mesmo do app (2200).
type UpdateCaptionDTO struct {
	Caption string `json:"caption"`
}

const maxCaptionLength = 2200

func (h *PostHandler) UpdatePostCaption(c *gin.Context) {
	// Sem esse Parse o "id = ?" manda texto para uma coluna uuid e o Postgres
	// responde erro de cast, que virava 500 em vez de 400.
	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	var body UpdateCaptionDTO

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos"})
		return
	}
	if len([]rune(body.Caption)) > maxCaptionLength {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("A legenda pode ter no máximo %d caracteres", maxCaptionLength),
		})
		return
	}

	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	// Só o dono edita. Sem esta checagem qualquer usuário logado mudava a
	// legenda de qualquer post pelo ID.
	result := h.DB.Model(&domain.Post{}).
		Where("id = ? AND user_id = ?", postID, userID).
		Update("caption", body.Caption)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao editar o post"})
		return
	}
	if result.RowsAffected == 0 {
		// Não distingue "não existe" de "não é seu" de propósito: confirmar a
		// existência do post alheio já é vazar informação.
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	var post domain.Post
	if err := h.DB.Preload("User").First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
		return
	}

	// likes_count é virtual no domain.Post; a tela recarrega a lista, mas
	// devolver o post já com o número evita a diferença visual.
	h.enrichPosts([]*domain.Post{&post}, &userID)

	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	postID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de post inválido"})
		return
	}

	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	// Apagar também leva junto curtidas e comentários: sem cascata eles
	// viravam linhas órfãs apontando para um post inexistente.
	// O `err` de cima (do uuid.Parse) ja foi tratado e devolvido; aqui o
	// nome e reatribuido para o erro da transacao.
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&domain.Post{}, "id = ? AND user_id = ?", postID, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		if err := tx.Where("post_id = ?", postID).Delete(&domain.PostLike{}).Error; err != nil {
			return err
		}
		return tx.Where("post_id = ?", postID).Delete(&domain.Comment{}).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Post não encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao apagar o post"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	// Autor = usuário logado (vem do token). Antes o corpo mandava user_id e
	// qualquer um podia publicar como outra pessoa.
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return
	}

	var input dto.CreatePostDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	post := domain.Post{
		UserID:   userID,
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
	h.enrichPosts([]*domain.Post{&post}, &userID)

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
