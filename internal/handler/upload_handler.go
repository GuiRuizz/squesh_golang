package handler

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"squesh_golang/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxUploadSize = 5 << 20 // 5MB

var allowedImageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

type UploadHandler struct {
	Storage storage.Storage
}

func NewUploadHandler(s storage.Storage) *UploadHandler {
	return &UploadHandler{Storage: s}
}

// PresignUpload (POST /uploads/presign, protegido) devolve a URL assinada onde
// o app deve fazer o upload direto, além da URL pública final da imagem.
func (h *UploadHandler) PresignUpload(c *gin.Context) {
	var input struct {
		Filename    string `json:"filename" binding:"required"`
		ContentType string `json:"content_type" binding:"required"`
		// Folder separa o que é foto de post do que é foto de perfil. Vazio
		// significa "posts" (o app antigo não mandava o campo).
		Folder string `json:"folder" binding:"omitempty,oneof=posts avatars"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename e content_type são obrigatórios"})
		return
	}

	folder := input.Folder
	if folder == "" {
		folder = "posts"
	}

	ext := strings.ToLower(filepath.Ext(input.Filename))
	if !allowedImageExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "formato de imagem não suportado (use jpg, jpeg, png, webp ou gif)"})
		return
	}

	// Chave única e opaca; a extensão real é a que você enviou no arquivo
	key := fmt.Sprintf("%s/%s%s", folder, uuid.NewString(), ext)

	const presignTTL = 15 * time.Minute
	uploadURL, err := h.Storage.PresignUpload(key, input.ContentType, presignTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao assinar URL de upload"})
		return
	}

	imageURL, err := h.Storage.PublicURL(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gerar URL pública"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": uploadURL,
		"image_url":  imageURL,
		"key":        key,
		"expires_in": int(presignTTL.Seconds()),
	})
}

// Upload (PUT /uploads/:key, público) recebe o binário enviado pelo app na URL
// assinada. A autorização é a assinatura na query string, não o token JWT.
func (h *UploadHandler) Upload(c *gin.Context) {
	// Rota wildcard: o c.Param vem com "/" inicial (ex.: "/posts/abc.png")
	key := strings.TrimPrefix(c.Param("key"), "/")
	exp, err := strconv.ParseInt(c.Query("exp"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parâmetro exp ausente ou inválido"})
		return
	}

	if err := h.Storage.ValidateUploadToken(key, exp, c.Query("sig")); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Lê o corpo respeitando o limite de 5MB (lê 1 byte a mais para detectar excesso)
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxUploadSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Erro ao ler o arquivo"})
		return
	}
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Arquivo vazio"})
		return
	}
	if len(body) > maxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Arquivo muito grande (máx. 5MB)"})
		return
	}

	if err := h.Storage.SaveFile(key, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao salvar o arquivo"})
		return
	}

	imageURL, err := h.Storage.PublicURL(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gerar URL pública"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Upload realizado com sucesso",
		"image_url": imageURL,
	})
}
