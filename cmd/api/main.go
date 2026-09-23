package main

import (
	"log"

	"squesh_golang/internal/database"
	"squesh_golang/internal/routes"

	"github.com/joho/godotenv"
)

func main() {
	// Carrega o arquivo .env se existir (funciona no Docker e no "go run").
	// Variáveis já definidas no ambiente (ex.: docker-compose) NÃO são sobrescritas;
	// sem o arquivo, a aplicação usa os valores padrão dos utilitários.
	if err := godotenv.Load(); err != nil {
		log.Println("Aviso: arquivo .env não encontrado — usando variáveis de ambiente / padrões")
	}

	db := database.InitDB()

	r := routes.SetupRouter(db)

	r.Run("0.0.0.0:8080")
}
