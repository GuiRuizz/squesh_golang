package handler

import (
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
