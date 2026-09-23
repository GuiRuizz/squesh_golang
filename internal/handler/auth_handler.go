package handler

import (
	"net/http"
	"time"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"
	"squesh_golang/internal/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB *gorm.DB
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{DB: db}
}

// issueTokens gera o par (access token + refresh token) e persiste o refresh
// token no banco. Retorna o access token, o refresh token e o tempo de vida
// (em segundos) do access token.
func (h *AuthHandler) issueTokens(user *domain.User) (accessToken, refreshToken string, expiresIn int64, err error) {
	accessToken, err = utils.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		return "", "", 0, err
	}

	refreshToken, err = utils.GenerateRefreshToken()
	if err != nil {
		return "", "", 0, err
	}

	refreshTokenRecord := domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: utils.HashToken(refreshToken),
		ExpiresAt: time.Now().Add(utils.RefreshTokenTTL()),
	}
	if err := h.DB.Create(&refreshTokenRecord).Error; err != nil {
		return "", "", 0, err
	}

	return accessToken, refreshToken, int64(utils.AccessTokenTTL().Seconds()), nil
}

// respondWithTokens monta a resposta padrão de autenticação (login, registro e refresh)
func (h *AuthHandler) respondWithTokens(c *gin.Context, status int, user *domain.User, message string) {
	accessToken, refreshToken, expiresIn, err := h.issueTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gerar os tokens de acesso"})
		return
	}

	resp := gin.H{
		"token":         accessToken,
		"refresh_token": refreshToken,
		"expires_in":    expiresIn, // segundos de validade do access token
		"token_type":    "Bearer",
		"user": gin.H{
			"id":    user.ID,
			"name":  user.Name,
			"email": user.Email,
			"role":  user.Role,
		},
	}
	if message != "" {
		resp["message"] = message
	}

	c.JSON(status, resp)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var dto dto.LoginDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Credenciais inválidas"})
		return
	}

	var user domain.User
	if err := h.DB.Where("email = ?", dto.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "E-mail ou senha incorretos"})
		return
	}

	// Compara a senha enviada no corpo com a senha em hash do banco
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(dto.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "E-mail ou senha incorretos"})
		return
	}

	h.respondWithTokens(c, http.StatusOK, &user, "")
}

func (h *AuthHandler) Register(c *gin.Context) {
	var dto dto.RegisterDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Criptografa a senha com bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(dto.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao processar senha"})
		return
	}

	user := domain.User{
		Name:     dto.Name,
		Email:    dto.Email,
		Password: string(hashedPassword),
		Role:     "user", // Define a role padrão de criação
	}

	if err := h.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "E-mail já cadastrado"})
		return
	}

	h.respondWithTokens(c, http.StatusCreated, &user, "Usuário criado com sucesso")
}

// Refresh troca um refresh token válido por um novo par de tokens (rotação).
// É o endpoint que o App chama ao abrir (se o access token já expirou) ou ao
// receber um 401, mantendo o usuário logado mesmo com o App fechado.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var dto dto.RefreshDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token é obrigatório"})
		return
	}

	tokenHash := utils.HashToken(dto.RefreshToken)
	var stored domain.RefreshToken
	if err := h.DB.Where("token_hash = ?", tokenHash).First(&stored).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token inválido"})
		return
	}

	if stored.RevokedAt != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token revogado"})
		return
	}

	if time.Now().After(stored.ExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expirado"})
		return
	}

	var user domain.User
	if err := h.DB.First(&user, "id = ?", stored.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não encontrado"})
		return
	}

	// Rotação: gera um novo refresh token e, numa transação, revoga o antigo
	// e grava o novo. Assim, um token usado indevidamente perde validade.
	newRefreshToken, err := utils.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gerar o novo refresh token"})
		return
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&stored).Update("revoked_at", now).Error; err != nil {
			return err
		}

		record := domain.RefreshToken{
			UserID:    user.ID,
			TokenHash: utils.HashToken(newRefreshToken),
			ExpiresAt: now.Add(utils.RefreshTokenTTL()),
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao renovar a sessão"})
		return
	}

	accessToken, err := utils.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gerar o token de acesso"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":         accessToken,
		"refresh_token": newRefreshToken,
		"expires_in":    int64(utils.AccessTokenTTL().Seconds()),
		"token_type":    "Bearer",
		"user": gin.H{
			"id":    user.ID,
			"name":  user.Name,
			"email": user.Email,
			"role":  user.Role,
		},
	})
}

// Logout revoga um refresh token específico, encerrando a sessão do dispositivo.
func (h *AuthHandler) Logout(c *gin.Context) {
	var dto dto.RefreshDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token é obrigatório"})
		return
	}

	tokenHash := utils.HashToken(dto.RefreshToken)
	result := h.DB.Model(&domain.RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Update("revoked_at", time.Now())

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao encerrar a sessão"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token inválido"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logout realizado com sucesso"})
}
