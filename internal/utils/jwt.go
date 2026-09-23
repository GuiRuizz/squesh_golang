package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func getJWTSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("sua_chave_secreta_super_segura")
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// AccessTokenTTL retorna o tempo de vida do token de acesso (padrão: 24h).
// Enquanto o App não implementar o refresh, mantenha um valor maior para não
// derrubar sessões existentes. Depois, pode baixar para 15m-1h por segurança.
func AccessTokenTTL() time.Duration {
	return getDurationEnv("ACCESS_TOKEN_EXPIRES", 24*time.Hour)
}

// RefreshTokenTTL retorna o tempo de vida do refresh token (padrão: 30 dias).
// É ele que mantém o usuário logado mesmo com o App fechado.
func RefreshTokenTTL() time.Duration {
	return getDurationEnv("REFRESH_TOKEN_EXPIRES", 30*24*time.Hour)
}

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"` // Guardamos a role no token
	jwt.RegisteredClaims
}

// GenerateAccessToken gera o JWT de acesso (curta duração) com o ID e a Role do usuário
func GenerateAccessToken(userID uuid.UUID, role string) (string, error) {
	expirationTime := time.Now().Add(AccessTokenTTL())

	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTSecret())
}

// GenerateRefreshToken gera um refresh token opaco e aleatório (256 bits).
// Ele NÃO é um JWT: é guardado apenas com hash no banco e usado só no
// endpoint /auth/refresh para emitir um novo par de tokens.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken gera o hash SHA-256 (hex) de um token. Usado para guardar o
// refresh token no banco sem expor o valor original.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inválido")
		}
		return getJWTSecret(), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token inválido ou expirado")
	}

	return claims, nil
}
