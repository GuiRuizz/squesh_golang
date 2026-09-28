package storage

import (
	"fmt"
	"os"
	"time"
)

// Storage abstrai o backend de arquivos. O fluxo é de URL assinada (presigned):
//   1. O app chama PresignUpload e recebe uma URL única e temporária.
//   2. O app faz o upload DIRETO nessa URL (a assinatura é a autenticação).
//   3. O app usa PublicURL como endereço público do arquivo.
// Assim o servidor nunca recebe o binário e a troca futura por S3/MinIO é
// invisível para os clientes.
type Storage interface {
	// PresignUpload gera a URL assinada para upload direto.
	PresignUpload(key, contentType string, expiresIn time.Duration) (string, error)

	// PublicURL gera o endereço público de leitura do arquivo.
	PublicURL(key string) (string, error)

	// Os métodos abaixo pertencem ao transporte LOCAL (a própria API recebe o
	// PUT). No driver S3 eles serão ignorados/retornarão erro, porque o upload
	// acontece direto no bucket.
	ValidateUploadToken(key string, exp int64, sig string) error
	SaveFile(key string, data []byte) error
	UploadDir() string
}

// New cria o driver configurado via STORAGE_DRIVER (default: "local").
func New() (Storage, error) {
	driver := os.Getenv("STORAGE_DRIVER")
	if driver == "" {
		driver = "local"
	}

	switch driver {
	case "local":
		return NewLocal()
	case "s3":
		return nil, fmt.Errorf("driver S3 ainda não implementado — use STORAGE_DRIVER=local por enquanto")
	default:
		return nil, fmt.Errorf("driver de storage desconhecido: %s", driver)
	}
}
