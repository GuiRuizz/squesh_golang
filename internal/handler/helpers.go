package handler

import (
	"net/http"
	"strings"

	"squesh_golang/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// userIDFromContext extrai o ID do usuário autenticado, definido pelo
// AuthMiddleware no contexto do Gin (aceita tanto uuid.UUID quanto string).
func userIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("userID")
	if !exists {
		return uuid.Nil, false
	}

	switch v := val.(type) {
	case uuid.UUID:
		return v, true
	case string:
		id, err := uuid.Parse(v)
		return id, err == nil
	default:
		return uuid.Nil, false
	}
}

// requireUserID é o userIDFromContext para handlers protegidos: quando não há
// usuário logado, já escreve o 401 e devolve false.
func requireUserID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
		return uuid.Nil, false
	}
	return id, true
}

// optionalUserID tenta extrair o usuário de um Bearer token, se presente.
// Retorna (uuid.Nil, false) quando não há token ou ele é inválido — o handler
// continua como rota pública nesses casos.
func optionalUserID(c *gin.Context) (uuid.UUID, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return uuid.Nil, false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return uuid.Nil, false
	}

	claims, err := utils.ValidateToken(parts[1])
	if err != nil {
		return uuid.Nil, false
	}
	return claims.UserID, true
}