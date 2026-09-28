package main

import (
	"log"

	"squesh_golang/internal/database"
	"squesh_golang/internal/routes"
	"squesh_golang/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	// Carrega o arquivo .env se existir (funciona no Docker e no "go run").
	// Variáveis já definidas no ambiente (ex.: docker-compose) NÃO são sobrescritas;
	// sem o arquivo, a aplicação usa os valores padrão dos utilitários.
	if err := godotenv.Load(); err != nil {
		log.Println("Aviso: arquivo .env não encontrado — usando variáveis de ambiente / padrões")
	}

	store, err := storage.New()
	if err != nil {
		log.Fatalf("Erro ao inicializar storage: %v", err)
	}

	db := database.InitDB()

	r := routes.SetupRouter(db, store)

	r.Run("0.0.0.0:8080")
}
