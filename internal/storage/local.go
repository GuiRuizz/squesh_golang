package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage é o "S3 caseiro": grava os arquivos num diretório e serve via
// rotas da própria API, mas com EXATAMENTE o mesmo fluxo de URL assinada que o
// S3 terá depois. Trocou para STORAGE_DRIVER=s3, o app não muda nada.
type LocalStorage struct {
	uploadDir string
	baseURL   string
	secret    []byte
}

func NewLocal() (*LocalStorage, error) {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, err
	}

	baseURL := os.Getenv("STORAGE_PUBLIC_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	secret := os.Getenv("STORAGE_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if secret == "" {
		secret = "sua_chave_secreta_super_segura"
	}

	return &LocalStorage{
		uploadDir: uploadDir,
		baseURL:   strings.TrimRight(baseURL, "/"),
		secret:    []byte(secret),
	}, nil
}

// PresignUpload gera a URL de upload assinada (PUT):
// {base}/api/v1/uploads/{key}?exp={unix}&sig={hmac}
// O key mantém as barras como em um object key de S3 (ex.: posts/abc.png).
func (s *LocalStorage) PresignUpload(key, contentType string, expiresIn time.Duration) (string, error) {
	exp := time.Now().Add(expiresIn).Unix()
	sig := s.signature(key, exp)
	return fmt.Sprintf("%s/api/v1/uploads/%s?exp=%d&sig=%s", s.baseURL, key, exp, sig), nil
}

// PublicURL gera o endereço público de leitura do arquivo.
func (s *LocalStorage) PublicURL(key string) (string, error) {
	return fmt.Sprintf("%s/api/v1/files/%s", s.baseURL, key), nil
}

// ValidateUploadToken valida expiração e assinatura de uma URL de upload.
func (s *LocalStorage) ValidateUploadToken(key string, exp int64, sig string) error {
	if time.Now().Unix() > exp {
		return fmt.Errorf("URL de upload expirada")
	}
	expected := s.signature(key, exp)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return fmt.Errorf("assinatura de upload inválida")
	}
	return nil
}

// SaveFile grava o conteúdo do upload no diretório local, protegendo contra
// chaves que tentem escapar do diretório de uploads.
func (s *LocalStorage) SaveFile(key string, data []byte) error {
	if key == "" || strings.HasPrefix(key, "/") ||
		strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return fmt.Errorf("chave de arquivo inválida")
	}

	fullPath := filepath.Join(s.uploadDir, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.uploadDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("chave de arquivo inválida")
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, data, 0o644)
}

// UploadDir retorna o diretório raiz dos arquivos (usado pela rota estática).
func (s *LocalStorage) UploadDir() string {
	return s.uploadDir
}

func (s *LocalStorage) signature(key string, exp int64) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(fmt.Sprintf("%s|%d", key, exp)))
	return hex.EncodeToString(mac.Sum(nil))
}
